package http

// Multi-repo HTTP routing.
//
// Server-side (bruv-server) hosts N repos, each with its own
// runtime + bus + attachment resolver. The transport routes:
//
//   GET /repos                         → JSON list of {id, name}
//   POST /repos/<id>/rpc               → that repo's RPC dispatcher
//   GET  /repos/<id>/events            → that repo's SSE stream
//   GET  /repos/<id>/attachments/...   → that repo's signed-URL handler
//
// All paths require a valid device token via requireAuth (mounted by
// the caller in server.go's buildMux). The repo ID comes from the URL
// path — the server resolves it through the RepoBackend to get the
// per-repo runtime + bus + attachment config, then dispatches.

import (
	"encoding/json"
	nethttp "net/http"
	"reflect"
	"strings"
)

// listReposHandler responds to GET /repos with the public list of
// repos this server hosts. The Path field on RepoSummary is
// intentionally omitted — clients have no business knowing the
// server's filesystem layout.
func listReposHandler(backend RepoBackend) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		repos := backend.List()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	})
}

// repoRouter dispatches /repos/<id>/<sub>... to the per-repo handlers.
// Returns 404 when the repo ID is unknown to the backend, 400 when the
// sub-path is empty, and otherwise delegates to the RPC dispatcher,
// SSE stream, or attachment handler for that repo.
func (s *Server) repoRouter() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// Strip the "/repos/" prefix and split into <id>/<rest...>.
		trimmed := strings.TrimPrefix(r.URL.Path, "/repos/")
		slash := strings.IndexByte(trimmed, '/')
		if slash <= 0 {
			nethttp.NotFound(w, r)
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

// singleRepoListHandler is the GET /repos handler for the desktop
// loopback's single-repo mode. Asks the dispatcher target for its
// current repo via reflection (target is *App on desktop, has a
// GetCurrentRepo method that returns a struct with ID + Name) and
// emits a 1-element list. Lets the connection-tree picker render
// Local symmetrically with Remote — Local just shows up as a
// "server" with one repo. Returns an empty list if no repo is
// open or if GetCurrentRepo isn't on the target.
func singleRepoListHandler(d *Dispatcher) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		out := []RepoSummary{}
		v := reflect.ValueOf(d.target)
		m := v.MethodByName("GetCurrentRepo")
		if m.IsValid() {
			results := m.Call(nil)
			if len(results) == 1 && !results[0].IsNil() {
				info := results[0].Elem()
				idField := info.FieldByName("ID")
				nameField := info.FieldByName("Name")
				if idField.IsValid() && nameField.IsValid() {
					out = append(out, RepoSummary{
						ID:   idField.String(),
						Name: nameField.String(),
					})
				}
			}
		}

		_ = json.NewEncoder(w).Encode(out)
	})
}

// localRegistryListHandler is the GET /repos handler for the desktop
// loopback when wired with a registry callback. Mirrors the multi-repo
// listReposHandler — both render whatever the host considers "the set
// of repos this connection knows about" — so the frontend's picker
// code path is identical for Local and Remote.
func localRegistryListHandler(reg *LocalRegistryConfig) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		out := reg.List()
		if out == nil {
			out = []RepoSummary{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

// localRegistryEnableRouter handles POST /repos/<id>/(enable|disable)
// in single-repo desktop mode. Anything else under /repos/<id>/ 404s
// — the desktop only has one repo open at a time, so per-repo /rpc /
// /events / /attachments routes stay flat.
func localRegistryEnableRouter(reg *LocalRegistryConfig) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/repos/")
		slash := strings.IndexByte(trimmed, '/')
		if slash <= 0 {
			nethttp.NotFound(w, r)
			return
		}
		repoID := trimmed[:slash]
		sub := trimmed[slash+1:]
		var enable bool
		switch sub {
		case "enable":
			enable = true
		case "disable":
			enable = false
		default:
			nethttp.NotFound(w, r)
			return
		}
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
			return
		}
		if err := reg.SetEnabled(repoID, enable); err != nil {
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
