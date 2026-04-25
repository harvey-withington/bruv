package config

import (
	"os"
	"path/filepath"
)

// The BRUV config directory is split into two logical zones:
//
//   - Server-owned state lives directly under <configDir>/bruv/.
//     This is the home of preferences, profile, LLM keys + accounts,
//     notification config + history, chat history, due-date state,
//     crashes, logs, auth tokens, and repo data. When BRUV runs as a
//     remote server (Mode A/B), this entire tree is what belongs to
//     the *server machine* — the user's laptop or phone shouldn't
//     carry a copy.
//
//   - Client-owned state lives under <configDir>/bruv/clientdata/.
//     This is for settings that are genuinely per-device: window
//     bounds (each device has its own screen), the device's cached
//     server URL + device token pointer, last-opened-card for that
//     device, and similar UI ephemera. Desktop devices keep this
//     locally; the server never reads or syncs it.
//
// Desktop today runs server + client in one process, so both zones
// resolve to the same machine and the distinction is architectural.
// It matters once a remote Mode A/B deployment separates the two
// physically — at which point `clientdata/` is all the desktop needs
// to carry, and the server machine owns everything else.
//
// ServerDataDir intentionally returns the same path as ConfigDir for
// now (all existing server-owned files are flat under bruv/).
// Reorganising those into an explicit serverdata/ subdir is a later
// migration; this pass only carves out the client-owned zone.

// ServerDataDir returns the directory holding server-owned state.
// Today this is the same as ConfigDir — all existing files stay at
// their current paths. The name exists so new server-owned storage
// can opt into it explicitly.
func ServerDataDir() (string, error) {
	return configDir()
}

// ClientDataDir returns the per-device, client-only state directory.
// Created on first access. Contains window.json today; device token
// + cached server URL land here in phase 5.
func ClientDataDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "clientdata")
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// migrateToClientData moves a flat-layout file from the server-owned
// root into clientdata/ on first boot if the new location doesn't
// already hold it. No-op when the source is missing or the target
// already exists. Errors are swallowed — migration is best-effort;
// the app still works if the move fails (we just lose the setting
// once, same as a fresh install).
func migrateToClientData(fileName string) {
	src, err := configDir()
	if err != nil {
		return
	}
	dst, err := ClientDataDir()
	if err != nil {
		return
	}
	srcPath := filepath.Join(src, fileName)
	dstPath := filepath.Join(dst, fileName)

	if _, err := os.Stat(dstPath); err == nil {
		return // already migrated
	}
	if _, err := os.Stat(srcPath); err != nil {
		return // nothing to migrate
	}
	if err := os.Rename(srcPath, dstPath); err != nil {
		// On Windows, rename can fail across different volumes. Fall
		// back to copy + delete. If even that fails, leave the file
		// at the old path — a stale copy is harmless.
		data, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			return
		}
		if writeErr := os.WriteFile(dstPath, data, 0o644); writeErr != nil {
			return
		}
		_ = os.Remove(srcPath)
	}
}
