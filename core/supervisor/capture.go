package supervisor

// Server-side clip ingest — the Go port of the extension's pipeline
// (clipper/src/lib/clip.ts executeJob), plus the pending-clip ladder the
// extension never needed. Four RPCs:
//
//   MatchCaptureURL  — cheap preflight: platform id or "" (mobile UI
//                      branches clip-vs-plain mode on this).
//   CaptureFromURL   — resolve + ingest. A resolver failure DEGRADES,
//                      never fails: the card, url block, pin, and (when a
//                      deck target was chosen) a live-bound slide are all
//                      created immediately; the card is tagged
//                      clip-pending and completed later from a real
//                      browser. Slides upgrade in place through their
//                      bindings — completion never re-appends.
//   RetryCapture     — server-side re-resolve into an existing pending
//                      card (the desktop banner's Retry button; transient
//                      flakiness resolves without the extension).
//   CompleteCapture  — extension-supplied ClipResult + downloaded media
//                      into an existing pending card (the popup's
//                      "Complete capture"). Deliberately tool-shaped.
//
// Ingest into an EXISTING card fills blocks in place matched by schema
// key — block IDs must survive completion because live slide bindings
// point at them. That invariant has a dedicated test.

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"bruv/core/capture"
	"bruv/internal/config"
	"bruv/internal/model"
	inotify "bruv/internal/notify"
)

const (
	// ClipPendingTag marks a card whose capture degraded to link-only and
	// awaits desktop completion. User-visible as a normal chip (searchable
	// everywhere for free); the extension polls it for its badge/list.
	ClipPendingTag = "clip-pending"

	// pinWithDeck mirrors the clipper's PIN_WITH_DECK sentinel: pin the
	// clipped card wherever the deck-target card is pinned.
	pinWithDeck = "__deck__"

	socialPostTypeLabel = "Social Post"

	captureTimeout = 60 * time.Second
)

// captureHTTP is the outbound client for resolvers + media downloads.
// Package-level so supervisor tests can swap in a fixture transport.
var captureHTTP = capture.NewClient()

// CaptureOpts is the options envelope for CaptureFromURL. CategoryID ""
// = Inbox (unpinned); "__deck__" = mirror the deck-target card's pins.
type CaptureOpts struct {
	IncludeInDeck bool   `json:"includeInDeck"`
	DeckCardID    string `json:"deckCardID"`
	DeckBlockID   string `json:"deckBlockID"`
	CategoryID    string `json:"categoryID"`
}

// CaptureResult mirrors the clipper's ClipOutcome, widened with the
// pending flag the mobile outcome panel branches on and honest pin
// reporting: a pin the user asked for that bounced (accepted-types gate,
// category from another vault, deleted category) leaves the card in the
// Inbox — recoverable, but the UI must SAY so, not celebrate. Found the
// hard way on 2026-07-31: "Pin with deck" into a livestream-only
// category failed silently on every surface.
type CaptureResult struct {
	CardID        string `json:"cardId"`
	SlideAppended bool   `json:"slideAppended"`
	Platform      string `json:"platform"`
	Pending       bool   `json:"pending"`
	// PinFailed is true when a pin destination was requested but no pin
	// landed; PinError carries the first rejection so the user learns WHY
	// (e.g. "category only accepts: livestream").
	PinFailed bool   `json:"pinFailed,omitempty"`
	PinError  string `json:"pinError,omitempty"`
	// MediaNotes explain media that landed in a degraded form — an
	// oversized video kept as a platform link rather than stored. The
	// card is still useful; the user just needs to know the video isn't
	// theirs to keep.
	MediaNotes []string `json:"mediaNotes,omitempty"`
}

// CompleteMedia is one already-downloaded media item posted by the
// extension — the same wire shape as the clipper's ClipJob media.
type CompleteMedia struct {
	Name   string `json:"name"`
	Base64 string `json:"base64"`
	Kind   string `json:"kind"`
}

// MatchCaptureURL returns the platform id claiming url, or "".
func (r *Runtime) MatchCaptureURL(url string) (string, error) {
	return capture.Match(strings.TrimSpace(url)), nil
}

// CaptureFromURL resolves a post URL and ingests it as a Social Post
// card (+ optional deck slide). Resolver failure degrades to a pending
// clip — see the file header for the ladder.
func (r *Runtime) CaptureFromURL(rawURL string, opts CaptureOpts) (*CaptureResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	platform := capture.Match(rawURL)
	if platform == "" {
		return nil, fmt.Errorf("no capture support for %s", rawURL)
	}

	ctx, cancel := context.WithTimeout(r.ctx, captureTimeout)
	defer cancel()

	clip, err := capture.Resolve(ctx, captureHTTP, rawURL)
	if err != nil {
		slog.Warn("capture: resolve failed, degrading to pending clip", "platform", platform, "url", rawURL, "err", err)
		pending := &capture.Clip{
			Platform:     platform,
			CanonicalURL: rawURL,
			EmbedVideo:   capture.EmbedForURL(rawURL),
		}
		res, ingErr := r.ingestClip(ctx, "", pending, nil, nil, opts, true)
		if ingErr != nil {
			return nil, ingErr
		}
		if card, err := r.Card.Get(res.CardID); err == nil {
			r.notifyPendingClip(card)
		}
		return res, nil
	}

	media, avatar := downloadClipMedia(ctx, clip)
	return r.ingestClip(ctx, "", clip, media, avatar, opts, false)
}

// RetryCapture re-resolves an existing (typically pending) clip card
// server-side, filling its blocks in place. Errors are honest here — the
// card already holds the pending state, so there is nothing to degrade to.
func (r *Runtime) RetryCapture(cardID string) (*CaptureResult, error) {
	card, err := r.Card.Get(cardID)
	if err != nil {
		return nil, err
	}
	rawURL := cardSourceURL(card)
	if rawURL == "" {
		return nil, fmt.Errorf("card %s has no source url block", cardID)
	}

	ctx, cancel := context.WithTimeout(r.ctx, captureTimeout)
	defer cancel()

	clip, err := capture.Resolve(ctx, captureHTTP, rawURL)
	if err != nil {
		return nil, fmt.Errorf("capture retry: %w", err)
	}
	if clip == nil {
		return nil, fmt.Errorf("no capture support for %s", rawURL)
	}
	media, avatar := downloadClipMedia(ctx, clip)
	return r.ingestClip(ctx, cardID, clip, media, avatar, CaptureOpts{}, false)
}

// CompleteCapture ingests an extension-captured clip (media already
// downloaded in the authenticated browser) into an existing pending card.
func (r *Runtime) CompleteCapture(cardID string, clip capture.Clip, media []CompleteMedia) (*CaptureResult, error) {
	if cardID == "" {
		return nil, fmt.Errorf("cardID is required")
	}
	if clip.Platform == "" {
		return nil, fmt.Errorf("clip.platform is required")
	}
	ims := make([]ingestMedia, 0, len(media))
	for _, m := range media {
		if m.Name == "" || m.Base64 == "" {
			continue
		}
		kind := capture.MediaKind(m.Kind)
		if kind != capture.MediaVideo {
			kind = capture.MediaImage
		}
		ims = append(ims, ingestMedia{Name: m.Name, Base64: m.Base64, Kind: kind})
	}

	ctx, cancel := context.WithTimeout(r.ctx, captureTimeout)
	defer cancel()
	return r.ingestClip(ctx, cardID, &clip, ims, nil, CaptureOpts{}, false)
}

// --- ingest ----------------------------------------------------------------

type ingestMedia struct {
	Name   string
	Base64 string
	Kind   capture.MediaKind
	// RemoteURL is set instead of Base64 when the media is kept as a link
	// rather than stored (oversized video). The block points at the
	// platform URL: it plays now, and may rot later — which is why it
	// comes with a Note the user actually sees.
	RemoteURL string
	Note      string
}

// ingestClip is the platform-blind pipeline: card → attachments → blocks
// + bindings → tags → pin → deck slide. existingID == "" creates a card;
// otherwise blocks are filled IN PLACE by schema key (IDs survive — live
// slide bindings depend on it) and pins/deck are left alone (they were
// established at capture time).
func (r *Runtime) ingestClip(ctx context.Context, existingID string, clip *capture.Clip, media []ingestMedia, avatar *ingestMedia, opts CaptureOpts, pending bool) (*CaptureResult, error) {
	completing := existingID != ""

	var card *model.Card
	var err error
	if completing {
		card, err = r.Card.Get(existingID)
	} else {
		// Failure to provision the type degrades to an untyped card —
		// never blocks a clip (clipper parity).
		card, err = r.Card.Create(r.ensureSocialPostType(), captureCardTitle(clip))
	}
	if err != nil {
		return nil, err
	}
	cardID := card.ID

	// Attachments first — the blocks reference them. Attachment IDs are
	// recovered by diffing the returned card (names may be de-duplicated
	// server-side, so name-matching alone isn't reliable — clipper rule).
	known := make(map[string]bool, len(card.FileAttachments))
	for _, a := range card.FileAttachments {
		known[a.ID] = true
	}
	addAttachment := func(m *ingestMedia) string {
		updated, err := r.Card.AddAttachment(cardID, m.Name, m.Base64)
		if err != nil {
			slog.Warn("capture: attachment failed", "card", cardID, "name", m.Name, "err", err)
			return ""
		}
		card = updated
		for _, a := range updated.FileAttachments {
			if !known[a.ID] {
				known[a.ID] = true
				return fmt.Sprintf("attachment:%s/%s", cardID, a.ID)
			}
		}
		return ""
	}

	var imageRefs []string
	firstVideoRef := ""
	mediaNotes := []string{}
	for i := range media {
		// Link-only media (oversized video) skips attachment storage: the
		// block points straight at the platform URL.
		if media[i].RemoteURL != "" {
			if media[i].Kind == capture.MediaVideo && firstVideoRef == "" {
				firstVideoRef = media[i].RemoteURL
			}
			if media[i].Note != "" {
				mediaNotes = append(mediaNotes, media[i].Note)
			}
			continue
		}
		ref := addAttachment(&media[i])
		if ref == "" {
			continue
		}
		if media[i].Kind == capture.MediaVideo {
			if firstVideoRef == "" {
				firstVideoRef = ref
			}
		} else {
			imageRefs = append(imageRefs, ref)
		}
	}
	avatarRef := ""
	if avatar != nil {
		avatarRef = addAttachment(avatar)
	}

	// One spec per captured field, keyed with the schema keys (the Social
	// Post template's / slideFieldTypes["post"]'s keys). A pending clip
	// gets the FULL set with empty values — stable block IDs are what let
	// the pre-bound slide upgrade in place at completion.
	type blockSpec struct {
		key, btype, label string
		value             any
	}
	var specs []blockSpec
	add := func(key, btype, label string, value any) {
		specs = append(specs, blockSpec{key, btype, label, value})
	}

	if pending {
		add("author", "text", "Author", "")
		add("handle", "text", "Handle", "")
		add("avatar", "image", "Avatar", map[string]any{"url": ""})
		add("text", "text", "Text", "")
		add("media", "image", "Media", map[string]any{"url": ""})
		add("video", "media", "Video", []any{})
		add("date", "text", "Date", displayDate(clip.PublishedAt))
		add("url", "url", "Source", map[string]any{"url": clip.CanonicalURL})
	} else {
		if clip.Author != "" {
			add("author", "text", "Author", clip.Author)
		}
		if clip.Handle != "" {
			add("handle", "text", "Handle", clip.Handle)
		}
		if avatarRef != "" {
			add("avatar", "image", "Avatar", map[string]any{"url": avatarRef})
		}
		if clip.Text != "" {
			add("text", "text", "Text", clip.Text)
		}
		if len(imageRefs) > 1 {
			// Gallery: one multi-item media block; renderers show a carousel.
			items := make([]any, 0, len(imageRefs))
			for _, ref := range imageRefs {
				items = append(items, map[string]any{"id": newShortID("m"), "url": ref})
			}
			add("media", "media", "Media", items)
		} else if len(imageRefs) == 1 {
			add("media", "image", "Media", map[string]any{"url": imageRefs[0]})
		}
		if firstVideoRef != "" {
			add("video", "media", "Video", []any{map[string]any{"id": newShortID("m"), "url": firstVideoRef, "mime": "video/mp4"}})
		} else if clip.EmbedVideo != nil {
			// The card carries the embed reference so future slides (and
			// completions of already-appended ones, whose values are
			// frozen) can resolve playback through the binding.
			add("video", "media", "Video", []any{map[string]any{
				"id":  newShortID("m"),
				"url": fmt.Sprintf("embed://%s/%s", clip.EmbedVideo.Provider, clip.EmbedVideo.ID),
			}})
		}
		if d := displayDate(clip.PublishedAt); d != "" {
			add("date", "text", "Date", d)
		}
		add("url", "url", "Source", map[string]any{"url": clip.CanonicalURL})
	}

	bindings := map[string]any{}
	var blocks []model.Block
	if completing {
		// Fill in place by schema key; untouched keys and any user-added
		// blocks stay exactly as they are.
		blocks = card.Blocks
		byKey := map[string]int{}
		for i, b := range blocks {
			if b.Key != "" {
				byKey[b.Key] = i
			}
		}
		for _, s := range specs {
			if i, ok := byKey[s.key]; ok {
				blocks[i].Type = s.btype
				blocks[i].Label = s.label
				blocks[i].Value = s.value
				bindings[s.key] = blocks[i].ID
			} else {
				id := newShortID("blk")
				blocks = append(blocks, model.Block{ID: id, Type: s.btype, Label: s.label, Key: s.key, Value: s.value})
				bindings[s.key] = id
			}
		}
	} else {
		// Fresh card: our blocks replace any template-applied ones
		// wholesale — ours carry the data (clipper rule).
		for _, s := range specs {
			id := newShortID("blk")
			blocks = append(blocks, model.Block{ID: id, Type: s.btype, Label: s.label, Key: s.key, Value: s.value})
			bindings[s.key] = id
		}
	}
	if card, err = r.Card.UpdateBlocks(cardID, blocks); err != nil {
		return nil, fmt.Errorf("capture: update blocks: %w", err)
	}

	// Completion un-freezes the machine-generated pending title ("<platform>:
	// <url>") but never touches a human rename.
	if completing && clip.Text != "" && looksMachineTitle(card.Title, clip.Platform) {
		if updated, err := r.Card.UpdateTitle(cardID, captureCardTitle(clip)); err == nil {
			card = updated
		} else {
			slog.Warn("capture: title update failed", "card", cardID, "err", err)
		}
	}

	// Tags: platform always; clip-pending added on the pending rung,
	// cleared by any successful completion/retry. Best-effort.
	tags := make([]string, 0, len(card.Tags)+2)
	for _, t := range card.Tags {
		if t != ClipPendingTag {
			tags = append(tags, t)
		}
	}
	if clip.Platform != "" && !slices.Contains(tags, clip.Platform) {
		tags = append(tags, clip.Platform)
	}
	if pending {
		tags = append(tags, ClipPendingTag)
	}
	if updated, err := r.Card.UpdateTags(cardID, tags); err == nil {
		card = updated
	} else {
		slog.Warn("capture: tags failed", "card", cardID, "err", err)
	}

	// Pinning is best-effort and capture-time only: a failed pin leaves
	// the card in the Inbox (visible, recoverable) — but the failure is
	// REPORTED, never swallowed: the user chose a destination.
	pinRequested := false
	pinLanded := false
	pinError := ""
	notePinErr := func(err error) {
		if pinError == "" && err != nil {
			pinError = err.Error()
		}
	}
	if !completing {
		switch {
		case opts.CategoryID == pinWithDeck && opts.DeckCardID != "":
			pinRequested = true
			if pins, err := r.Card.GetPins(opts.DeckCardID); err == nil {
				for _, p := range pins {
					if p.CategoryID == "" {
						continue
					}
					if err := r.Card.Pin(cardID, p.CategoryID); err != nil {
						slog.Warn("capture: pin to deck location failed", "card", cardID, "category", p.CategoryID, "err", err)
						notePinErr(err)
					} else {
						pinLanded = true
					}
				}
				if len(pins) == 0 {
					// Mirroring an unpinned deck mirrors nothing — that's
					// Inbox by definition, and worth saying.
					notePinErr(fmt.Errorf("deck card has no pins to mirror"))
				}
			} else {
				slog.Warn("capture: resolve deck pins failed", "deckCard", opts.DeckCardID, "err", err)
				notePinErr(err)
			}
		case opts.CategoryID != "" && opts.CategoryID != pinWithDeck:
			pinRequested = true
			if err := r.Card.Pin(cardID, opts.CategoryID); err != nil {
				slog.Warn("capture: pin failed", "card", cardID, "category", opts.CategoryID, "err", err)
				notePinErr(err)
			} else {
				pinLanded = true
			}
		}
	}

	// Deck slide — capture-time only; completion upgrades the existing
	// slide through its live bindings instead of appending a duplicate.
	// Every field binds LIVE to the card's blocks; `platform` is the only
	// literal (routing data, not content). templateId 'auto': BRUV
	// resolves the template from the bound url at render time.
	slideAppended := false
	if !completing && opts.IncludeInDeck && opts.DeckCardID != "" && opts.DeckBlockID != "" {
		values := map[string]any{"platform": clip.Platform}
		if clip.EmbedVideo != nil {
			values["video"] = fmt.Sprintf("embed://%s/%s", clip.EmbedVideo.Provider, clip.EmbedVideo.ID)
		}
		slide := map[string]any{
			"contentTypeId": "post",
			"templateId":    "auto",
			"cardId":        cardID,
			"values":        values,
			"bindings":      bindings,
		}
		if _, err := r.AppendDeckSlide(opts.DeckCardID, opts.DeckBlockID, slide); err != nil {
			// The card landed; failing the whole capture here would push
			// the user into re-capturing (duplicate card). The result's
			// slideAppended=false lets the UI say "saved, deck append
			// failed" honestly.
			slog.Warn("capture: deck append failed", "card", cardID, "deckCard", opts.DeckCardID, "err", err)
		} else {
			slideAppended = true
		}
	}

	res := &CaptureResult{
		CardID:        cardID,
		SlideAppended: slideAppended,
		Platform:      clip.Platform,
		Pending:       pending,
		MediaNotes:    mediaNotes,
	}
	if pinRequested && !pinLanded {
		res.PinFailed = true
		res.PinError = pinError
	}
	return res, nil
}

// --- Social Post type provisioning (clipper parity) ------------------------

// socialPostTemplateBlocks mirrors clipper/src/lib/clip.ts
// SOCIAL_POST_TEMPLATE_BLOCKS. NOTE: this is the 5th mirror of the post
// field schema (shared/slideContentTypes.ts, the two Go maps in
// present.go, the clipper's template, and this) — keep all five in sync.
// The clipper's copy retires when its pipeline moves onto CompleteCapture
// (planned unification).
func socialPostTemplateBlocks() []model.Block {
	return []model.Block{
		{ID: "tpl-author", Type: "text", Label: "Author", Key: "author", Value: ""},
		{ID: "tpl-handle", Type: "text", Label: "Handle", Key: "handle", Value: ""},
		{ID: "tpl-avatar", Type: "image", Label: "Avatar", Key: "avatar", Value: map[string]any{"url": ""}},
		{ID: "tpl-text", Type: "text", Label: "Text", Key: "text", Value: ""},
		{ID: "tpl-media", Type: "image", Label: "Media", Key: "media", Value: map[string]any{"url": ""}},
		{ID: "tpl-video", Type: "media", Label: "Video", Key: "video", Value: []any{}},
		{ID: "tpl-date", Type: "text", Label: "Date", Key: "date", Value: ""},
		{ID: "tpl-url", Type: "url", Label: "Source", Key: "url", Value: map[string]any{"url": ""}},
	}
}

// ensureSocialPostType returns the type ID, creating template + type on
// first use. Failure degrades to an untyped card — never blocks a clip.
func (r *Runtime) ensureSocialPostType() string {
	for _, t := range r.ListCardTypes() {
		if t.Label == socialPostTypeLabel {
			return t.ID
		}
	}
	tpl, err := r.CreateCardTemplate(socialPostTypeLabel, socialPostTemplateBlocks())
	if err != nil {
		slog.Warn("capture: create Social Post template failed (clipping untyped)", "err", err)
		return ""
	}
	created, err := r.CreateUserCardType(socialPostTypeLabel, "#1d9bf0", "A captured social post (web clipper)", "", tpl.ID)
	if err != nil {
		slog.Warn("capture: create Social Post type failed (clipping untyped)", "err", err)
		return ""
	}
	return created.ID
}

// --- helpers ----------------------------------------------------------------

// downloadClipMedia fetches every media item + the avatar so the ingest
// is fully self-contained — attachments are the durable home, never
// remote links (they rot). Individual failures drop that item, never the
// clip; an unfetchable video degrades to its poster (plugin parity).
func downloadClipMedia(ctx context.Context, clip *capture.Clip) ([]ingestMedia, *ingestMedia) {
	var out []ingestMedia
	n := 0
	push := func(data []byte, mime string, kind capture.MediaKind) {
		n++
		out = append(out, ingestMedia{
			Name:   fmt.Sprintf("%s-%d.%s", clip.Platform, n, extFromMime(mime, kind)),
			Base64: base64.StdEncoding.EncodeToString(data),
			Kind:   kind,
		})
	}
	for _, m := range clip.Media {
		if m.URL == "" {
			continue
		}
		// Known-oversized: don't even try to download it.
		if m.LinkOnly {
			out = append(out, ingestMedia{Kind: m.Kind, RemoteURL: m.URL, Note: m.Note})
			continue
		}
		data, mime, err := captureHTTP.Download(ctx, m.URL)
		if err != nil {
			slog.Warn("capture: media download failed", "url", m.URL, "err", err)
			if m.Kind == capture.MediaVideo {
				// A video that wouldn't download used to be replaced by
				// its poster image, so the card showed a thumbnail and
				// nothing said the video was missing (Harvey, 2026-08-02:
				// "it doesn't seem to make it to the capture card at
				// all"). Keep it as a link instead — the slide plays —
				// and say what happened.
				out = append(out, ingestMedia{
					Kind:      m.Kind,
					RemoteURL: m.URL,
					Note:      "Video couldn't be stored in the card, so it's linked to the platform — it will stop working if the platform removes it.",
				})
			}
			continue
		}
		push(data, mime, m.Kind)
	}

	var avatar *ingestMedia
	if clip.AvatarURL != "" {
		// Avatar is decoration — never blocks a clip.
		if data, mime, err := captureHTTP.Download(ctx, clip.AvatarURL); err == nil {
			avatar = &ingestMedia{
				Name:   fmt.Sprintf("%s-avatar.%s", clip.Platform, extFromMime(mime, capture.MediaImage)),
				Base64: base64.StdEncoding.EncodeToString(data),
				Kind:   capture.MediaImage,
			}
		}
	}
	return out, avatar
}

// extFromMime mirrors clip.ts extFromMime.
func extFromMime(mime string, kind capture.MediaKind) string {
	switch {
	case strings.Contains(mime, "png"):
		return "png"
	case strings.Contains(mime, "gif"):
		return "gif"
	case strings.Contains(mime, "webp"):
		return "webp"
	case strings.Contains(mime, "mp4"):
		return "mp4"
	case strings.Contains(mime, "webm"):
		return "webm"
	}
	if kind == capture.MediaVideo {
		return "mp4"
	}
	return "jpg"
}

// captureCardTitle mirrors clip.ts cardTitle, with the canonical URL as
// the text fallback so pending clips read "<platform>: <url>".
func captureCardTitle(clip *capture.Clip) string {
	who := clip.Handle
	if who == "" {
		who = clip.Author
	}
	if who == "" {
		who = clip.Platform
	}
	text := strings.Join(strings.Fields(clip.Text), " ")
	if text == "" {
		text = clip.CanonicalURL
	}
	if text == "" {
		return who
	}
	if runes := []rune(text); len(runes) > 60 {
		text = string(runes[:60]) + "…"
	}
	return who + ": " + text
}

// looksMachineTitle reports whether title still has the generated
// pending shape ("<platform>: http…") — the only case completion is
// allowed to overwrite.
func looksMachineTitle(title, platform string) bool {
	return strings.HasPrefix(title, platform+": http")
}

// cardSourceURL pulls the url out of the card's source block (schema key
// "url").
func cardSourceURL(card *model.Card) string {
	for _, b := range card.Blocks {
		if b.Key != "url" {
			continue
		}
		if v, ok := b.Value.(map[string]any); ok {
			if u, ok := v["url"].(string); ok {
				return strings.TrimSpace(u)
			}
		}
	}
	return ""
}

// displayDate formats an ISO timestamp the way the clipper did ("Jan 2,
// 2026"); empty/unparseable in → empty out.
func displayDate(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	return t.Format("Jan 2, 2006")
}

func newShortID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

// notifyPendingClip lands the desktop nudge in the in-app notification
// feed (both surfaces read the same store; desktop's tray/tooltip rides
// the notification:new event).
func (r *Runtime) notifyPendingClip(card *model.Card) {
	cfg, err := config.LoadNotifyConfig()
	if err != nil {
		cfg = config.DefaultNotifyConfig()
	}
	d := inotify.NewDispatcher(cfg, func(name string, data any) { r.bus.Publish(name, data) })
	d.Send(inotify.Request{
		Title:     "Clip waiting for desktop capture",
		Body:      card.Title,
		Source:    "capture",
		CardID:    card.ID,
		CardTitle: card.Title,
		Channels:  inotify.ParseChannels("in-app"),
	})
}
