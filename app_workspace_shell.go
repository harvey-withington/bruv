package main

// Device-side Workspace shell actions (Tier 1: open in default app, reveal
// in file manager, per-workspace launch command). These are SHELL_METHODS —
// they act on files on the machine the user is sitting at, so they run in
// the Wails shell process, never over RPC. Every path resolves through the
// internal/workspace chokepoint against the workspace root the frontend
// passes, so a malformed relative path can't reach outside the workspace.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	pathsafe "bruv/internal/workspace"
)

// resolveWorkspacePath validates root+rel via the chokepoint and confirms
// the target exists (shell-open on a missing path gives OS-flavoured
// nonsense errors; fail clearly instead).
func resolveWorkspacePath(root, rel string) (string, error) {
	abs, err := pathsafe.Resolve(root, rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("path does not exist: %s", rel)
	}
	return abs, nil
}

// OpenWorkspacePath opens a workspace file (or folder) with the OS default
// application. rel = "" opens the workspace root folder.
func (a *App) OpenWorkspacePath(root, rel string) error {
	abs, err := resolveWorkspacePath(root, rel)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		// NOT `cmd /C start`: start's failures are silent under a hidden
		// console, and its argument parsing mangles paths with quotes/
		// apostrophes/parens. rundll32's FileProtocolHandler opens files
		// AND folders with the default handler from a single argument.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", abs).Start()
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}

// RevealWorkspacePath shows the file in the OS file manager, selected when
// the platform supports it.
func (a *App) RevealWorkspacePath(root, rel string) error {
	abs, err := resolveWorkspacePath(root, rel)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		// explorer /select returns a nonzero exit code even on success —
		// Start (not Run) and ignore the status, like openInFileManager.
		return exec.Command("explorer", "/select,", abs).Start()
	case "darwin":
		return exec.Command("open", "-R", abs).Start()
	default:
		info, err := os.Stat(abs)
		if err == nil && !info.IsDir() {
			abs = strings.TrimSuffix(abs, "/"+rel) // best effort: open parent
		}
		return exec.Command("xdg-open", abs).Start()
	}
}

// RunWorkspaceLaunchCommand runs the user-configured per-workspace launcher
// (e.g. `code .`, `obsidian://open?...`) with the workspace root as working
// directory. The command is the user's own configuration for their own
// machine — it runs as typed, via the platform shell so PATH lookup, URL
// protocols, and arguments behave like a terminal would.
func (a *App) RunWorkspaceLaunchCommand(root, command string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("no launch command configured")
	}
	if _, err := pathsafe.Resolve(root, ""); err != nil {
		return err
	}
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("workspace folder not found: %s", root)
	}
	// URL-scheme launchers (obsidian://open?..., vscode://...) aren't shell
	// commands — hand them to the OS protocol handler instead. This is the
	// spec's own example (§3 Tier 1), so it must work as typed.
	if isURLScheme(command) {
		switch runtime.GOOS {
		case "windows":
			return exec.Command("rundll32", "url.dll,FileProtocolHandler", strings.TrimSpace(command)).Start()
		case "darwin":
			return exec.Command("open", strings.TrimSpace(command)).Start()
		default:
			return exec.Command("xdg-open", strings.TrimSpace(command)).Start()
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", command)
		hideConsole(cmd)
	default:
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = root
	return cmd.Start()
}

// isURLScheme reports whether the launch command is a protocol URL like
// obsidian://… (scheme per RFC 3986: letter, then letters/digits/+/-/.),
// rather than a shell command line.
func isURLScheme(command string) bool {
	s := strings.TrimSpace(command)
	i := strings.Index(s, "://")
	if i <= 0 {
		return false
	}
	for j, r := range s[:i] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case j > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	// A shell command containing spaces before :// is not a bare URL.
	return !strings.ContainsAny(s, " \t")
}
