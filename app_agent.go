package main

// Desktop-only agent wrappers — methods where the runtime behaviour
// needs an extra desktop-specific side effect (tray checkmark, Wails
// quit). Pure forwarders without side effects live on *Runtime in
// core/supervisor/runtime.go and are promoted to App via embedding.

import (
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PauseAllAgents pauses the scheduler + flips the tray checkmark on.
// Tray toggle is intentionally on the App shell because it's a Wails
// runtime concern.
func (a *App) PauseAllAgents() error {
	if a.Runtime == nil {
		return nil
	}
	err := a.AgentRT().PauseAllAgents()
	if a.trayPauseItem != nil && !a.trayPauseItem.Checked() {
		a.trayPauseItem.Check()
	}
	return err
}

// ResumeAllAgents resumes the scheduler + flips the tray checkmark off.
func (a *App) ResumeAllAgents() error {
	if a.Runtime == nil {
		return nil
	}
	err := a.AgentRT().ResumeAllAgents()
	if a.trayPauseItem != nil && a.trayPauseItem.Checked() {
		a.trayPauseItem.Uncheck()
	}
	return err
}

// ForceQuit actually terminates the app (bypasses hide-to-tray).
// Stays on App because wailsRuntime.Quit needs the Wails context and
// the forceQuit flag is only meaningful in desktop mode.
func (a *App) ForceQuit() {
	a.forceQuit = true
	if a.Runtime != nil {
		if sched := a.AgentRT().Scheduler(); sched != nil {
			sched.Stop()
		}
	}
	wailsRuntime.Quit(a.ctx)
}
