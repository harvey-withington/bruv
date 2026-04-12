package mcp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

// NamespaceSeparator joins a server's name to a tool name when
// constructing the namespaced ID used by the agent tool catalogue.
// Two underscores keeps collisions with real tool names vanishingly
// unlikely and is visually distinct in debug logs. The UI displays
// tools with the prefix stripped so users see clean names.
const NamespaceSeparator = "__"

// Registry manages the full set of MCP servers for one open repo.
// It owns a map of ServerProcess values, a tool-name index for
// fast lookup during agent dispatch, and the per-repo config
// lifecycle (load, add, update, delete, save).
//
// The Registry is the single point of contact between the BRUV app
// layer and the MCP subsystem. app.go never touches ServerProcess
// or Client directly — it goes through the Registry.
//
// Concurrency: the Registry is safe for concurrent use. Mutations
// (add/update/delete/reload) are serialised by mu. Reads
// (Tools/CallTool/Health) take the lock briefly and copy.
type Registry struct {
	repoID   string
	resolver SecretResolver

	mu      sync.Mutex
	servers map[string]*ServerProcess

	// toolIndex maps a namespaced tool ID (server__tool) to the
	// server that owns it. Rebuilt on every mutation. Exists so
	// tool dispatch is O(1) not O(servers × tools).
	toolIndex map[string]*ServerProcess
}

// NewRegistry creates an empty Registry for the given repo. Call
// LoadAndStart to populate it from config and spawn the servers.
func NewRegistry(repoID string, resolver SecretResolver) *Registry {
	return &Registry{
		repoID:    repoID,
		resolver:  resolver,
		servers:   make(map[string]*ServerProcess),
		toolIndex: make(map[string]*ServerProcess),
	}
}

// LoadAndStart brings up every server in specs. Returns a per-server
// error map — startup failures don't abort the whole load, so a
// broken config for one server doesn't take out the rest.
//
// The caller supplies the full list of specs (from per-repo config).
// Disabled servers are created but not started.
func (r *Registry) LoadAndStart(ctx context.Context, specs []ServerSpec) map[string]error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop any existing servers first — LoadAndStart is idempotent
	// so calling it twice rebuilds the whole registry. This makes
	// the "reload config after user edit" flow trivial.
	for name, sp := range r.servers {
		if err := sp.Stop(); err != nil {
			log.Printf("mcp: stop %q during reload: %v", name, err)
		}
	}
	r.servers = make(map[string]*ServerProcess)
	r.toolIndex = make(map[string]*ServerProcess)

	errs := make(map[string]error)
	for _, spec := range specs {
		if spec.Name == "" {
			errs["(unnamed)"] = fmt.Errorf("server config has empty name")
			continue
		}
		if _, exists := r.servers[spec.Name]; exists {
			errs[spec.Name] = fmt.Errorf("duplicate server name")
			continue
		}
		sp := NewServerProcess(spec, r.repoID, r.resolver)
		r.servers[spec.Name] = sp

		if !spec.Enabled {
			continue
		}
		if err := sp.Start(ctx); err != nil {
			errs[spec.Name] = err
			log.Printf("mcp[%s]: failed to start: %v", spec.Name, err)
			continue
		}
		r.indexServerTools(sp)
	}
	return errs
}

// indexServerTools adds every tool from sp to the registry's
// toolIndex under its namespaced ID. Assumes r.mu is held.
func (r *Registry) indexServerTools(sp *ServerProcess) {
	for _, tool := range sp.Tools() {
		id := NamespaceTool(sp.Spec().Name, tool.Name)
		r.toolIndex[id] = sp
	}
}

// Shutdown stops every server in the registry. Called when the repo
// is closed or the app is exiting. Errors are logged but not
// returned — at shutdown time nothing useful can be done with them.
func (r *Registry) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, sp := range r.servers {
		if err := sp.Stop(); err != nil {
			log.Printf("mcp[%s]: shutdown: %v", name, err)
		}
	}
	r.servers = make(map[string]*ServerProcess)
	r.toolIndex = make(map[string]*ServerProcess)
}

// Tools returns every ready tool across every enabled server, each
// identified by its namespaced ID. Used by the agent tool catalogue
// to advertise MCP tools alongside built-in ones.
//
// The returned slice is independent of the registry's internal
// state — safe to hold and iterate after subsequent mutations.
func (r *Registry) Tools() []NamespacedTool {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []NamespacedTool
	for _, sp := range r.servers {
		h := sp.Health()
		if h.Status != HealthReady {
			continue
		}
		for _, tool := range sp.Tools() {
			out = append(out, NamespacedTool{
				ServerName:  sp.Spec().Name,
				Tool:        tool,
				NamespaceID: NamespaceTool(sp.Spec().Name, tool.Name),
			})
		}
	}
	return out
}

// NamespacedTool bundles a tool with its owning server name and its
// namespaced ID so callers can display either the user-friendly or
// the fully-qualified form.
type NamespacedTool struct {
	ServerName  string
	Tool        Tool
	NamespaceID string
}

// ToolsByServer groups ready tools by server name — used by the UI
// to render one section per server in the agent permissions view.
func (r *Registry) ToolsByServer() map[string][]Tool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string][]Tool)
	for name, sp := range r.servers {
		h := sp.Health()
		if h.Status != HealthReady {
			continue
		}
		out[name] = sp.Tools()
	}
	return out
}

// CallTool dispatches a namespaced tool call to the owning server.
// Returns an error if no server in the registry owns the given ID —
// typically because the user disabled the server or renamed it
// between config reload and this call.
func (r *Registry) CallTool(ctx context.Context, namespacedID string, args map[string]interface{}) (*CallToolResult, error) {
	r.mu.Lock()
	sp, ok := r.toolIndex[namespacedID]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no MCP server owns tool %q", namespacedID)
	}
	serverName, toolName := SplitNamespacedTool(namespacedID)
	_ = serverName // kept for potential logging
	return sp.CallTool(ctx, toolName, args)
}

// Health returns a snapshot of every known server's state, including
// disabled and failed ones, so the Settings UI can show the user
// what's going on.
func (r *Registry) Health() []ServerHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ServerHealth, 0, len(r.servers))
	for _, sp := range r.servers {
		out = append(out, sp.Health())
	}
	return out
}

// Specs returns the current ServerSpec for every registered server,
// including disabled ones. Used by the Settings UI to render the
// add/edit/delete list.
func (r *Registry) Specs() []ServerSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ServerSpec, 0, len(r.servers))
	for _, sp := range r.servers {
		out = append(out, sp.Spec())
	}
	return out
}

// OwnsTool reports whether the given namespaced ID is one of the
// MCP tools this registry knows about. Used by the agent dispatch
// loop to decide whether to route a tool call through the registry
// or through the built-in tool switch.
func (r *Registry) OwnsTool(namespacedID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.toolIndex[namespacedID]
	return ok
}

// NamespaceTool joins a server name and a tool name into the
// namespaced ID used throughout the agent subsystem. The reverse
// operation is SplitNamespacedTool.
func NamespaceTool(serverName, toolName string) string {
	return serverName + NamespaceSeparator + toolName
}

// SplitNamespacedTool reverses NamespaceTool. Returns the server
// name and tool name parts. If the input doesn't contain the
// separator, serverName is empty and toolName is the whole input —
// this is the fallback for built-in tools that never went through
// the namespacing path.
func SplitNamespacedTool(namespacedID string) (serverName, toolName string) {
	idx := strings.Index(namespacedID, NamespaceSeparator)
	if idx < 0 {
		return "", namespacedID
	}
	return namespacedID[:idx], namespacedID[idx+len(NamespaceSeparator):]
}
