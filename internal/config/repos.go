package config

// Server-side multi-repo registry.
//
// A bruv-server hosts one or more BRUV repositories. The list is
// persisted in <configDir>/repos.json and edited via:
//
//   bruv.exe service install --repo X    # appends an entry
//   manual edit + restart service        # for power users
//
// The server opens every entry on startup, indexes them by ID, and
// the HTTP transport routes per-repo URLs (/repos/<id>/rpc etc.) to
// the right runtime.
//
// Entry IDs are stable UUIDs (NOT the path) so a repo can be moved
// on disk without breaking client bookmarks. Names are user-facing
// labels, defaulted from the path's basename when an entry is added.
//
// Storage: <configDir>/repos.json — same directory the rest of
// server-owned state already lives in.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// RepoEntry is one row in the server's repo registry.
//
// Disabled lets an operator suspend a repo without removing it: the
// supervisor doesn't open it, no scheduler runs, no MCP servers
// start, no notifications fire. Toggling Disabled to false brings
// the runtime back up on the next supervisor refresh.
type RepoEntry struct {
	ID       string `json:"id"`                 // stable UUID
	Name     string `json:"name"`               // user-friendly label
	Path     string `json:"path"`               // absolute path on the server
	Disabled bool   `json:"disabled,omitempty"` // true = present in registry but not loaded
}

// ReposStore is the on-disk shape.
type ReposStore struct {
	Repos []RepoEntry `json:"repos"`
}

const reposFileName = "repos.json"

func reposFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, reposFileName), nil
}

// LoadRepos reads the registry. Returns an empty store (not an error)
// when the file is missing — that's the normal state on a brand-new
// server before the first AppendRepo.
func LoadRepos() (ReposStore, error) {
	var s ReposStore
	path, err := reposFilePath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ReposStore{}, err
	}
	return s, nil
}

// SaveRepos writes the registry atomically.
func SaveRepos(s ReposStore) error {
	path, err := reposFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SetRepoDisabled flips the Disabled flag for an entry. The caller is
// responsible for reloading the supervisor afterwards.
func SetRepoDisabled(id string, disabled bool) error {
	store, err := LoadRepos()
	if err != nil {
		return err
	}
	found := false
	for i := range store.Repos {
		if store.Repos[i].ID == id {
			store.Repos[i].Disabled = disabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("repo %q not found", id)
	}
	return SaveRepos(store)
}

// RemoveRepo drops an entry from the registry. The caller is
// responsible for shutting down its runtime.
func RemoveRepo(id string) error {
	store, err := LoadRepos()
	if err != nil {
		return err
	}
	filtered := make([]RepoEntry, 0, len(store.Repos))
	for _, e := range store.Repos {
		if e.ID == id {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == len(store.Repos) {
		return fmt.Errorf("repo %q not found", id)
	}
	store.Repos = filtered
	return SaveRepos(store)
}

// AppendRepo adds a new entry to the registry, generating an ID and
// defaulting the name from the path's basename. Idempotent on path:
// if an entry already references this path, returns it unchanged.
func AppendRepo(path, name string) (RepoEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return RepoEntry{}, fmt.Errorf("resolve path: %w", err)
	}
	if name == "" {
		name = filepath.Base(abs)
	}

	store, err := LoadRepos()
	if err != nil {
		return RepoEntry{}, err
	}
	for _, e := range store.Repos {
		if e.Path == abs {
			return e, nil
		}
	}

	entry := RepoEntry{
		ID:   uuid.NewString(),
		Name: name,
		Path: abs,
	}
	store.Repos = append(store.Repos, entry)
	if err := SaveRepos(store); err != nil {
		return RepoEntry{}, err
	}
	return entry, nil
}
