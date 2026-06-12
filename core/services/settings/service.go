// Package settings is the SettingsService — server-zone preferences,
// profile, auth info. Stateless; delegates to internal/config which
// owns persistence. Per-device UI preferences (theme, locale, layout,
// LLM nudge flag) are NOT here — they're served by the local shell
// (ShellAPI → config.UIPreferences), never over RPC.
//
// Due-date settings are NOT here: they cross into scheduler lifecycle
// (see App.SaveDueDateSettings) and stay on App until the scheduler
// is formally extracted.
package settings

import (
	"encoding/json"

	"bruv/internal/config"
)

// Service exposes user preference + profile operations.
type Service struct{}

// New constructs a SettingsService.
func New() *Service { return &Service{} }

// GetPreferences returns the current user preferences.
func (s *Service) GetPreferences() (config.Preferences, error) {
	return config.LoadPreferences()
}

// SetPreferences persists user preferences.
func (s *Service) SetPreferences(p json.RawMessage) error {
	return config.UpdatePreferencesPartial(p)
}

// GetProfile returns the user display profile.
func (s *Service) GetProfile() (config.UserProfile, error) {
	return config.LoadProfile()
}

// SetProfile persists the user display profile.
func (s *Service) SetProfile(p config.UserProfile) error {
	return config.SaveProfile(p)
}

// GetAuthInfo returns the local OS-user-derived auth info. Real auth
// lands in phase 5 (bootstrap + per-device tokens).
func (s *Service) GetAuthInfo() config.AuthInfo {
	return config.GetLocalAuthInfo()
}
