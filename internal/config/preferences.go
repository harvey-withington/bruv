package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var prefsMu sync.Mutex

// Preferences holds server-zone application settings: values that
// describe the data or the backend's behaviour and therefore SHOULD be
// shared by every device talking to this backend. Per-device
// presentation settings (theme, locale, layout, first-run flags) live
// in UIPreferences (ui_preferences.go) under <clientdata>/ instead.
//
// See the server/client zone audit table in CONTRIBUTING.md.
type Preferences struct {
	DefaultCategoryName string `json:"default_category_name"` // auto-created when a project is made

	TrelloAPIKey   string `json:"trello_api_key"`
	TrelloAPIToken string `json:"trello_api_token"`

	// Due-date notifications
	DueDateNotify     bool     `json:"due_date_notify"`
	DueDateThresholds []string `json:"due_date_thresholds"` // ["24h", "1h", "0", "overdue"]
	DueDateChannels   string   `json:"due_date_channels"`   // "in-app,system"
}

// DefaultPreferences returns sensible defaults.
func DefaultPreferences() Preferences {
	return Preferences{
		DefaultCategoryName: "Ideas",
		DueDateNotify:       true,
		DueDateThresholds:   []string{"24h", "1h", "0"},
		DueDateChannels:     "in-app,system",
	}
}

func prefsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

// LoadPreferences reads preferences from disk, returning defaults if not found.
func LoadPreferences() (Preferences, error) {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	return loadPreferencesUnlocked()
}

func loadPreferencesUnlocked() (Preferences, error) {
	p := DefaultPreferences()
	path, err := prefsPath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return DefaultPreferences(), err
	}
	return p, nil
}

// SavePreferences writes preferences to disk.
func SavePreferences(p Preferences) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	return savePreferencesUnlocked(p)
}

func savePreferencesUnlocked(p Preferences) error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// UpdatePreferencesPartial merges partial JSON bytes into the loaded preferences.
func UpdatePreferencesPartial(raw []byte) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	p, err := loadPreferencesUnlocked()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return savePreferencesUnlocked(p)
}
