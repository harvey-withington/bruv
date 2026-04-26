package http

// Multi-repo HTTP routing.
//
// The transport routes:
//
//   GET    /repos                        list of {id,name,disabled}
//   POST   /repos                        init-or-open (body: {path,name?})
//   POST   /repos/inspect                inspect a path before commit
//   PATCH  /repos/<id>                   rename
//   DELETE /repos/<id>                   remove from registry
//   POST   /repos/<id>/(enable|disable)  toggle
//   POST   /repos/<id>/rpc               that repo's RPC dispatcher
//   GET    /repos/<id>/events            that repo's SSE stream
//   GET    /repos/<id>/attachments/...   that repo's signed-URL handler
//
// All paths require a valid device token via requireAuth (mounted by
// the caller in server.go's buildMux). The repo ID comes from the URL
// path — the server resolves it through the RepoBackend to get the
// per-repo runtime + bus + attachment config, then dispatches.

import (
	"encoding/json"
	nethttp "net/http"
	"strings"
)

// RepoInspect is the result of POST /repos/inspect — surfaces just
// enough for the UI to decide between "Open this existing repo"
// (Exists=true, Name set) and "Name your new repo" (Exists=false).
type RepoInspect struct {
	Exists bool   `json:"exists"`
	Name   string `json:"name,omitempty"`
	ID     string `json:"id,omitempty"`
}

// reposCollectionHandler dispatches the /repos top-level resource:
//   GET  → list
//   POST → init-or-open at the body's path
// Anything else → 405.
//
// POST /repos with {path, name?} routes through InitOrOpen on the
// backend, which inspects the path and either Init's a fresh repo
// (when name is supplied + path isn't a BRUV repo yet) or Open's the
// existing one. Returns the RepoSummary so the client can stamp the
// new ID into its connection's repo-recents and reload.
func reposCollectionHandler(backend RepoBackend) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.Method {
		case nethttp.MethodGet:
			repos := backend.List()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repos)
		case nethttp.MethodPost:
			var body struct {
				Path string `json:"path"`
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				nethttp.Error(w, "invalid body: "+err.Error(), nethttp.StatusBadRequest)
				return
			}
			if body.Path == "" {
				nethttp.Error(w, "path is required", nethttp.StatusBadRequest)
				return
			}
			summary, err := backend.InitOrOpen(body.Path, body.Name)
			if err != nil {
				nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(summary)
		default:
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		}
	})
}

// reposInspectHandler handles POST /repos/inspect — pure read, no
// side effects. Tells the client whether the path is already a BRUV
// repo (returning name + manifest ID) or a candidate folder for init.
func reposInspectHandler(backend RepoBackend) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			nethttp.Error(w, "invalid body: "+err.Error(), nethttp.StatusBadRequest)
			return
		}
		if body.Path == "" {
			nethttp.Error(w, "path is required", nethttp.StatusBadRequest)
			return
		}
		out, err := backend.Inspect(body.Path)
		if err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// repoRouter dispatches /repos/<id>/<sub>... to the per-repo handlers
// and the /repos/inspect special-case.
func (s *Server) repoRouter() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// /repos/inspect — POST-only, special-cased ahead of the
		// per-repo routing because "inspect" looks like a repo ID
		// otherwise.
		trimmed := strings.TrimPrefix(r.URL.Path, "/repos/")
		if trimmed == "inspect" {
			reposInspectHandler(s.cfg.Repos).ServeHTTP(w, r)
			return
		}

		slash := strings.IndexByte(trimmed, '/')
		// No slash → /repos/<id> with no sub-path. Handles PATCH (rename)
		// and DELETE (remove). Anything else → 405.
		if slash < 0 {
			repoID := trimmed
			if repoID == "" {
				nethttp.NotFound(w, r)
				return
			}
			switch r.Method {
			case nethttp.MethodPatch:
				repoRenameHandler(s.cfg.Repos, repoID).ServeHTTP(w, r)
			case nethttp.MethodDelete:
				repoRemoveHandler(s.cfg.Repos, repoID).ServeHTTP(w, r)
			default:
				nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			}
			return
		}
		repoID := trimmed[:slash]
		sub := trimmed[slash+1:] // "rpc", "events", "attachments/<cardID>/<id>"

		// Enable/disable are server-level supervisor calls; they DON'T
		// require the per-repo dispatcher (the repo might be disabled
		// → no runtime, no dispatcher). Handle these before the
		// resolve-or-404 below.
		switch sub {
		case "enable":
			repoEnableHandler(s.cfg.Repos, true).ServeHTTP(w, r)
			return
		case "disable":
			repoEnableHandler(s.cfg.Repos, false).ServeHTTP(w, r)
			return
		}

		dispatcher, target := s.dispatcherFor(repoID)
		if dispatcher == nil || target == nil {
			nethttp.NotFound(w, r)
			return
		}

		switch {
		case sub == "rpc":
			dispatcher.Handler().ServeHTTP(w, r)
		case sub == "events":
			sseHandler(target.Bus).ServeHTTP(w, r)
		case strings.HasPrefix(sub, "attachments/"):
			if target.Attachments == nil {
				nethttp.NotFound(w, r)
				return
			}
			// The attachment handler expects URLs to start with
			// "/attachments/<cardID>/<id>" — rewrite the request URL
			// so the handler's prefix-trim still works.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/" + sub
			attachmentHandler(target.Attachments).ServeHTTP(w, r2)
		default:
			nethttp.NotFound(w, r)
		}
	})
}

// repoRenameHandler handles PATCH /repos/<id> {name}.
func repoRenameHandler(backend RepoBackend, id string) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			nethttp.Error(w, "invalid body: "+err.Error(), nethttp.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			nethttp.Error(w, "name is required", nethttp.StatusBadRequest)
			return
		}
		if err := backend.Rename(id, body.Name); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
	})
}

// repoRemoveHandler handles DELETE /repos/<id>. The folder on disk
// is left alone; this only drops the registry entry + unloads the
// runtime if loaded.
func repoRemoveHandler(backend RepoBackend, id string) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if err := backend.Remove(id); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
	})
}

// repoEnableHandler handles POST /repos/<id>/enable and
// POST /repos/<id>/disable. The target's Resolve isn't used (this
// is a supervisor-level operation, not per-repo dispatch), so we
// route by the URL path manually before this handler runs — see
// server.go's mux setup.
func repoEnableHandler(backend RepoBackend, enable bool) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		// Strip "/repos/" then take everything before the next "/".
		trimmed := strings.TrimPrefix(r.URL.Path, "/repos/")
		slash := strings.IndexByte(trimmed, '/')
		if slash <= 0 {
			nethttp.Error(w, "missing repo id", nethttp.StatusBadRequest)
			return
		}
		repoID := trimmed[:slash]
		if err := backend.SetEnabled(repoID, enable); err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
	})
}
