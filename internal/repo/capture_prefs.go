package repo

// Vault-level capture preferences.
//
// Every capture defect found during the 2026-07-31/08-02 build was BRUV
// silently choosing on the user's behalf — which video variant, what counts
// as "too large", whether an un-storable video becomes a thumbnail, where a
// card pins. Harvey's ruling (2026-08-02): make those decisions the user's,
// at capture time, pre-populated from defaults they control.
//
// These prefs are the defaults half. They live in the REPO (like
// template_prefs.json) rather than per device, because the phone, the
// browser extension and the desktop all capture into the same vault and
// must agree about what happens to a 3.5 GB video. Switching vaults
// switches capture policy with it.
//
// The server stores and applies these; it never invents them.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Video/image mode values. Kept as strings (not ints) so the on-disk file
// stays readable and a future mode doesn't renumber anything.
const (
	VideoModeFit      = "fit"      // richest variant that fits the budget (default)
	VideoModeBest     = "best"     // always the highest quality, whatever the size
	VideoModeSmallest = "smallest" // always the cheapest variant
	VideoModeLink     = "link"     // never store video; link to the platform
	VideoModeSkip     = "skip"     // ignore video entirely

	ImageModeAll   = "all"   // store every image (galleries included)
	ImageModeFirst = "first" // store only the first
	ImageModeLink  = "link"  // link, don't store
	ImageModeSkip  = "skip"

	AskAlways   = "always"   // show the capture dialog every time
	AskTriggers = "triggers" // show it when one of the user's triggers fires (default)
	AskNever    = "never"    // never ask; apply defaults silently
)

// CaptureTriggers is the user's own definition of "consequential".
//
// Harvey, 2026-08-02, on being asked how BRUV should decide when to
// prompt: "How do we define consequential? That would need to come from
// the user in a setting." Exactly — anything BRUV decides here is another
// silent judgement call, so the thresholds are the user's. A zero
// threshold or false flag means that trigger is off.
type CaptureTriggers struct {
	// VideoOverMB fires when the video BRUV would store exceeds this size.
	VideoOverMB int `json:"videoOverMB,omitempty"`
	// GalleryOverCount fires when a post carries more images than this.
	GalleryOverCount int `json:"galleryOverCount,omitempty"`
	// UnsupportedURL fires when no resolver claims the URL (it would
	// become a plain link card).
	UnsupportedURL bool `json:"unsupportedUrl,omitempty"`
	// Blocked fires when the platform refused the server, so the capture
	// would land as a pending clip needing the desktop extension.
	Blocked bool `json:"blocked,omitempty"`
	// PinMayReject fires when the chosen pin target's accepted-types would
	// bounce the card into the Inbox.
	PinMayReject bool `json:"pinMayReject,omitempty"`
}

// CapturePrefs mirrors shared/types.ts CapturePrefs — JSON key casing must
// stay in lockstep; both frontends consume this verbatim.
type CapturePrefs struct {
	VideoMode string `json:"videoMode,omitempty"`
	// VideoBudgetMB bounds VideoModeFit. Also the natural companion to
	// the VideoOverMB trigger, but kept separate: how much you're willing
	// to STORE and when you want to be ASKED are different questions.
	VideoBudgetMB int             `json:"videoBudgetMB,omitempty"`
	ImageMode     string          `json:"imageMode,omitempty"`
	AskMode       string          `json:"askMode,omitempty"`
	Triggers      CaptureTriggers `json:"triggers"`
}

// DefaultCapturePrefs are what a fresh vault captures with: keep quality
// sensible, keep quick capture quick, and ask only when something is
// genuinely at stake.
func DefaultCapturePrefs() CapturePrefs {
	return CapturePrefs{
		VideoMode:     VideoModeFit,
		VideoBudgetMB: 50,
		ImageMode:     ImageModeAll,
		AskMode:       AskTriggers,
		Triggers: CaptureTriggers{
			VideoOverMB:      50,
			GalleryOverCount: 8,
			UnsupportedURL:   true,
			Blocked:          true,
			PinMayReject:     true,
		},
	}
}

// withDefaults fills blanks so a partially-written file (or an older
// schema) still behaves, without overwriting anything the user set.
func (p CapturePrefs) withDefaults() CapturePrefs {
	d := DefaultCapturePrefs()
	if p.VideoMode == "" {
		p.VideoMode = d.VideoMode
	}
	if p.VideoBudgetMB <= 0 {
		p.VideoBudgetMB = d.VideoBudgetMB
	}
	if p.ImageMode == "" {
		p.ImageMode = d.ImageMode
	}
	if p.AskMode == "" {
		p.AskMode = d.AskMode
	}
	return p
}

// CapturePrefsStore is the on-disk wrapper — a version marker so future
// schema additions don't break the file format.
type CapturePrefsStore struct {
	Version int `json:"version"`
	CapturePrefs
}

func (r *Repository) capturePrefsPath() string {
	return filepath.Join(r.Root, "capture_prefs.json")
}

// LoadCapturePrefs reads the vault's capture prefs. A missing file is the
// normal fresh-vault state and returns the defaults, not an error.
func (r *Repository) LoadCapturePrefs() (CapturePrefs, error) {
	data, err := os.ReadFile(r.capturePrefsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultCapturePrefs(), nil
		}
		return DefaultCapturePrefs(), err
	}
	var store CapturePrefsStore
	if err := json.Unmarshal(data, &store); err != nil {
		return DefaultCapturePrefs(), err
	}
	return store.CapturePrefs.withDefaults(), nil
}

// SaveCapturePrefs writes the vault's capture prefs.
func (r *Repository) SaveCapturePrefs(p CapturePrefs) error {
	store := CapturePrefsStore{Version: 1, CapturePrefs: p.withDefaults()}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.capturePrefsPath(), data, 0o644)
}
