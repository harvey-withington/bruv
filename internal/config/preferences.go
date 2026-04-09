package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Preferences holds user-level application settings.
type Preferences struct {
	ReopenLastRepo      bool   `json:"reopen_last_repo"`
	Theme               string `json:"theme"`  // "dark", "light", "system"
	Locale              string `json:"locale"` // e.g. "en", "es"
	ConfirmBeforeDelete bool   `json:"confirm_before_delete"`
	SidebarWidth          int    `json:"sidebar_width"`
	TypeBadgeDisplay      string `json:"type_badge_display"`       // "text", "color", "hidden"
	DefaultCategoryName   string `json:"default_category_name"`    // auto-created when a project is made
	InboxRecentCardsLimit int    `json:"inbox_recent_cards_limit"` // max cards shown in Recently Updated panel
	InboxActivityLimit     int  `json:"inbox_activity_limit"`     // max entries shown in Activity feed
	SidebarCollapseDefault bool `json:"sidebar_collapse_default"` // if true, tree starts fully collapsed on load

	// Due-date notifications
	DueDateNotify     bool     `json:"due_date_notify"`
	DueDateThresholds []string `json:"due_date_thresholds"` // ["24h", "1h", "0", "overdue"]
	DueDateChannels   string   `json:"due_date_channels"`   // "in-app,system"
}

// DefaultPreferences returns sensible defaults.
func DefaultPreferences() Preferences {
	return Preferences{
		ReopenLastRepo:        true,
		Theme:                 "dark",
		Locale:                "en",
		ConfirmBeforeDelete:   true,
		SidebarWidth:          260,
		TypeBadgeDisplay:      "color",
		DefaultCategoryName:   "Ideas",
		InboxRecentCardsLimit: 21,
		InboxActivityLimit:    25,
		DueDateNotify:         true,
		DueDateThresholds:     []string{"24h", "1h", "0"},
		DueDateChannels:       "in-app,system",
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
