//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// hideChildWindow tells Windows not to allocate a console window for
// the child process. Required because bruv.exe is a GUI-subsystem
// binary; spawning a console-subsystem child (cmd.exe for npx-wrapped
// MCP servers, python.exe for python-based servers, etc.) otherwise
// pops up a fresh, blank console window. The supervisor goroutine
// would then restart the child every time the user closed it.
//
// CREATE_NO_WINDOW (0x08000000) is the canonical flag — HideWindow is
// the higher-level cousin that asks the new process not to call
// ShowWindow on its main window, but for console programs the only
// window IS the console host, so we need to suppress it at creation.
func hideChildWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
