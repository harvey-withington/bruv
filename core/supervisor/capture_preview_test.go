package supervisor

// The capture dialog's decision logic. Harvey's ruling (2026-08-02) was
// that BRUV must stop choosing for the user, so these tests are mostly
// about NOT deciding: the ladder is reported whole, the defaults are the
// vault's, and "is this consequential enough to ask?" is answered only by
// the user's own thresholds.

import (
	"testing"

	"bruv/core/capture"
	"bruv/internal/repo"
)

// The real ladder from the 44-minute tweet that started this.
func ladder() []capture.MediaVariant {
	return []capture.MediaVariant{
		{ID: "480x270", Label: "480×270", URL: "https://v/480.mp4", Bitrate: 256000, EstBytes: 85 << 20},
		{ID: "640x360", Label: "640×360", URL: "https://v/640.mp4", Bitrate: 832000, EstBytes: 277 << 20},
		{ID: "1280x720", Label: "1280×720", URL: "https://v/720.mp4", Bitrate: 2176000, EstBytes: 725 << 20},
		{ID: "1920x1080", Label: "1920×1080", URL: "https://v/1080.mp4", Bitrate: 10368000, EstBytes: 3500 << 20},
	}
}

func videoMedia() capture.Media {
	return capture.Media{Kind: capture.MediaVideo, URL: "https://v/1080.mp4", Variants: ladder()}
}

func TestDefaultVariantIDPerVideoMode(t *testing.T) {
	cases := []struct {
		mode     string
		budgetMB int
		want     string
	}{
		{repo.VideoModeBest, 50, "1920x1080"},   // size is no object
		{repo.VideoModeSmallest, 50, "480x270"}, // always cheapest
		{repo.VideoModeLink, 50, ""},            // never store
		{repo.VideoModeSkip, 50, ""},            // no video at all
		{repo.VideoModeFit, 50, ""},             // nothing fits 50 MB → link
		{repo.VideoModeFit, 100, "480x270"},     // only the cheapest fits
		{repo.VideoModeFit, 300, "640x360"},     // steps up with the budget
		{repo.VideoModeFit, 4000, "1920x1080"},  // everything fits
	}
	for _, c := range cases {
		prefs := repo.DefaultCapturePrefs()
		prefs.VideoMode, prefs.VideoBudgetMB = c.mode, c.budgetMB
		if got := defaultVariantID(videoMedia(), prefs); got != c.want {
			t.Errorf("mode %s budget %dMB: got %q, want %q", c.mode, c.budgetMB, got, c.want)
		}
	}
}

func TestDefaultVariantIDUnknownSizesFit(t *testing.T) {
	// No duration reported → no estimate. Treat as fitting and let the
	// download cap be the backstop, rather than refusing to store video.
	m := capture.Media{Kind: capture.MediaVideo, Variants: []capture.MediaVariant{
		{ID: "a", Bitrate: 100}, {ID: "b", Bitrate: 200},
	}}
	prefs := repo.DefaultCapturePrefs()
	if got := defaultVariantID(m, prefs); got != "b" {
		t.Errorf("got %q, want the richest rung when sizes are unknown", got)
	}
}

// "Consequential" is the USER's definition — these assert the triggers do
// what they're configured to do, and nothing more.
func TestShouldAskUsesOnlyUserTriggers(t *testing.T) {
	bigVideo := &CapturePreview{
		Supported: true,
		Media: []CaptureMediaPreview{{
			Kind: string(capture.MediaVideo), Variants: ladder(), DefaultVariantID: "1280x720",
		}},
	}

	t.Run("always asks", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.AskMode = repo.AskAlways
		if ask, _ := shouldAskFor(&CapturePreview{Supported: true}, p); !ask {
			t.Error("AskAlways must ask even for a trivial capture")
		}
	})

	t.Run("never asks", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.AskMode = repo.AskNever
		if ask, _ := shouldAskFor(bigVideo, p); ask {
			t.Error("AskNever must not ask even for a 725 MB video")
		}
	})

	t.Run("video threshold fires", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.Triggers.VideoOverMB = 50
		ask, reasons := shouldAskFor(bigVideo, p)
		if !ask || !hasReason(reasons, "video_large") {
			t.Errorf("725 MB should trip a 50 MB trigger; got %v %v", ask, reasons)
		}
	})

	t.Run("video threshold respected when raised", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.Triggers.VideoOverMB = 1000 // user is happy with big videos
		if ask, _ := shouldAskFor(bigVideo, p); ask {
			t.Error("a 725 MB video must not ask when the user's threshold is 1 GB")
		}
	})

	t.Run("video trigger off", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.Triggers.VideoOverMB = 0
		if ask, _ := shouldAskFor(bigVideo, p); ask {
			t.Error("a zero threshold means the trigger is off")
		}
	})

	t.Run("gallery threshold", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.Triggers.GalleryOverCount = 3
		gallery := &CapturePreview{Supported: true}
		for i := 0; i < 5; i++ {
			gallery.Media = append(gallery.Media, CaptureMediaPreview{Kind: string(capture.MediaImage)})
		}
		ask, reasons := shouldAskFor(gallery, p)
		if !ask || !hasReason(reasons, "gallery_large") {
			t.Errorf("5 images should trip a 3-image trigger; got %v %v", ask, reasons)
		}
	})

	t.Run("unsupported and blocked", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		if ask, reasons := shouldAskFor(&CapturePreview{Supported: false}, p); !ask || !hasReason(reasons, "unsupported") {
			t.Errorf("unsupported URL should ask by default; got %v %v", ask, reasons)
		}
		if ask, reasons := shouldAskFor(&CapturePreview{Supported: true, Blocked: true}, p); !ask || !hasReason(reasons, "blocked") {
			t.Errorf("a blocked platform should ask by default; got %v %v", ask, reasons)
		}
		p.Triggers.UnsupportedURL, p.Triggers.Blocked = false, false
		if ask, _ := shouldAskFor(&CapturePreview{Supported: false}, p); ask {
			t.Error("triggers turned off must not ask")
		}
	})

	// The trap: when NOTHING fits the budget, no rung is pre-selected, so
	// "the size that would be stored" is nothing — and a naive reading
	// makes the biggest videos the ones that never prompt. Found by the
	// extension port on 2026-08-02; this pins that the server can't
	// regress into it. The item's own EstBytes (the smallest rung) is the
	// fallback the code relies on.
	t.Run("oversized video with no pre-selected rung still asks", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.Triggers.VideoOverMB = 50
		nothingFits := &CapturePreview{
			Supported: true,
			Media: []CaptureMediaPreview{{
				Kind:             string(capture.MediaVideo),
				Variants:         ladder(),
				DefaultVariantID: "", // budget too small for every rung
				EstBytes:         85 << 20,
			}},
		}
		ask, reasons := shouldAskFor(nothingFits, p)
		if !ask || !hasReason(reasons, "video_large") {
			t.Errorf("a video too big for ANY rung must still ask; got %v %v", ask, reasons)
		}
	})

	t.Run("ordinary capture is silent", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		plain := &CapturePreview{Supported: true, Media: []CaptureMediaPreview{
			{Kind: string(capture.MediaImage), EstBytes: 2 << 20},
		}}
		if ask, reasons := shouldAskFor(plain, p); ask {
			t.Errorf("a normal capture must stay one tap; got reasons %v", reasons)
		}
	})
}

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// The user's explicit choice must beat every default, including the
// size-based link-only fallback: if they pick the 3.5 GB rung, they get it.
func TestApplyCaptureChoices(t *testing.T) {
	prefs := repo.DefaultCapturePrefs()

	t.Run("explicit variant overrides link-only default", func(t *testing.T) {
		clip := &capture.Clip{Media: []capture.Media{{
			Kind: capture.MediaVideo, URL: "https://v/480.mp4", Variants: ladder(),
			LinkOnly: true, Note: "too big",
		}}}
		applyCaptureChoices(clip, CaptureOpts{VideoVariantID: "1920x1080"}, prefs)
		got := clip.Media[0]
		if got.URL != "https://v/1080.mp4" {
			t.Errorf("URL = %s, want the chosen 1080p rung", got.URL)
		}
		if got.LinkOnly || got.Note != "" {
			t.Error("an explicit choice must clear the link-only default — the user meant it")
		}
	})

	t.Run("video link mode", func(t *testing.T) {
		clip := &capture.Clip{Media: []capture.Media{videoMedia()}}
		applyCaptureChoices(clip, CaptureOpts{VideoMode: repo.VideoModeLink}, prefs)
		if !clip.Media[0].LinkOnly || clip.Media[0].Note == "" {
			t.Error("link mode must mark the video link-only and explain why")
		}
	})

	t.Run("video skip drops it", func(t *testing.T) {
		clip := &capture.Clip{Media: []capture.Media{videoMedia()}}
		applyCaptureChoices(clip, CaptureOpts{VideoMode: repo.VideoModeSkip}, prefs)
		if len(clip.Media) != 0 {
			t.Errorf("skip should drop the video, got %+v", clip.Media)
		}
	})

	t.Run("image modes", func(t *testing.T) {
		mk := func() *capture.Clip {
			return &capture.Clip{Media: []capture.Media{
				{Kind: capture.MediaImage, URL: "1"},
				{Kind: capture.MediaImage, URL: "2"},
				{Kind: capture.MediaImage, URL: "3"},
			}}
		}
		c := mk()
		applyCaptureChoices(c, CaptureOpts{ImageMode: repo.ImageModeFirst}, prefs)
		if len(c.Media) != 1 || c.Media[0].URL != "1" {
			t.Errorf("first mode should keep exactly the first image, got %+v", c.Media)
		}
		c = mk()
		applyCaptureChoices(c, CaptureOpts{ImageMode: repo.ImageModeSkip}, prefs)
		if len(c.Media) != 0 {
			t.Errorf("skip should drop all images, got %+v", c.Media)
		}
		c = mk()
		applyCaptureChoices(c, CaptureOpts{ImageMode: repo.ImageModeLink}, prefs)
		if len(c.Media) != 3 || !c.Media[0].LinkOnly {
			t.Errorf("link mode should keep all images as links, got %+v", c.Media)
		}
	})

	t.Run("empty opts follow vault prefs", func(t *testing.T) {
		p := repo.DefaultCapturePrefs()
		p.ImageMode = repo.ImageModeFirst
		clip := &capture.Clip{Media: []capture.Media{
			{Kind: capture.MediaImage, URL: "1"}, {Kind: capture.MediaImage, URL: "2"},
		}}
		applyCaptureChoices(clip, CaptureOpts{}, p)
		if len(clip.Media) != 1 {
			t.Errorf("no explicit choice must fall back to the vault default, got %+v", clip.Media)
		}
	})
}
