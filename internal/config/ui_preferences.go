package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var uiPrefsMu sync.Mutex

// UIPreferences holds per-device presentation settings. They live in
// <clientdata>/ui_preferences.json and are served by the local shell
// (ShellAPI binding), never by the backend RPC surface — so in remote
// mode each device keeps its own theme, locale, layout, and first-run
// flags instead of sharing them through the home server. Server-zone
// settings (due-date notification config, default category name,
// importer credentials) stay in Preferences (preferences.json).
//
// See the server/client zone audit table in CONTRIBUTING.md.
type UIPreferences struct {
	ReopenLastRepo         bool   `json:"reopen_last_repo"`
	Theme                  string `json:"theme"`  // "dark", "light", "system"
	Locale                 string `json:"locale"` // e.g. "en", "es"
	ConfirmBeforeDelete    bool   `json:"confirm_before_delete"`
	SidebarWidth           int    `json:"sidebar_width"`
	TypeBadgeDisplay       string `json:"type_badge_display"`       // "text", "color", "hidden"
	InboxRecentCardsLimit  int    `json:"inbox_recent_cards_limit"` // max cards shown in Recently Updated panel
	InboxActivityLimit     int    `json:"inbox_activity_limit"`     // max entries shown in Activity feed
	SidebarCollapseDefault bool   `json:"sidebar_collapse_default"` // if true, tree starts fully collapsed on load

	// First-run guidance: set to true after the LLM-configuration nudge
	// has been shown once on THIS device.
	LLMNudgeShown bool `json:"llm_nudge_shown"`
}

// DefaultUIPreferences returns sensible defaults.
func DefaultUIPreferences() UIPreferences {
	return UIPreferences{
		ReopenLastRepo:        true,
		Theme:                 "dark",
		Locale:                "en",
		ConfirmBeforeDelete:   true,
		SidebarWidth:          260,
		TypeBadgeDisplay:      "color",
		InboxRecentCardsLimit: 21,
		InboxActivityLimit:    25,
	}
}

func uiPrefsPath() (string, error) {
	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ui_preferences.json"), nil
}

// LoadUIPreferences reads per-device preferences from disk. On first
// load (file absent) it seeds them from the legacy server-zone
// preferences.json, which carried these fields before the split —
// a one-shot read-time migration so existing installs keep their
// theme/layout instead of snapping back to defaults.
func LoadUIPreferences() (UIPreferences, error) {
	uiPrefsMu.Lock()
	defer uiPrefsMu.Unlock()
	return loadUIPreferencesUnlocked()
}

func loadUIPreferencesUnlocked() (UIPreferences, error) {
	p := DefaultUIPreferences()
	path, err := uiPrefsPath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			migrated := migrateLegacyUIPrefs(p)
			// Persist so the migration runs exactly once; best-effort —
			// failure just means we re-derive next time.
			_ = saveUIPreferencesUnlocked(migrated)
			return migrated, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return DefaultUIPreferences(), err
	}
	return p, nil
}

// migrateLegacyUIPrefs seeds UIPreferences from the pre-split
// preferences.json. The client-zone fields kept their JSON tags in
// the move, so unmarshalling the legacy file into the new struct
// lifts exactly the fields that migrated and ignores the rest.
func migrateLegacyUIPrefs(defaults UIPreferences) UIPreferences {
	path, err := prefsPath()
	if err != nil {
		return defaults
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	p := defaults
	if err := json.Unmarshal(data, &p); err != nil {
		return defaults
	}
	return p
}

// SaveUIPreferences writes per-device preferences to disk.
func SaveUIPreferences(p UIPreferences) error {
	uiPrefsMu.Lock()
	defer uiPrefsMu.Unlock()
	return saveUIPreferencesUnlocked(p)
}

func saveUIPreferencesUnlocked(p UIPreferences) error {
	path, err := uiPrefsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// UpdateUIPreferencesPartial merges partial JSON into the stored
// preferences — mirrors UpdatePreferencesPartial on the server zone.
func UpdateUIPreferencesPartial(raw []byte) error {
	uiPrefsMu.Lock()
	defer uiPrefsMu.Unlock()

	p, err := loadUIPreferencesUnlocked()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return saveUIPreferencesUnlocked(p)
}
