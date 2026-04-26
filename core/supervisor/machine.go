package supervisor

// MachineService is the dispatcher target for the host's per-machine
// (non-per-repo) RPC surface — preferences, profile, LLM accounts,
// notification config, etc. Mounted at /server/rpc by the transport,
// reached by clients before any repo is open (or while no repo is
// selected). On the desktop, the App's loopback HTTP server hosts
// one of these against the desktop's config directory; on a headless
// server the same struct hosts against the server's config directory.
//
// Why a separate service instead of putting these methods on Runtime:
// pre-Phase-1 the desktop had App-level overrides in app_settings.go
// because the embedded *Runtime was nil before any repo opened, and
// the Runtime versions panicked on nil. After the multi-repo pivot
// (audit plan 2026-04-26) Local routes through /repos/<id>/rpc just
// like Remote, so there's no Runtime to fall back to when no repo is
// selected — yet the per-machine config (last-used LLM, due-date
// thresholds, the boot-time first-run nudge flag) still needs to be
// reachable. /server/rpc is that "no repo" RPC surface; MachineService
// is its dispatcher target.
//
// The service has no dependency on a Supervisor or Runtime — every
// method talks straight to internal/config helpers. That's the whole
// point: it works regardless of repo state.

import (
	"bruv/internal/config"
)

// MachineService is the per-machine RPC surface. Construct via NewMachineService.
type MachineService struct{}

// NewMachineService constructs a MachineService.
func NewMachineService() *MachineService { return &MachineService{} }

// --- Preferences / Profile / Auth ---

func (m *MachineService) GetPreferences() (config.Preferences, error) { return config.LoadPreferences() }
func (m *MachineService) SetPreferences(p config.Preferences) error   { return config.SavePreferences(p) }
func (m *MachineService) GetProfile() (config.UserProfile, error)     { return config.LoadProfile() }
func (m *MachineService) SetProfile(p config.UserProfile) error       { return config.SaveProfile(p) }
func (m *MachineService) GetAuthInfo() config.AuthInfo                { return config.GetLocalAuthInfo() }

// MarkLLMNudgeShown persists a flag so the first-run LLM-configuration
// nudge only fires once per install. Doesn't depend on a Runtime.
func (m *MachineService) MarkLLMNudgeShown() error {
	p, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	p.LLMNudgeShown = true
	return config.SavePreferences(p)
}

// --- LLM accounts / config / pricing ---

func (m *MachineService) GetLLMConfig() (config.LLMConfig, error)       { return config.LoadLLMConfig() }
func (m *MachineService) SetLLMConfig(c config.LLMConfig) error         { return config.SaveLLMConfig(c) }
func (m *MachineService) GetLLMAccounts() ([]config.LLMAccount, error)  { return config.LoadLLMAccounts() }
func (m *MachineService) SaveLLMAccounts(x []config.LLMAccount) error   { return config.SaveLLMAccounts(x) }
func (m *MachineService) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return config.LoadCustomPricing()
}
func (m *MachineService) SaveTokenPricing(p map[string]config.ModelPricing) error {
	return config.SaveCustomPricing(p)
}

// IsLLMConfigured returns true if any usable LLM provider exists.
// Mirrors core/services/llm.IsConfigured but doesn't require a Runtime
// — the boot LLM-nudge check fires before any repo is open.
func (m *MachineService) IsLLMConfigured() bool {
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

func (m *MachineService) GetNotifyConfig() (config.NotifyConfig, error) {
	return config.LoadNotifyConfig()
}
func (m *MachineService) SetNotifyConfig(c config.NotifyConfig) error {
	return config.SaveNotifyConfig(c)
}

// GetNotifications returns the host's notification feed. Reads
// directly from the on-disk store (which the per-repo notify service
// also writes to), so it works without a loaded Runtime — boot path
// uses this to populate the tray badge.
func (m *MachineService) GetNotifications() ([]config.Notification, error) {
	return config.LoadNotifications()
}

// --- Due-date settings (per-machine in storage; live scanner update
// requires a Runtime — see Runtime.SaveDueDateSettings for the
// scheduler-update path. Per-machine versions persist only). ---

func (m *MachineService) GetDueDateSettings() (map[string]interface{}, error) {
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

func (m *MachineService) SaveDueDateSettings(enabled bool, thresholds []string, channels string) error {
	prefs, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	prefs.DueDateNotify = enabled
	prefs.DueDateThresholds = thresholds
	prefs.DueDateChannels = channels
	return config.SavePreferences(prefs)
}
