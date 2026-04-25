package main

// Wails-bound forwarders for MCP server management. Domain logic lives
// in core/services/mcpsvc. Registry lifecycle (reloadMCPRegistry) stays
// on App because it depends on the OS keychain secret resolver and
// repo open state — the service triggers reloads via the Deps callback.

import (
	"bruv/core/services/mcpsvc"
	"bruv/internal/config"
	"bruv/internal/mcp"
	"bruv/internal/repo"
	"context"
	"log/slog"
	"time"
)

// MCPServerView is the Wails-bound response shape. Aliased to the
// service type so frontend TS bindings remain stable.
type MCPServerView = mcpsvc.ServerView

// MCPServerViewTool is the Wails-bound tool shape. Aliased to the
// service type.
type MCPServerViewTool = mcpsvc.ServerViewTool

// mcpDeps adapts App to mcpsvc.Deps without exposing repo/registry
// lifecycle on App's public Wails surface.
type mcpDeps struct{ app *App }

func (d mcpDeps) Repo() *repo.Repository    { return d.app.repo }
func (d mcpDeps) Registry() *mcp.Registry   { return d.app.mcpRegistry }
func (d mcpDeps) ReloadRegistry()           { d.app.reloadMCPRegistry() }

// ListMCPServers returns every configured server for the current repo.
func (a *App) ListMCPServers() ([]MCPServerView, error) {
	return a.mcpService.List()
}

// AddMCPServer appends a new server and reloads the registry.
func (a *App) AddMCPServer(spec mcp.ServerSpec) error {
	return a.mcpService.Add(spec)
}

// UpdateMCPServer replaces an existing server's spec in place.
func (a *App) UpdateMCPServer(spec mcp.ServerSpec) error {
	return a.mcpService.Update(spec)
}

// DeleteMCPServer removes a server and purges its keychain secrets.
func (a *App) DeleteMCPServer(name string) error {
	return a.mcpService.Delete(name, func(server, env string, err error) {
		slog.Warn("mcp delete secret failed", "server", server, "env", env, "err", err)
	})
}

// SetMCPServerSecret stores a single env var value in the OS keychain.
func (a *App) SetMCPServerSecret(serverName, envVarName, value string) error {
	return a.mcpService.SetSecret(serverName, envVarName, value)
}

// GetMCPServerSecretStatus reports presence of each declared secret.
func (a *App) GetMCPServerSecretStatus(serverName string) (map[string]bool, error) {
	return a.mcpService.SecretStatus(serverName)
}

// RestartMCPServer tears down and re-starts a single server.
func (a *App) RestartMCPServer(name string) error {
	return a.mcpService.Restart(name)
}

// reloadMCPRegistry is the single path by which MCP config changes
// take effect. Kept on App because registry lifecycle ties to the
// OS keychain resolver and the repo manifest.
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
