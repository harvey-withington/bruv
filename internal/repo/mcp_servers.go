package repo

// Repo-scoped MCP server configurations.
//
// Each repo has its own .bruv/mcp_servers.json file listing the MCP
// servers agents in that repo can use. Like card_types.json, this
// travels with the repo when it's shared — the project definition
// of "what tools are available" is part of the project, not the
// user's machine.
//
// Secrets referenced by these configs (environment variable values
// for API keys) do NOT live in this file. They're stored in the
// OS keychain keyed by repo ID + server name + variable name, so
// sharing a repo never leaks credentials.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bruv/internal/mcp"
)

// MCPServerStore is the on-disk root for <repo>/.bruv/mcp_servers.json.
// A thin wrapper around a slice of server specs so future schema
// additions (e.g. last-modified tracking) don't require breaking
// the file format.
type MCPServerStore struct {
	Version int              `json:"version"`
	Servers []mcp.ServerSpec `json:"servers"`
}

// mcpServersPath returns the location of the repo-scoped MCP server
// store. Parallel to cardTypesPath — both live under .bruv/ so they
// travel with the project data.
func (r *Repository) mcpServersPath() string {
	return filepath.Join(r.Root, bruvDir, "mcp_servers.json")
}

// LoadMCPServerStore reads the repo-scoped MCP server store. Returns
// an empty store (not an error) when the file does not exist —
// that's the normal state for a fresh repo before any servers have
// been configured.
func (r *Repository) LoadMCPServerStore() (MCPServerStore, error) {
	var store MCPServerStore
	data, err := os.ReadFile(r.mcpServersPath())
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return MCPServerStore{}, err
	}
	return store, nil
}

// SaveMCPServerStore writes the repo-scoped MCP server store. Ensures
// the .bruv directory exists and bumps the version marker so future
// readers can detect format changes. File mode is 0o600 because
// while the file itself contains no secret *values* (those live in
// the OS keychain), it does describe what commands the app will
// execute — not something we want world-readable.
func (r *Repository) SaveMCPServerStore(store MCPServerStore) error {
	if store.Version == 0 {
		store.Version = 1
	}
	path := r.mcpServersPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
