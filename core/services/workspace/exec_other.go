//go:build !windows

package workspace

import "os/exec"

// hideWindow is Windows-only; elsewhere there is no console window to hide.
func hideWindow(*exec.Cmd) {}
