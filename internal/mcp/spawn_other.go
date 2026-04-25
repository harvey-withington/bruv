//go:build !windows

package mcp

import "os/exec"

// hideChildWindow is a no-op everywhere except Windows. Unix-y OSes
// don't have the GUI-vs-console subsystem distinction that creates
// the popup-window problem this function exists to solve.
func hideChildWindow(_ *exec.Cmd) {}
