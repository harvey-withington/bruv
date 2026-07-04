//go:build windows

package workspace

import (
	"os/exec"
	"syscall"
)

// hideWindow stops the console window flashing when the GUI app shells out.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
