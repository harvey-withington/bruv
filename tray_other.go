//go:build !windows

package main

// Non-Windows stubs for the system tray integration.
//
// The real tray lives in tray_windows.go and depends on github.com/energye/systray,
// which requires a native build toolchain on macOS and Linux that we don't yet
// run in CI. Until those platforms get a proper release story (Sprint C), BRUV
// on non-Windows builds simply runs without a tray icon — the main window is
// the only surface.
//
// These stubs exist so cross-compilation from Windows → Linux works for CI
// smoke tests and so future Mac / Linux contributors have a buildable starting
// point.

// setupTray is a no-op on non-Windows builds. See tray_windows.go for the real
// implementation.
func (a *App) setupTray() {}

// refreshTrayTooltip is a no-op on non-Windows builds. Unread notification
// counts are still tracked; they just aren't surfaced via a tray tooltip.
func (a *App) refreshTrayTooltip() {}
