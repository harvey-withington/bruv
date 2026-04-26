package main

// Per-machine settings overrides on App that bypass the embedded
// *Runtime. These methods don't depend on a specific repo — they
// read/write `<configDir>/...` files directly via internal/config —
// so requiring a Runtime is wrong. Without these overrides the
// promoted Runtime versions panic on nil before any repo is open
// (App boot calls GetPreferences before the user picks a repo).
//
// Server-side, the same RPC names dispatch to Runtime versions (which
// also call config.* — same effect). Either side, the wire shape
// is identical.

import (
	"bruv/internal/config"
)

// --- Preferences / Profile / Auth ---

func (a *App) GetPreferences() (config.Preferences, error) { return config.LoadPreferences() }
func (a *App) SetPreferences(p config.Preferences) error   { return config.SavePreferences(p) }
func (a *App) GetProfile() (config.UserProfile, error)     { return config.LoadProfile() }
func (a *App) SetProfile(p config.UserProfile) error       { return config.SaveProfile(p) }
func (a *App) GetAuthInfo() config.AuthInfo                { return config.GetLocalAuthInfo() }

// MarkLLMNudgeShown persists a flag so the first-run LLM-configuration
// nudge only fires once per install. Doesn't depend on a Runtime.
func (a *App) MarkLLMNudgeShown() error {
	p, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	p.LLMNudgeShown = true
	return config.SavePreferences(p)
}

// --- LLM accounts / config / pricing ---

func (a *App) GetLLMConfig() (config.LLMConfig, error)       { return config.LoadLLMConfig() }
func (a *App) SetLLMConfig(c config.LLMConfig) error         { return config.SaveLLMConfig(c) }
func (a *App) GetLLMAccounts() ([]config.LLMAccount, error)  { return config.LoadLLMAccounts() }
func (a *App) SaveLLMAccounts(x []config.LLMAccount) error   { return config.SaveLLMAccounts(x) }
func (a *App) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return config.LoadCustomPricing()
}
func (a *App) SaveTokenPricing(p map[string]config.ModelPricing) error {
	return config.SaveCustomPricing(p)
}

// IsLLMConfigured returns true if any usable LLM provider exists.
// Mirrors core/services/llm.IsConfigured but doesn't require a
// Runtime — the boot LLM-nudge check fires before any repo is open.
func (a *App) IsLLMConfigured() bool {
	cfg, err := config.LoadLLMConfig()
	if err == nil && cfg.Provider != "" {
		return true
	}
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return false
	}
	for _, acct := range accounts {
		if acct.APIKey != "" || acct.Provider == "ollama" {
			return true
		}
	}
	return false
}

// --- Notifications config (per-machine; the actual notification list
// + read/clear ops still go through the Runtime so they touch the
// per-repo notify service. Config is shared across repos). ---

func (a *App) GetNotifyConfig() (config.NotifyConfig, error) { return config.LoadNotifyConfig() }
func (a *App) SetNotifyConfig(c config.NotifyConfig) error   { return config.SaveNotifyConfig(c) }

// GetNotifications: when no Runtime, return an empty list rather than
// panic. The UI calls this on boot to populate the tray badge —
// pre-repo state is "no notifications yet", which is honest.
func (a *App) GetNotifications() ([]config.Notification, error) {
	if a.Runtime == nil {
		return []config.Notification{}, nil
	}
	return a.Runtime.GetNotifications()
}

// --- Due-date settings (per-machine in storage; live scanner update
// requires a Runtime, so degrade gracefully when none is loaded). ---

func (a *App) GetDueDateSettings() (map[string]interface{}, error) {
	prefs, err := config.LoadPreferences()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"enabled":    prefs.DueDateNotify,
		"thresholds": prefs.DueDateThresholds,
		"channels":   prefs.DueDateChannels,
	}, nil
}

func (a *App) SaveDueDateSettings(enabled bool, thresholds []string, channels string) error {
	prefs, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	prefs.DueDateNotify = enabled
	prefs.DueDateThresholds = thresholds
	prefs.DueDateChannels = channels
	if err := config.SavePreferences(prefs); err != nil {
		return err
	}
	if a.Runtime != nil {
		if s := a.AgentRT().DueDateScanner(); s != nil {
			s.Configure(enabled, thresholds, channels)
		}
	}
	return nil
}
