package main

// Wails-bound agent methods — thin forwarders over the agent runtime
// (see core/runtime/agent). The runtime owns the scheduler, due-date
// scanner, executeAgent loop, tool dispatch, and all agent-control
// semantics (Cancel / Trigger / Pause / Resume / scheduler status /
// aggregate reads). This file's job is to:
//
//   - Expose that surface to Wails (auto-bound by method name).
//   - Layer on desktop-only presentation: tray "Pause Agents" checkbox
//     toggles, ForceQuit's Wails quit call.
//
// The headless binary (cmd/bruv-server) mirrors the same method names
// through its own thin forwarders so the JSON-RPC reflection
// dispatcher exposes an identical surface to the frontend.

import (
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) CancelAgent(cardID string) error  { return a.agentRT.CancelAgent(cardID) }
func (a *App) TriggerAgent(cardID string) error { return a.agentRT.TriggerAgent(cardID) }

// PauseAllAgents pauses the scheduler + flips the tray checkmark on.
// Tray toggle is intentionally on the App shell because it's a Wails
// runtime concern.
func (a *App) PauseAllAgents() error {
	err := a.agentRT.PauseAllAgents()
	if a.trayPauseItem != nil && !a.trayPauseItem.Checked() {
		a.trayPauseItem.Check()
	}
	return err
}

// ResumeAllAgents resumes the scheduler + flips the tray checkmark off.
func (a *App) ResumeAllAgents() error {
	err := a.agentRT.ResumeAllAgents()
	if a.trayPauseItem != nil && a.trayPauseItem.Checked() {
		a.trayPauseItem.Uncheck()
	}
	return err
}

func (a *App) GetAgentSchedulerStatus() map[string]any { return a.agentRT.GetAgentSchedulerStatus() }
func (a *App) GetAllAgents() ([]map[string]any, error) { return a.agentRT.GetAllAgents() }
func (a *App) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	return a.agentRT.GetAllAgentRuns(limit)
}
func (a *App) GetAgentAnalytics() (map[string]any, error) { return a.agentRT.GetAgentAnalytics() }

// ForceQuit actually terminates the app (bypasses hide-to-tray).
// Stays on App because wailsRuntime.Quit needs the Wails context and
// the forceQuit flag is only meaningful in desktop mode.
func (a *App) ForceQuit() {
	a.forceQuit = true
	if sched := a.agentRT.Scheduler(); sched != nil {
		sched.Stop()
	}
	wailsRuntime.Quit(a.ctx)
}
