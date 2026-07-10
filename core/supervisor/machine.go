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
	"encoding/json"
	"fmt"

	"bruv/internal/config"
	"bruv/internal/push"
)

// MachineService is the per-machine RPC surface. Construct via NewMachineService.
type MachineService struct {
	// Optional. Set via WithPush by the server bootstrap when push is
	// configured. Nil when the host doesn't support push (Wails desktop
	// in dev, tests). Push RPC methods return a clear error in that case
	// rather than panicking.
	vapid    *push.VAPID
	registry *push.Registry
}

// NewMachineService constructs a MachineService.
func NewMachineService() *MachineService { return &MachineService{} }

// WithPush wires the VAPID keypair + subscription registry into the
// service. Optional — only the headless server bootstrap calls this
// today; the desktop and tests run without push and the relevant RPCs
// return errPushNotConfigured if invoked.
func (m *MachineService) WithPush(v *push.VAPID, r *push.Registry) *MachineService {
	m.vapid = v
	m.registry = r
	return m
}

// errPushNotConfigured is returned by push RPCs on a host without
// push wired up. Phrased so the user-facing toast can render it
// directly without exposing internals.
var errPushNotConfigured = fmt.Errorf("push notifications are not configured on this server")

// --- Preferences / Profile / Auth ---

func (m *MachineService) GetPreferences() (config.Preferences, error) {
	return config.LoadPreferences()
}
func (m *MachineService) SetPreferences(p json.RawMessage) error {
	return config.UpdatePreferencesPartial(p)
}
func (m *MachineService) GetProfile() (config.UserProfile, error) { return config.LoadProfile() }
func (m *MachineService) SetProfile(p config.UserProfile) error   { return config.SaveProfile(p) }
func (m *MachineService) GetAuthInfo() config.AuthInfo            { return config.GetLocalAuthInfo() }

// --- LLM accounts / config / pricing ---

func (m *MachineService) GetLLMConfig() (config.LLMConfig, error) { return config.LoadLLMConfig() }
func (m *MachineService) SetLLMConfig(c config.LLMConfig) error   { return config.SaveLLMConfig(c) }
func (m *MachineService) GetLLMAccounts() ([]config.LLMAccount, error) {
	return config.LoadLLMAccounts()
}
func (m *MachineService) SaveLLMAccounts(x []config.LLMAccount) error {
	return config.SaveLLMAccounts(x)
}
// Token-pricing RPCs deleted 2026-07-10 (ruled: costs stay estimates
// from the built-in table; hand-editing <configDir>/pricing.json is
// still honoured by config.EstimateCost's merge).

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

// --- Push notifications (Phase 3 prep) ---
//
// The mobile PWA's service worker calls navigator.pushManager.subscribe
// with the public key returned by GetVapidPublicKey, then forwards the
// resulting endpoint + keys to RegisterPushSubscription. Unregistering
// is a clean-uninstall path. Send is server-side only (agents, due-
// date scanner) — there's no RPC for "send a notification to me",
// because the trust model is the server pushes when it has news.
//
// Trust note: deviceID is supplied by the caller and isn't validated
// against the bearer token. Within a single Tailscale-trust group
// this is acceptable; the worst case is a malicious paired client
// stealing notifications meant for another paired client. When the
// device-store hook lands (auth context → method param), this gets
// tightened automatically.

// GetVapidPublicKey returns the server's VAPID application server
// key as a base64url-encoded string. The mobile client passes it to
// the W3C PushManager.subscribe() call's `applicationServerKey`.
func (m *MachineService) GetVapidPublicKey() (string, error) {
	if m.vapid == nil {
		return "", errPushNotConfigured
	}
	return m.vapid.Public(), nil
}

// RegisterPushSubscription stores (or replaces) the push subscription
// for a device. Called by the mobile client right after PushManager
// subscribe() resolves with a fresh PushSubscription.
func (m *MachineService) RegisterPushSubscription(deviceID, endpoint, p256dh, auth string) error {
	if m.registry == nil {
		return errPushNotConfigured
	}
	return m.registry.Upsert(push.Subscription{
		DeviceID: deviceID,
		Endpoint: endpoint,
		P256dh:   p256dh,
		Auth:     auth,
	})
}

// UnregisterPushSubscription drops a device's subscription. The
// mobile client calls this on explicit "stop notifications" actions;
// the sender also removes subscriptions automatically when the push
// service returns 410 Gone (a stronger signal that the subscription
// is dead).
func (m *MachineService) UnregisterPushSubscription(deviceID string) error {
	if m.registry == nil {
		return errPushNotConfigured
	}
	return m.registry.Remove(deviceID)
}
