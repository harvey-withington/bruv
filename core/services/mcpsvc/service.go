// Package mcpsvc is the MCPService — CRUD for MCP server configs and
// keychain-backed secret management. Named mcpsvc (not mcp) to avoid
// colliding with the internal/mcp runtime package.
//
// The registry lifecycle (spawn/shutdown subprocesses) stays on the
// host via Deps.ReloadRegistry because it's tied to repo lifecycle
// and the OS keychain secret resolver.
package mcpsvc

import (
	"bruv/internal/config"
	"bruv/internal/mcp"
	"bruv/internal/repo"
	"fmt"
	"strings"
)

// ServerView is the frontend-friendly shape for rendering one server
// in the Settings UI: merges static config (Spec), live health, and
// the namespaced tool list so the UI doesn't have to reconcile.
type ServerView struct {
	Spec   mcp.ServerSpec   `json:"spec"`
	Health mcp.ServerHealth `json:"health"`
	Tools  []ServerViewTool `json:"tools"`
}

// ServerViewTool mirrors mcp.NamespacedTool but flattens for UI.
type ServerViewTool struct {
	Name        string `json:"name"`         // plain name as the server returns it
	NamespaceID string `json:"namespace_id"` // server__tool, used by allowed_tools lists
	Description string `json:"description"`
}

// Deps is the narrow host contract for MCPService.
type Deps interface {
	Repo() *repo.Repository
	Registry() *mcp.Registry
	// ReloadRegistry tears down and rebuilds the live registry after
	// config mutations. Implemented by the host because registry
	// lifecycle is tied to the OS keychain resolver and repo open state.
	ReloadRegistry()
}

// Service performs MCP server config CRUD.
type Service struct{ deps Deps }

// New constructs an MCPService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// List returns every configured server for the current repo including
// disabled/failed ones, merged with live health from the registry.
func (s *Service) List() ([]ServerView, error) {
	r := s.deps.Repo()
	if r == nil {
		return []ServerView{}, nil
	}
	store, err := r.LoadMCPServerStore()
	if err != nil {
		return nil, fmt.Errorf("load mcp server store: %w", err)
	}

	var health map[string]mcp.ServerHealth
	if reg := s.deps.Registry(); reg != nil {
		healthList := reg.Health()
		health = make(map[string]mcp.ServerHealth, len(healthList))
		for _, h := range healthList {
			health[h.Name] = h
		}
	}

	var toolsByServer map[string][]mcp.Tool
	if reg := s.deps.Registry(); reg != nil {
		toolsByServer = reg.ToolsByServer()
	}

	out := make([]ServerView, 0, len(store.Servers))
	for _, spec := range store.Servers {
		view := ServerView{Spec: spec}
		if h, ok := health[spec.Name]; ok {
			view.Health = h
		} else {
			view.Health = mcp.ServerHealth{Name: spec.Name, Status: mcp.HealthDisabled}
		}
		view.Tools = make([]ServerViewTool, 0)
		if tools, ok := toolsByServer[spec.Name]; ok {
			for _, t := range tools {
				view.Tools = append(view.Tools, ServerViewTool{
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

// Add appends a new server to the repo config and triggers a registry
// reload so its tools become immediately available.
func (s *Service) Add(spec mcp.ServerSpec) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := validateServerSpec(spec); err != nil {
		return err
	}
	store, err := r.LoadMCPServerStore()
	if err != nil {
		return fmt.Errorf("load mcp server store: %w", err)
	}
	for _, existing := range store.Servers {
		if strings.EqualFold(existing.Name, spec.Name) {
			return fmt.Errorf("server %q already exists", spec.Name)
		}
	}
	store.Servers = append(store.Servers, spec)
	if err := r.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}
	s.deps.ReloadRegistry()
	return nil
}

// Update replaces an existing server's spec in place. Name is immutable.
func (s *Service) Update(spec mcp.ServerSpec) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if err := validateServerSpec(spec); err != nil {
		return err
	}
	store, err := r.LoadMCPServerStore()
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
	if err := r.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}
	s.deps.ReloadRegistry()
	return nil
}

// Delete removes a server and purges its keychain secrets. Keychain
// cleanup is best-effort — an orphaned secret is strictly better than
// a server that won't delete.
func (s *Service) Delete(name string, logWarn func(server, env string, err error)) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := r.LoadMCPServerStore()
	if err != nil {
		return fmt.Errorf("load mcp server store: %w", err)
	}
	var deleted *mcp.ServerSpec
	filtered := store.Servers[:0]
	for _, spec := range store.Servers {
		if spec.Name == name {
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
	if err := r.SaveMCPServerStore(store); err != nil {
		return fmt.Errorf("save mcp server store: %w", err)
	}

	for _, envName := range deleted.EnvNames {
		if err := config.DeleteMCPSecret(r.Manifest.ID, deleted.Name, envName); err != nil && logWarn != nil {
			logWarn(deleted.Name, envName, err)
		}
	}
	s.deps.ReloadRegistry()
	return nil
}

// SetSecret stores one env var value in the OS keychain. Empty values
// clear the entry rather than storing an empty string.
func (s *Service) SetSecret(serverName, envVarName, value string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	if serverName == "" || envVarName == "" {
		return fmt.Errorf("server name and env var name are required")
	}
	if value == "" {
		return config.DeleteMCPSecret(r.Manifest.ID, serverName, envVarName)
	}
	return config.SetMCPSecret(r.Manifest.ID, serverName, envVarName, value)
}

// SecretStatus reports presence of each declared env var's secret
// without returning the value itself.
func (s *Service) SecretStatus(serverName string) (map[string]bool, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	store, err := r.LoadMCPServerStore()
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
		_, ok := config.GetMCPSecret(r.Manifest.ID, serverName, name)
		out[name] = ok
	}
	return out, nil
}

// Restart tears down and re-starts a single server. Implemented as a
// full registry reload — simpler and keeps the single reload codepath.
func (s *Service) Restart(name string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	s.deps.ReloadRegistry()
	return nil
}

// validateServerSpec enforces add/update invariants: non-empty name,
// non-empty command, no namespace separator in name, unique env names.
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
