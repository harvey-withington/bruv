//go:build !windows

package main

import "os/exec"

// hideConsole is Windows-only; elsewhere there is no console window to hide.
func hideConsole(*exec.Cmd) {}
