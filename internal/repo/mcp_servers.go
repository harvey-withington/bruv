package repo

// Repo-scoped MCP server configurations.
//
// Each repo has its own mcp_servers.json file (at the repo root) listing the MCP
// servers agents in that repo can use. Like card_types.json, this
// travels with the repo when it's shared — the project definition
// of "what tools are available" is part of the project, not the
// user's machine.
//
// Secrets referenced by these configs (environment variable values
// for API keys) do NOT live in this file. They're stored in the
// OS keychain keyed by repo ID + server name + variable name, so
// sharing a repo never leaks credentials. And because the file travels,
// its "enabled" flag never runs anything by itself: each machine must
// approve a server's command before it is spawned (see
// internal/mcp/approval.go and internal/config/mcp_approvals.go).

import (
	"encoding/json"
	"os"
	"path/filepath"

	"bruv/internal/fsutil"
	"bruv/internal/mcp"
)

// MCPServerStore is the on-disk root for <repo>/mcp_servers.json.
// A thin wrapper around a slice of server specs so future schema
// additions (e.g. last-modified tracking) don't require breaking
// the file format.
type MCPServerStore struct {
	Version int              `json:"version"`
	Servers []mcp.ServerSpec `json:"servers"`
}

// mcpServersPath returns the location of the repo-scoped MCP server
// store. Lives at the repo root so the project's tool definitions
// travel when the repo is shared (secrets stay in the OS keychain).
func mcpServersPath(root string) string {
	return filepath.Join(root, "mcp_servers.json")
}

// LoadMCPServerStore reads the repo-scoped MCP server store. Returns
// an empty store (not an error) when the file does not exist —
// that's the normal state for a fresh repo before any servers have
// been configured.
func (r *Repository) LoadMCPServerStore() (MCPServerStore, error) {
	return LoadMCPServerStoreAt(r.Root)
}

// LoadMCPServerStoreAt reads the MCP server store of the repo rooted at
// root without opening the repo — used by the one-time MCP approval
// migration, which inspects every repo registered on this machine.
func LoadMCPServerStoreAt(root string) (MCPServerStore, error) {
	var store MCPServerStore
	data, err := os.ReadFile(mcpServersPath(root))
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

// SaveMCPServerStore writes the repo-scoped MCP server store and bumps
// the version marker so future
// readers can detect format changes. File mode is 0o600 because
// while the file itself contains no secret *values* (those live in
// the OS keychain), it does describe what commands the app will
// execute — not something we want world-readable.
func (r *Repository) SaveMCPServerStore(store MCPServerStore) error {
	if store.Version == 0 {
		store.Version = 1
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(mcpServersPath(r.Root), data, 0o600)
}
