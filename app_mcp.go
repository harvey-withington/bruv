package main

// Wails-bound methods for managing MCP servers from the frontend.
//
// All methods operate on the current repo's MCP registry and the
// corresponding repo-scoped config file. They're grouped here
// rather than in app.go to keep the MCP integration self-contained
// — a future extraction of MCP into its own sub-package becomes
// easier if the file split already mirrors the feature boundary.

import (
	"bruv/internal/config"
	"bruv/internal/mcp"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// MCPServerView is the frontend-friendly shape for rendering one
// server in the Settings UI. It merges the static config (from
// ServerSpec) with the live health state (from Registry.Health)
// so the UI doesn't have to do two calls and then reconcile.
type MCPServerView struct {
	Spec   mcp.ServerSpec   `json:"spec"`
	Health mcp.ServerHealth `json:"health"`
	// Tools is the list of tools this server exposes, with their
	// namespaced IDs pre-computed for per-card permission toggles.
	Tools []MCPServerViewTool `json:"tools"`
}

// MCPServerViewTool mirrors mcp.NamespacedTool but flattens the
// schema for UI consumption.
type MCPServerViewTool struct {
	Name        string `json:"name"`          // plain tool name as the server returns it
	NamespaceID string `json:"namespace_id"`  // server__tool, used by allowed_tools lists
	Description string `json:"description"`
}

// ListMCPServers returns every configured server for the current
// repo, including disabled and failed ones, so the UI can show the
// full list with accurate health.
func (a *App) ListMCPServers() ([]MCPServerView, error) {
	if a.repo == nil {
		return []MCPServerView{}, nil
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		return nil, fmt.Errorf("load mcp server store: %w", err)
	}

	// Map health entries by name so we can merge in O(1) — the
	// registry only has entries for servers that were loaded at
	// registry start, but the store might have servers added after
	// (not currently possible because AddMCPServer reloads, but
	// defensive in case that changes).
	var health map[string]mcp.ServerHealth
	if a.mcpRegistry != nil {
		healthList := a.mcpRegistry.Health()
		health = make(map[string]mcp.ServerHealth, len(healthList))
		for _, h := range healthList {
			health[h.Name] = h
		}
	}

	var toolsByServer map[string][]mcp.Tool
	if a.mcpRegistry != nil {
		toolsByServer = a.mcpRegistry.ToolsByServer()
	}

	out := make([]MCPServerView, 0, len(store.Servers))
	for _, spec := range store.Servers {
		view := MCPServerView{Spec: spec}
		if h, ok := health[spec.Name]; ok {
			view.Health = h
		} else {
			view.Health = mcp.ServerHealth{
				Name:   spec.Name,
				Status: mcp.HealthDisabled,
			}
		}
		// Ensure all slice fields marshal as [] not null — Svelte's
		// {#each} calls .length on the value and blows up on null.
		view.Tools = make([]MCPServerViewTool, 0)
		if tools, ok := toolsByServer[spec.Name]; ok {
			for _, t := range tools {
				view.Tools = append(view.Tools, MCPServerViewTool{
					Name:        t.Name,
					NamespaceID: mcp.NamespaceTool(spec.Name, t.Name),
					Description: t.Description,
				})
			}
		}
		if view.Spec.Args == nil {
			view.Spec.Args = []string{}
		}
		if view.Spec.EnvNames == nil {
			view.Spec.EnvNames = []string{}
		}
		out = append(out, view)
	}
	return out, nil
}

// AddMCPServer appends a new server to the repo config, persists,
// and reloads the registry so the new server's tools become
// available immediately. Returns the populated view for the caller
// to render, or an error if validation/reload failed.
//
// Validation: names must be non-empty, unique within the repo,
// and safe to use as a keychain key component (see mcp_secrets.go
// for the sanitiser). Command must be non-empty.
func (a *App) AddMCPServer(spec mcp.ServerSpec) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := validateServerSpec(spec); err != nil {
		return err
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		return fmt.Errorf("load mcp server store: %w", err)
	}
	for _, existing := range store.Servers {
		if strings.EqualFold(existing.Name, spec.Name) {
			return fmt.Errorf("server %q already exists", spec.Name)
		}
	}
	store.Servers = append(store.Servers, spec)
	if err := a.repo.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}
	a.reloadMCPRegistry()
	return nil
}

// UpdateMCPServer replaces an existing server's spec in place.
// Name is immutable — to rename, delete and re-add. This avoids
// an awkward keychain-secrets migration path when a server's
// name is part of the secret keys.
func (a *App) UpdateMCPServer(spec mcp.ServerSpec) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := validateServerSpec(spec); err != nil {
		return err
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		return fmt.Errorf("load mcp server store: %w", err)
	}
	found := false
	for i, existing := range store.Servers {
		if existing.Name == spec.Name {
			store.Servers[i] = spec
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("server %q not found", spec.Name)
	}
	if err := a.repo.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}
	a.reloadMCPRegistry()
	return nil
}

// DeleteMCPServer removes a server from the repo config and purges
// its keychain secrets. Reloads the registry so the deleted
// server's tools disappear from the agent catalogue immediately.
//
// Keychain cleanup is best-effort: missing entries are not an
// error, but other errors are logged. We never block the delete
// on keychain state because an orphaned secret is strictly better
// than a server that won't delete.
func (a *App) DeleteMCPServer(name string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		return fmt.Errorf("load mcp server store: %w", err)
	}
	var deleted *mcp.ServerSpec
	filtered := store.Servers[:0]
	for _, spec := range store.Servers {
		if spec.Name == name {
			// Snapshot so we can purge its secrets below.
			copy := spec
			deleted = &copy
			continue
		}
		filtered = append(filtered, spec)
	}
	if deleted == nil {
		return fmt.Errorf("server %q not found", name)
	}
	store.Servers = filtered
	if err := a.repo.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}

	// Purge keychain entries for this server's declared env var
	// names. Only names we know about — orphaned secrets from
	// renamed servers would be cleaned by a future tombstone
	// process.
	for _, envName := range deleted.EnvNames {
		if err := config.DeleteMCPSecret(a.repo.Manifest.ID, deleted.Name, envName); err != nil {
			slog.Warn("mcp delete secret failed",
				"server", deleted.Name, "env", envName, "err", err)
		}
	}

	a.reloadMCPRegistry()
	return nil
}

// SetMCPServerSecret stores a single env var value in the OS
// keychain for this repo + server. Called from the Settings UI
// when the user pastes an API key. Empty values clear the entry
// rather than storing an empty string — simpler to reason about
// and matches user expectation of "clear this field".
func (a *App) SetMCPServerSecret(serverName, envVarName, value string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if serverName == "" || envVarName == "" {
		return fmt.Errorf("server name and env var name are required")
	}
	if value == "" {
		return config.DeleteMCPSecret(a.repo.Manifest.ID, serverName, envVarName)
	}
	return config.SetMCPSecret(a.repo.Manifest.ID, serverName, envVarName, value)
}

// GetMCPServerSecretStatus reports whether a secret is present for
// each of the declared env vars on a given server, WITHOUT
// returning the actual values. The UI uses this to show "(set)"
// vs "(not set)" badges next to each variable in the edit form
// — displaying secret values in the UI is never desirable.
func (a *App) GetMCPServerSecretStatus(serverName string) (map[string]bool, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		return nil, err
	}
	var spec *mcp.ServerSpec
	for i := range store.Servers {
		if store.Servers[i].Name == serverName {
			spec = &store.Servers[i]
			break
		}
	}
	if spec == nil {
		return nil, fmt.Errorf("server %q not found", serverName)
	}
	out := make(map[string]bool, len(spec.EnvNames))
	for _, name := range spec.EnvNames {
		_, ok := config.GetMCPSecret(a.repo.Manifest.ID, serverName, name)
		out[name] = ok
	}
	return out, nil
}

// RestartMCPServer tears down and re-starts a single server without
// touching the others. Used by the UI's "retry" button after a
// failed startup, and after the user pastes a previously-missing
// secret.
func (a *App) RestartMCPServer(name string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Simplest correct implementation: full reload. The alternative
	// (per-server restart without touching siblings) is a nice
	// optimisation but risks inconsistent state during the
	// transition. A full reload is a few seconds at most for a
	// typical config and lets us reuse the same code path we
	// already test.
	a.reloadMCPRegistry()
	return nil
}

// reloadMCPRegistry is the single path by which MCP config changes
// take effect. Stops any existing registry, reads the current
// store, and spawns a fresh registry. Called from every mutation
// method so the caller never has to remember to reload.
func (a *App) reloadMCPRegistry() {
	if a.repo == nil {
		return
	}
	if a.mcpRegistry != nil {
		a.mcpRegistry.Shutdown()
		a.mcpRegistry = nil
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		slog.Warn("mcp reload: load store failed", "err", err)
		return
	}
	reg := mcp.NewRegistry(a.repo.Manifest.ID, config.MCPSecretResolver{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errs := reg.LoadAndStart(ctx, store.Servers)
	for name, err := range errs {
		slog.Warn("mcp reload failed", "server", name, "err", err)
	}
	a.mcpRegistry = reg
}

// validateServerSpec enforces the invariants AddMCPServer and
// UpdateMCPServer rely on: non-empty name, non-empty command,
// name components don't contain the MCP namespace separator so
// SplitNamespacedTool round-trips cleanly.
func validateServerSpec(spec mcp.ServerSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("server name is required")
	}
	if strings.Contains(spec.Name, mcp.NamespaceSeparator) {
		return fmt.Errorf("server name must not contain %q (used as tool namespace separator)", mcp.NamespaceSeparator)
	}
	if strings.TrimSpace(spec.Command) == "" {
		return fmt.Errorf("command is required")
	}
	// Env var names must be unique within a single server spec.
	seen := make(map[string]bool, len(spec.EnvNames))
	for _, name := range spec.EnvNames {
		if name == "" {
			return fmt.Errorf("env variable names must not be empty")
		}
		if seen[name] {
			return fmt.Errorf("duplicate env variable name %q", name)
		}
		seen[name] = true
	}
	return nil
}
