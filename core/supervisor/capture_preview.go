package supervisor

// Capture preview + prefs — the "user decides at capture time" half of the
// capture pipeline (design: plan/2026-08-02 capture options at capture
// time.md; ruled by Harvey the same day).
//
// Capture is two-phase now:
//
//	PreviewCapture(url)        → what WOULD be captured. Resolves the post,
//	                             reports the media inventory INCLUDING the
//	                             video quality ladder with estimated sizes,
//	                             and says whether the dialog should be shown
//	                             (per the vault's own trigger thresholds).
//	                             Writes nothing.
//	CaptureFromURL(url, opts)  → acts on the user's choices.
//
// The point is that nothing here decides FOR the user. It reports facts
// (this video is 85 MB at its smallest rung, 3.5 GB at its best) and the
// vault's stored defaults, and lets the surfaces present that.

import (
	"context"
	"fmt"
	"strings"

	"bruv/core/capture"
	"bruv/internal/repo"
)

// CapturePrefs re-exports the repo type for the RPC surface (stable TS
// shape, same trick as TemplatePrefs).
type CapturePrefs = repo.CapturePrefs

// GetCapturePrefs returns this vault's capture defaults (never an error
// for a fresh vault — it returns the defaults).
func (r *Runtime) GetCapturePrefs() (CapturePrefs, error) {
	if r.repo == nil {
		return repo.DefaultCapturePrefs(), fmt.Errorf("repo not loaded")
	}
	return r.repo.LoadCapturePrefs()
}

// SetCapturePrefs replaces this vault's capture defaults.
func (r *Runtime) SetCapturePrefs(p CapturePrefs) error {
	if r.repo == nil {
		return fmt.Errorf("repo not loaded")
	}
	return r.repo.SaveCapturePrefs(p)
}

// CaptureMediaPreview is one media item as the dialog should show it.
type CaptureMediaPreview struct {
	Kind      string                 `json:"kind"` // "image" | "video"
	URL       string                 `json:"url"`
	PosterURL string                 `json:"posterUrl,omitempty"`
	EstBytes  int64                  `json:"estBytes,omitempty"`
	Variants  []capture.MediaVariant `json:"variants,omitempty"`
	// DefaultVariantID is the rung the vault's prefs would take — the
	// dialog pre-selects it so the common case is one tap.
	DefaultVariantID string `json:"defaultVariantId,omitempty"`
	// Note explains a degrade the defaults would apply (e.g. link-only).
	Note string `json:"note,omitempty"`
}

// CapturePreview is what WOULD happen, with nothing written yet.
type CapturePreview struct {
	URL       string `json:"url"`
	Platform  string `json:"platform"`
	Supported bool   `json:"supported"`
	// Blocked means a resolver claimed the URL but the platform refused
	// the server — capturing lands a pending clip for the extension to
	// complete.
	Blocked      bool   `json:"blocked"`
	BlockedError string `json:"blockedError,omitempty"`

	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Handle      string `json:"handle,omitempty"`
	Text        string `json:"text,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`

	Media []CaptureMediaPreview `json:"media,omitempty"`

	// Prefs the dialog should pre-populate from.
	Prefs CapturePrefs `json:"prefs"`
	// ShouldAsk is the vault's own answer to "is this consequential?" —
	// computed from the user's trigger thresholds, never from a rule
	// invented here. Reasons say WHICH trigger fired, so the dialog can
	// lead with what actually needs a decision.
	ShouldAsk  bool     `json:"shouldAsk"`
	AskReasons []string `json:"askReasons,omitempty"`
}

// PreviewCapture resolves a URL and reports what capturing it would do.
// It never writes: no card, no attachment, no slide.
func (r *Runtime) PreviewCapture(url string) (*CapturePreview, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	prefs, err := r.GetCapturePrefs()
	if err != nil {
		prefs = repo.DefaultCapturePrefs()
	}

	p := &CapturePreview{URL: url, Prefs: prefs}
	p.Platform = capture.Match(url)
	p.Supported = p.Platform != ""
	if !p.Supported {
		p.Title = url
		p.ShouldAsk, p.AskReasons = shouldAskFor(p, prefs)
		return p, nil
	}

	ctx, cancel := context.WithTimeout(r.ctx, captureTimeout)
	defer cancel()

	clip, err := capture.Resolve(ctx, captureHTTP, url)
	if err != nil || clip == nil {
		// The pending path — the post exists, the platform just won't
		// let this server read it.
		p.Blocked = true
		if err != nil {
			p.BlockedError = err.Error()
		}
		p.Title = url
		p.ShouldAsk, p.AskReasons = shouldAskFor(p, prefs)
		return p, nil
	}

	p.Title = captureCardTitle(clip)
	p.Author, p.Handle, p.Text, p.PublishedAt = clip.Author, clip.Handle, clip.Text, clip.PublishedAt
	for _, m := range clip.Media {
		mp := CaptureMediaPreview{
			Kind:      string(m.Kind),
			URL:       m.URL,
			PosterURL: m.PosterURL,
			EstBytes:  m.EstBytes,
			Variants:  m.Variants,
			Note:      m.Note,
		}
		if m.Kind == capture.MediaVideo {
			mp.DefaultVariantID = defaultVariantID(m, prefs)
		}
		p.Media = append(p.Media, mp)
	}
	p.ShouldAsk, p.AskReasons = shouldAskFor(p, prefs)
	return p, nil
}

// defaultVariantID applies the vault's video mode to a ladder, returning
// the rung to pre-select. Empty means "link, don't store".
func defaultVariantID(m capture.Media, prefs CapturePrefs) string {
	if len(m.Variants) == 0 {
		return ""
	}
	switch prefs.VideoMode {
	case repo.VideoModeBest:
		return m.Variants[len(m.Variants)-1].ID
	case repo.VideoModeSmallest:
		return m.Variants[0].ID
	case repo.VideoModeLink, repo.VideoModeSkip:
		return ""
	}
	// VideoModeFit: richest rung inside the budget; unknown sizes are
	// treated as fitting, with the download cap as the backstop.
	budget := int64(prefs.VideoBudgetMB) << 20
	for i := len(m.Variants) - 1; i >= 0; i-- {
		if m.Variants[i].EstBytes == 0 || m.Variants[i].EstBytes <= budget {
			return m.Variants[i].ID
		}
	}
	return "" // nothing fits → link
}

// shouldAskFor answers "should the capture dialog appear?" using ONLY the
// user's configured triggers. Harvey, 2026-08-02: "How do we define
// consequential? That would need to come from the user in a setting."
func shouldAskFor(p *CapturePreview, prefs CapturePrefs) (bool, []string) {
	switch prefs.AskMode {
	case repo.AskAlways:
		return true, nil
	case repo.AskNever:
		return false, nil
	}

	var reasons []string
	t := prefs.Triggers
	if t.UnsupportedURL && !p.Supported {
		reasons = append(reasons, "unsupported")
	}
	if t.Blocked && p.Blocked {
		reasons = append(reasons, "blocked")
	}
	images := 0
	for _, m := range p.Media {
		if m.Kind == string(capture.MediaImage) {
			images++
		}
		if m.Kind != string(capture.MediaVideo) || t.VideoOverMB <= 0 {
			continue
		}
		// The size that WOULD be stored — the pre-selected rung, or the
		// item's own estimate when there's no ladder.
		size := m.EstBytes
		for _, v := range m.Variants {
			if v.ID == m.DefaultVariantID {
				size = v.EstBytes
			}
		}
		if size > int64(t.VideoOverMB)<<20 {
			reasons = append(reasons, "video_large")
		}
	}
	if t.GalleryOverCount > 0 && images > t.GalleryOverCount {
		reasons = append(reasons, "gallery_large")
	}
	return len(reasons) > 0, reasons
}
