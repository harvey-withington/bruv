package supervisor

// The vault's capture prefs must reach ingest, not just applyCaptureChoices
// (pre-release sweep 2026-09-29, §9.5/9.6): image mode "link" used to drop
// every image, a capture with no explicit variant ignored the vault's
// video mode and budget, and RetryCapture ignored the prefs entirely.

import (
	"strings"
	"testing"

	"bruv/internal/repo"
)

// tweetVideoJSON: one 60-second video offered at ~1.8 MB and ~16 MB. The
// resolver's own 48 MB budget picks the 720p rung.
const tweetVideoJSON = `{
  "user": {"name": "Some One", "screen_name": "someone"},
  "text": "A clip.",
  "created_at": "2026-07-30T12:00:00.000Z",
  "mediaDetails": [{"type": "video", "media_url_https": "https://pbs.twimg.com/media/poster.jpg",
    "video_info": {"duration_millis": 60000, "variants": [
      {"content_type": "video/mp4", "bitrate": 256000, "url": "https://video.twimg.com/vid/480x270/small.mp4"},
      {"content_type": "video/mp4", "bitrate": 2176000, "url": "https://video.twimg.com/vid/1280x720/big.mp4"}
    ]}}]
}`

func videoRoutes() captureRoutes {
	return captureRoutes{
		"cdn.syndication.twimg.com /tweet-result": {200, tweetVideoJSON},
		"video.twimg.com /vid/480x270/small.mp4":  {200, "small-video-bytes"},
		"video.twimg.com /vid/1280x720/big.mp4":   {200, "big-video-bytes"},
	}
}

func setPrefs(t *testing.T, rt *Runtime, edit func(*CapturePrefs)) {
	t.Helper()
	p := repo.DefaultCapturePrefs()
	edit(&p)
	if err := rt.SetCapturePrefs(p); err != nil {
		t.Fatal(err)
	}
}

// mediaURL is the url a media/image block points at (first item).
func mediaURL(t *testing.T, v any) string {
	t.Helper()
	switch m := v.(type) {
	case map[string]any:
		s, _ := m["url"].(string)
		return s
	case []any:
		if len(m) > 0 {
			if item, ok := m[0].(map[string]any); ok {
				s, _ := item["url"].(string)
				return s
			}
		}
	}
	return ""
}

func TestCaptureImageModeLinkKeepsImagesAsLinks(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, successRoutes())
	setPrefs(t, rt, func(p *CapturePrefs) { p.ImageMode = repo.ImageModeLink })

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil {
		t.Fatal(err)
	}
	card, _ := rt.GetCard(res.CardID)
	if got := mediaURL(t, blocksByKey(card)["media"].Value); !strings.HasPrefix(got, "https://pbs.twimg.com/media/pic.jpg") {
		t.Errorf("media block = %q, want the linked platform image", got)
	}
	for _, a := range card.FileAttachments {
		if !strings.Contains(a.Name, "avatar") {
			t.Errorf("image stored as attachment %q in link mode", a.Name)
		}
	}
}

func TestCaptureHonoursVaultVideoMode(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, videoRoutes())
	setPrefs(t, rt, func(p *CapturePrefs) { p.VideoMode = repo.VideoModeSmallest })

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil {
		t.Fatal(err)
	}
	card, _ := rt.GetCard(res.CardID)
	ref := mediaURL(t, blocksByKey(card)["video"].Value)
	if !strings.HasPrefix(ref, "attachment:") {
		t.Fatalf("video block = %q, want a stored attachment", ref)
	}
	attID := ref[strings.LastIndex(ref, "/")+1:]
	data, _, err := rt.ReadCardAttachment(card.ID, attID)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "small-video-bytes" {
		t.Errorf("stored %q, want the smallest rung the vault's video mode asks for", data)
	}
}

func TestCaptureHonoursVaultVideoBudget(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, videoRoutes())
	setPrefs(t, rt, func(p *CapturePrefs) { p.VideoMode, p.VideoBudgetMB = repo.VideoModeFit, 1 })

	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil {
		t.Fatal(err)
	}
	card, _ := rt.GetCard(res.CardID)
	if got := mediaURL(t, blocksByKey(card)["video"].Value); got != "https://video.twimg.com/vid/480x270/small.mp4" {
		t.Errorf("video block = %q, want the smallest rung kept as a link (nothing fits 1 MB)", got)
	}
	if len(res.MediaNotes) == 0 {
		t.Error("the user must be told why the video wasn't stored")
	}
}

func TestRetryCaptureAppliesVaultPrefs(t *testing.T) {
	rt := newTestRuntime(t)
	swapCaptureHTTP(t, failRoutes())
	res, err := rt.CaptureFromURL(tweetURL, CaptureOpts{})
	if err != nil || !res.Pending {
		t.Fatalf("pending capture failed: %v %+v", err, res)
	}
	setPrefs(t, rt, func(p *CapturePrefs) { p.ImageMode = repo.ImageModeSkip })

	swapCaptureHTTP(t, successRoutes())
	if _, err := rt.RetryCapture(res.CardID); err != nil {
		t.Fatal(err)
	}
	card, _ := rt.GetCard(res.CardID)
	if got := mediaURL(t, blocksByKey(card)["media"].Value); got != "" {
		t.Errorf("media block = %q, want no image (vault image mode skip)", got)
	}
	for _, a := range card.FileAttachments {
		if !strings.Contains(a.Name, "avatar") {
			t.Errorf("retry stored image %q despite image mode skip", a.Name)
		}
	}
}
