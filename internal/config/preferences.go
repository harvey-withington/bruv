package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Preferences holds user-level application settings.
type Preferences struct {
	ReopenLastRepo     bool   `json:"reopen_last_repo"`
	Theme              string `json:"theme"`               // "dark", "light", "system"
	Locale             string `json:"locale"`               // e.g. "en", "es"
	ConfirmBeforeDelete bool  `json:"confirm_before_delete"`
	SidebarWidth       int    `json:"sidebar_width"`
}

// DefaultPreferences returns sensible defaults.
func DefaultPreferences() Preferences {
	return Preferences{
		ReopenLastRepo:     false,
		Theme:              "dark",
		Locale:             "en",
		ConfirmBeforeDelete: true,
		SidebarWidth:       260,
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
