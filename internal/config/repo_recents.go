package config

// Per-device "last selected repo per connection" storage.
//
// When the user picks a repo on a remote server, we save that
// choice keyed by the connection's UUID. Next launch, the desktop
// auto-restores it without forcing the user back through the picker.
//
// Lives in <clientdata>/repo-recents.json — strictly per-device:
// "what repo did I pick on RIPPED *from this machine*". Other
// devices on the same server can have their own picks.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RepoRecents maps connection ID → last-selected repo ID for that
// connection. The empty string means "no choice yet, show picker".
type RepoRecents map[string]string

const repoRecentsFileName = "repo-recents.json"

func repoRecentsPath() (string, error) {
	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, repoRecentsFileName), nil
}

// LoadRepoRecents reads the per-connection repo selection map.
// Returns an empty map (not an error) when the file is missing.
func LoadRepoRecents() (RepoRecents, error) {
	out := RepoRecents{}
	path, err := repoRecentsPath()
	if err != nil {
		return out, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return RepoRecents{}, err
	}
	return out, nil
}

// SaveRepoRecents writes the map atomically.
func SaveRepoRecents(r RepoRecents) error {
	path, err := repoRecentsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// GetRecentRepoForConnection returns the last-selected repo ID for
// the given connection, or "" if none has been picked yet.
func GetRecentRepoForConnection(connectionID string) string {
	r, _ := LoadRepoRecents()
	return r[connectionID]
}

// SetRecentRepoForConnection persists the user's repo choice. Pass
// repoID="" to clear the entry (e.g. when the user explicitly wants
// the picker on next launch).
//
// Empty connectionID is the legitimate Local sentinel: the desktop
// stamps "" → activeRepoID after Open/Init so reopen_last_repo can
// auto-restore on next launch. We accept the empty key here for that
// reason — it's not corruption, it's the Local connection's row.
func SetRecentRepoForConnection(connectionID, repoID string) error {
	r, _ := LoadRepoRecents()
	if repoID == "" {
		delete(r, connectionID)
	} else {
		r[connectionID] = repoID
	}
	return SaveRepoRecents(r)
}
