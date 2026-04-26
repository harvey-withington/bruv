package main

// Desktop-only tray-driven agent controls. The user can pause / resume
// every loaded runtime's scheduler from the system tray menu — useful
// for "I'm offline, stop trying to call models" without having to
// switch repos. With multi-repo runtimes, "all" means across every
// loaded Runtime, not just the one the UI happens to be looking at.

import (
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// PauseAllAgents pauses the scheduler in every loaded runtime + flips
// the tray checkmark on. Best-effort — keeps going on per-runtime
// failures so a single-repo error doesn't strand the rest paused/
// unpaused inconsistently.
func (a *App) PauseAllAgents() error {
	for _, rt := range a.sup.LoadedRuntimes() {
		_ = rt.PauseAllAgents()
	}
	if a.trayPauseItem != nil && !a.trayPauseItem.Checked() {
		a.trayPauseItem.Check()
	}
	return nil
}

// ResumeAllAgents mirrors PauseAllAgents.
func (a *App) ResumeAllAgents() error {
	for _, rt := range a.sup.LoadedRuntimes() {
		_ = rt.ResumeAllAgents()
	}
	if a.trayPauseItem != nil && a.trayPauseItem.Checked() {
		a.trayPauseItem.Uncheck()
	}
	return nil
}

// ForceQuit actually terminates the app (bypasses hide-to-tray).
// Stays on App because wailsRuntime.Quit needs the Wails context and
// the forceQuit flag is only meaningful in desktop mode. Best-effort
// scheduler stop on every loaded runtime so in-flight work persists
// rather than getting half-killed.
func (a *App) ForceQuit() {
	a.forceQuit = true
	for _, rt := range a.sup.LoadedRuntimes() {
		if sched := rt.AgentRT().Scheduler(); sched != nil {
			sched.Stop()
		}
	}
	wailsRuntime.Quit(a.ctx)
}
