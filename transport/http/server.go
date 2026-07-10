package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	nethttp "net/http"
	"runtime"
	"sync"
	"time"

	"bruv/core/events"
)

// Config holds server construction inputs. Addr is the TCP listen
// address ("127.0.0.1:0" for random loopback port in desktop mode).
//
// All hosts run the multi-repo transport since the 2026-04-26
// "local-as-remote" pivot — Repos is required. Routes:
//   GET    /repos                        list of {id,name,disabled}
//   POST   /repos                        init-or-open (body: {path,name?})
//   POST   /repos/inspect                inspect path (body: {path})
//   PATCH  /repos/<id>                   rename (body: {name})
//   DELETE /repos/<id>                   remove from registry
//   POST   /repos/<id>/(enable|disable)  toggle
//   POST   /repos/<id>/rpc               per-repo dispatcher
//   GET    /repos/<id>/events            per-repo SSE
//   GET    /repos/<id>/attachments/...   signed-URL handler
//   POST   /server/rpc                   per-machine dispatcher
//                                        (when MachineTarget is set)
//   GET    /pair?token=<bootstrap>        operator-facing pairing page
//                                        (renders QR for /m/enrol)
//   GET    /app/                          desktop Svelte bundle
//                                        (when StaticAssets is set)
//   GET    /m/                            mobile PWA bundle
//                                        (when MobileAssets is set)
type Config struct {
	Addr      string
	ConfigDir string
	Version   string
	BuildDate string
	// StaticAssets optionally embeds the Svelte frontend so the server
	// can serve the bundle at /app/*. Set from the main binary's own
	// //go:embed directive. Leave nil in headless server builds that
	// don't carry UI bytes.
	StaticAssets fs.FS
	// MobileAssets optionally embeds the mobile PWA bundle, served at
	// /m/*. Same rules as StaticAssets — set from the main binary's
	// embed, leave nil to skip the mount. The mobile bundle is its own
	// Svelte app under /mobile/ in the source tree.
	MobileAssets fs.FS
	// Repos is the per-repo backend. Required.
	Repos RepoBackend
	// MachineTarget is the dispatcher target for /server/rpc — the
	// host's per-machine RPC surface (preferences, profile, LLM
	// accounts, etc.). Reachable when no repo is selected. Leave nil
	// to skip the route (clients then can't call any per-machine
	// method until a repo is open and the per-repo dispatcher takes
	// over). Both the desktop and the headless server set this in
	// practice.
	MachineTarget any
	// MCPHandler optionally serves the Model Context Protocol endpoint at
	// /repos/<id>/mcp, exposing each repo to external agentic chat apps
	// (Claude Desktop, etc.). The handler resolves the repo from the URL
	// itself, so the transport just forwards matching requests. Built by
	// the caller (it depends on supervisor types this package can't
	// import); leave nil to skip the route.
	MCPHandler nethttp.Handler
}

// AttachmentConfig wires the signed-URL handler. Secret is the HMAC
// key (32 bytes) used to verify URLs the desktop App generated via
// SignAttachmentURL. Resolve maps a (cardID, attachmentID) pair to a
// disk path + metadata, or returns ok=false when the attachment is
// unknown. The transport package stays free of internal/repo to
// avoid an import cycle.
type AttachmentConfig struct {
	Secret  []byte
	Resolve func(cardID, attachmentID string) (path, mime, name string, ok bool)
}

// RepoSummary is the public view of one repo on the server, returned
// by GET /repos. Path is intentionally NOT included — clients have no
// business knowing the server's filesystem layout. Disabled flags
// repos the operator has registered but turned off — clients render
// them greyed out and can request enable via the SetRepoEnabled RPC.
type RepoSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled,omitempty"`
}

// RepoTarget is everything the per-repo HTTP handlers need from one
// repo's runtime: the dispatcher target (for RPC reflection), the
// event bus (for SSE), and the attachment resolver (for signed URLs).
type RepoTarget struct {
	Target      any
	Bus         *events.MemBus
	Attachments *AttachmentConfig
}

// RepoBackend is the transport's window onto the host's per-repo
// state. Implemented by core/supervisor.HTTPAdapter. The transport
// asks it to:
//   - Resolve(id)        — look up a per-repo dispatcher target.
//   - List()             — enumerate registered repos for GET /repos.
//   - SetEnabled         — flip enable/disable.
//   - Inspect(path)      — read manifest at path without touching state.
//   - InitOrOpen         — register + load a repo at the given path.
//   - Rename             — update the registry name + manifest.
//   - Remove             — drop a repo from the registry (unloads).
//
// Repo lifecycle operations (SetEnabled, InitOrOpen, Rename, Remove)
// live here rather than on Runtime because they're registry-level —
// the runtime might not exist (disabled, just-removed).
type RepoBackend interface {
	Resolve(id string) *RepoTarget
	List() []RepoSummary
	SetEnabled(id string, enabled bool) error
	Inspect(path string) (RepoInspect, error)
	InitOrOpen(path, name string) (RepoSummary, error)
	Rename(id, name string) error
	Remove(id string) error
}

// Server is the BRUV HTTP transport. Same wiring on desktop (loopback)
// and the headless bruv-server — both run multi-repo since the
// 2026-04-26 local-as-remote pivot.
type Server struct {
	cfg     Config
	devices *DeviceStore

	// Per-repo dispatchers, built lazily on first request and cached.
	// The bus + attachment resolver come from the resolved RepoTarget
	// at request time. Guarded by dispatchersMu — concurrent HTTP
	// handlers read and write this map (e.g. the desktop's parallel
	// RPC burst on first repo load).
	dispatchersMu sync.RWMutex
	dispatchers   map[string]*dispatcherEntry

	// Per-machine dispatcher for /server/rpc, built once at construct
	// time when Config.MachineTarget is set.
	machineDispatcher *Dispatcher

	httpServer *nethttp.Server
	listener   net.Listener
}

// dispatcherEntry pairs a cached dispatcher with the target it was
// built from, so a repo whose runtime was swapped out (disable →
// enable, remove → re-add) gets a fresh dispatcher instead of one
// bound to the dead runtime.
type dispatcherEntry struct {
	d      *Dispatcher
	target any
}

// NewMulti builds a multi-repo server. Repos is required; per-repo
// dispatchers are built lazily on first lookup and cached. When
// MachineTarget is set, /server/rpc is also mounted with a dispatcher
// against that target — used for per-machine RPCs (preferences,
// LLM accounts, etc.) that don't belong to a specific repo.
func NewMulti(cfg Config) (*Server, error) {
	if cfg.Repos == nil {
		return nil, fmt.Errorf("transport.NewMulti: Config.Repos is required")
	}
	devices, err := NewDeviceStore(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("device store: %w", err)
	}
	s := &Server{
		cfg:         cfg,
		devices:     devices,
		dispatchers: make(map[string]*dispatcherEntry),
	}
	if cfg.MachineTarget != nil {
		s.machineDispatcher = NewDispatcher(cfg.MachineTarget, DefaultDeniedMethods())
	}
	return s, nil
}

// dispatcherFor returns the per-repo dispatcher, building + caching it
// on first request. Returns nil when the repo ID isn't known to the
// backend.
func (s *Server) dispatcherFor(repoID string) (*Dispatcher, *RepoTarget) {
	target := s.cfg.Repos.Resolve(repoID)
	if target == nil {
		return nil, nil
	}

	s.dispatchersMu.RLock()
	entry, ok := s.dispatchers[repoID]
	s.dispatchersMu.RUnlock()
	if ok && entry.target == target.Target {
		return entry.d, target
	}

	s.dispatchersMu.Lock()
	defer s.dispatchersMu.Unlock()
	// Re-check under the write lock — another handler may have built
	// it while we waited.
	if entry, ok := s.dispatchers[repoID]; ok && entry.target == target.Target {
		return entry.d, target
	}
	d := NewDispatcher(target.Target, DefaultDeniedMethods())
	s.dispatchers[repoID] = &dispatcherEntry{d: d, target: target.Target}
	return d, target
}

// Devices exposes the underlying device store so the desktop host
// can self-enrol at startup without round-tripping through HTTP
// (both sides are the same process).
func (s *Server) Devices() *DeviceStore { return s.devices }

// Addr returns the resolved listen address (host:port) once Start has
// bound the listener. Useful when Addr was "127.0.0.1:0".
func (s *Server) Addr() string {
	if s.listener == nil {
		return s.cfg.Addr
	}
	return s.listener.Addr().String()
}

// Start binds the listener and serves in a background goroutine.
// Returns immediately once bound so callers can capture the assigned
// port before continuing. Shutdown via Stop.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.Addr, err)
	}
	s.listener = ln

	mux := s.buildMux()
	s.httpServer = &nethttp.Server{
		// CORS wraps the whole mux so every response — including
		// /app/* static assets and preflight OPTIONS — carries the
		// right headers. The Wails webview lives at
		// wails.localhost:<port> while this server binds to
		// 127.0.0.1:<port>; without CORS every fetch gets blocked.
		Handler:      withCORS(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 0, // SSE streams need unbounded write
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != nethttp.ErrServerClosed {
			slog.Warn("http server exited with error", "err", err)
		}
	}()
	slog.Info("http transport listening", "addr", ln.Addr().String())
	return nil
}

// Stop gracefully shuts down the server with a short timeout.
func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// buildMux constructs the route table. Kept small so the listing in
// the package-level doc stays accurate.
func (s *Server) buildMux() *nethttp.ServeMux {
	mux := nethttp.NewServeMux()

	mux.HandleFunc("/healthz", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/version", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version":    s.cfg.Version,
			"build_date": s.cfg.BuildDate,
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"go_version": runtime.Version(),
		})
	})

	mux.Handle("/auth/enrol", requireBootstrap(s.devices, enrolHandler(s.devices)))

	// /pair — operator-facing pairing page. Self-authenticated via the
	// bootstrap token in ?token=, so it works in headless server mode
	// (no Wails IPC needed). Renders a QR encoding the mobile EnrolPage
	// URL for the operator's phone to scan.
	mux.Handle("/pair", pairHandler(s.cfg.ConfigDir))

	// /repos and /repos/<id>/... — multi-repo routing (used by both
	// desktop loopback and headless bruv-server since the local-as-
	// remote pivot). See package-level Config doc for the full route
	// table.
	mux.Handle("/repos", requireAuth(s.devices, reposCollectionHandler(s.cfg.Repos)))
	mux.Handle("/repos/", requireAuth(s.devices, s.repoRouter()))

	// /server/rpc — per-machine dispatcher (preferences, profile,
	// LLM accounts, signed-attachment-URL). Mounted only when the
	// host wired a MachineTarget; clients that hit it without a
	// configured target get a clean 404.
	if s.machineDispatcher != nil {
		mux.Handle("/server/rpc", requireAuth(s.devices, s.machineDispatcher.Handler()))
	}

	// Mode B: serve the embedded Svelte bundle at /app/*. Intentionally
	// unauthenticated — the bundle is the UI, not data. Data access
	// still requires a bearer token on /rpc + /events.
	if s.cfg.StaticAssets != nil {
		mux.Handle("/app/", nethttp.StripPrefix("/app/", staticHandler(s.cfg.StaticAssets)))
		// Bare /app redirects to /app/ so relative asset URLs resolve.
		mux.HandleFunc("/app", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			nethttp.Redirect(w, r, "/app/", nethttp.StatusMovedPermanently)
		})
		// Visiting the server's bare host should land on the UI rather
		// than 404. Only intercept exactly "/" so the other handlers
		// (/healthz, /version, /rpc, …) still match before this fires.
		mux.HandleFunc("/", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			if r.URL.Path != "/" {
				nethttp.NotFound(w, r)
				return
			}
			nethttp.Redirect(w, r, "/app/", nethttp.StatusFound)
		})
	}

	// Mobile PWA at /m/*. Same shape as /app/ — unauthenticated bundle
	// serving (data still gated on bearer tokens). Mounted independently
	// so a build can ship desktop-only or mobile-only assets without
	// pulling in the other.
	if s.cfg.MobileAssets != nil {
		// Manifest is server-rendered so name/short_name reflect the
		// host the user installed from — distinct PWA tiles per server
		// when one phone pairs with multiple BRUVs. Longest-pattern-
		// wins routing means this beats the /m/ subtree match below.
		mux.Handle("/m/manifest.webmanifest", mobileManifestHandler())
		// Note: /m/share isn't mounted explicitly — the manifest's
		// share_target uses GET, so the browser hits /m/share?…
		// directly. The static handler's SPA fallback serves the
		// shell, and the SPA router maps /share → SharePage which
		// reads the query string. No handler indirection needed.
		mux.Handle("/m/", nethttp.StripPrefix("/m/", staticHandler(s.cfg.MobileAssets)))
		mux.HandleFunc("/m", func(w nethttp.ResponseWriter, r *nethttp.Request) {
			nethttp.Redirect(w, r, "/m/", nethttp.StatusMovedPermanently)
		})
	}

	return mux
}
