//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsole stops a cmd.exe console window flashing over the GUI when
// workspace shell actions run. Same pattern as internal/mcp/spawn_windows.go.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
