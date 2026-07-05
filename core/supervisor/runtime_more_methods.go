// Per-repo RPC methods that mirror App helpers not yet in
// runtime_methods.go. Kept in a separate file purely for diff
// hygiene — eventually merge with runtime_methods.go.
//
// The constraint: these are forwarders that the JSON-RPC dispatcher
// reflects. Bodies stay tiny so it's obvious where the real logic
// lives (the corresponding service or repo method).

package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"bruv/internal/config"
	"bruv/internal/model"
)

// --- Repo metadata ---

func (r *Runtime) GetRepoDescription() (string, error) { return r.Repository.GetDescription() }
func (r *Runtime) UpdateRepoDescription(description string) error {
	return r.Repository.UpdateDescription(description)
}

// --- Brand / Stream / Project mutations not in runtime_methods.go ---

func (r *Runtime) MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) error {
	return r.Project.MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream)
}
func (r *Runtime) MoveStream(fromBrand, streamSlug, toBrand string) error {
	return r.Project.MoveStream(fromBrand, streamSlug, toBrand)
}
func (r *Runtime) CopyBrand(brandSlug string) (*model.Brand, error) {
	return r.Project.CopyBrand(brandSlug)
}
func (r *Runtime) CopyStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	return r.Project.CopyStream(fromBrand, streamSlug, toBrand)
}
func (r *Runtime) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string, position int) (*model.Project, error) {
	return r.Project.CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream, position)
}
func (r *Runtime) ReorderBrands(orderedSlugs []string) error {
	return r.Project.ReorderBrands(orderedSlugs)
}
func (r *Runtime) ReorderStreams(brandSlug string, orderedSlugs []string) error {
	return r.Project.ReorderStreams(brandSlug, orderedSlugs)
}
func (r *Runtime) ReorderProjects(brandSlug, streamSlug string, orderedSlugs []string) error {
	return r.Project.ReorderProjects(brandSlug, streamSlug, orderedSlugs)
}
func (r *Runtime) ReorderCategories(brandSlug, streamSlug, projectSlug string, orderedSlugs []string) error {
	return r.Project.ReorderCategories(brandSlug, streamSlug, projectSlug, orderedSlugs)
}

// --- Settings / Profile / Auth ---
//
// Settings + Profile + AuthInfo are per-machine config — both desktop
// App and headless Server read from the same files. Putting them on
// Runtime is fine: each runtime has its own *settings.Service instance
// but the underlying state is shared via config.* helpers.

func (r *Runtime) GetPreferences() (config.Preferences, error) { return r.Settings.GetPreferences() }
func (r *Runtime) SetPreferences(p json.RawMessage) error      { return r.Settings.SetPreferences(p) }
func (r *Runtime) GetAuthInfo() config.AuthInfo                { return r.Settings.GetAuthInfo() }
func (r *Runtime) GetProfile() (config.UserProfile, error)     { return r.Settings.GetProfile() }
func (r *Runtime) SetProfile(p config.UserProfile) error       { return r.Settings.SetProfile(p) }

// --- Notifications ---

func (r *Runtime) GetNotifyConfig() (config.NotifyConfig, error)    { return r.Notify.GetConfig() }
func (r *Runtime) SetNotifyConfig(c config.NotifyConfig) error      { return r.Notify.SetConfig(c) }
func (r *Runtime) GetNotifications() ([]config.Notification, error) { return r.Notify.List() }
func (r *Runtime) MarkNotificationRead(id string) error             { return r.Notify.MarkRead(id) }
func (r *Runtime) MarkAllNotificationsRead() error                  { return r.Notify.MarkAllRead() }
func (r *Runtime) ClearAllNotifications() error                     { return r.Notify.ClearAll() }

// --- Due-date settings ---
//
// GetDueDateSettings + SaveDueDateSettings touch preferences AND the
// live due-date scanner — config writes alone wouldn't reconfigure
// the running scanner. Reaching the scanner via the agent runtime
// means the JSON shape and live-update behaviour stay identical to
// the desktop App's pre-extraction version.

func (r *Runtime) GetDueDateSettings() (map[string]interface{}, error) {
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

func (r *Runtime) SaveDueDateSettings(enabled bool, thresholds []string, channels string) error {
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
	if s := r.agentRT.DueDateScanner(); s != nil {
		s.Configure(enabled, thresholds, channels)
	}
	return nil
}

// --- Agent config + run history ---

func (r *Runtime) GetAgentConfig(cardID string) (*model.AgentFile, error) {
	return r.Agent.GetConfig(cardID)
}
func (r *Runtime) SaveAgentConfig(cardID string, cfg model.AgentConfig) error {
	return r.Agent.SaveConfig(cardID, cfg)
}
func (r *Runtime) ValidateSchedulePreview(schedule, startDate, endDate, timezone string, count int) ([]string, error) {
	return r.Agent.ValidateSchedulePreview(schedule, startDate, endDate, timezone, count)
}
func (r *Runtime) GetAgentRuns(cardID string) ([]model.AgentRun, error) {
	return r.Agent.GetRuns(cardID)
}
func (r *Runtime) ClearAgentRuns(cardID string) error {
	return r.Agent.ClearRuns(cardID)
}

// ListAgentCardStates scans the cards directory for *.agent.json
// files and returns each card's enabled/disabled flag. Used by the
// dashboard to badge agent-configured cards. Returns an empty map
// rather than an error when the repo isn't open or the cards dir
// is unreadable — caller treats absent state as "no agent".
func (r *Runtime) ListAgentCardStates() (map[string]bool, error) {
	states := map[string]bool{}
	if r.repo == nil {
		return states, nil
	}
	cardsDir := filepath.Join(r.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return states, nil
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := r.repo.GetAgentConfig(cardID)
		if err != nil {
			continue
		}
		states[cardID] = af.Config.Enabled
	}
	return states, nil
}
