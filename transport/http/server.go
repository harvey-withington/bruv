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
	"time"

	"bruv/core/events"
)

// Config holds server construction inputs. Addr is the TCP listen
// address ("127.0.0.1:0" for random loopback port in desktop mode).
//
// Single-repo mode (desktop loopback): pass `target` and `bus` to
// `New(...)`. Routes are flat: /rpc, /events, /attachments/*.
//
// Multi-repo mode (bruv-server): pass `Repos` here. Routes become
// /repos/<id>/rpc, /repos/<id>/events, /repos/<id>/attachments/*,
// plus a top-level GET /repos that lists every repo. The server
// builds one runtime per entry and routes by URL path.
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
	// Attachments wires the signed-URL download handler. In single-repo
	// mode the handler is mounted at /attachments/{cardID}/{id}. In
	// multi-repo mode each repo provides its own resolver via
	// RepoBackend and this top-level field is ignored.
	Attachments *AttachmentConfig
	// Repos enables multi-repo routing. Set in bruv-server mode; leave
	// nil for the desktop loopback (which only has one repo at a time).
	Repos RepoBackend

	// LocalRegistry, when set, makes single-repo (desktop loopback)
	// mode serve a registry-backed GET /repos response and
	// POST /repos/<id>/(enable|disable) endpoints — so the picker
	// can render Local symmetrically with Remote without a real
	// supervisor on the desktop side. Ignored when Repos is set
	// (multi-repo mode uses the RepoBackend itself).
	LocalRegistry *LocalRegistryConfig
}

// LocalRegistryConfig wires the desktop's repos.json into the
// transport's single-repo routes. Both fields are required when set.
// The List callback supplies GET /repos; SetEnabled supplies the
// enable/disable POSTs.
type LocalRegistryConfig struct {
	List       func() []RepoSummary
	SetEnabled func(id string, enabled bool) error
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

// RepoBackend is what the server passes in for multi-repo mode. The
// transport asks it to resolve a repo ID at request time and to list
// available repos for the GET /repos endpoint. The internal/server
// package implements this; the transport stays free of repo concerns.
//
// SetEnabled is a server-level operation (per-repo state but not
// per-repo dispatcher target) so it lives on the backend interface
// rather than in the per-repo RPC dispatcher. Clients call it via
// POST /repos/<id>/enable or DELETE /repos/<id>.
type RepoBackend interface {
	Resolve(id string) *RepoTarget
	List() []RepoSummary
	SetEnabled(id string, enabled bool) error
}

// Server is the BRUV HTTP transport. Embed it in the desktop binary
// (loopback, single-repo) or run it headless via bruv-server
// (multi-repo).
type Server struct {
	cfg     Config
	devices *DeviceStore

	// Single-repo mode (Config.Repos == nil): one dispatcher + bus.
	dispatcher *Dispatcher
	bus        *events.MemBus

	// Multi-repo mode (Config.Repos != nil): per-repo dispatchers,
	// built lazily on first request and cached. The bus and
	// attachment resolver come from the resolved RepoTarget at
	// request time.
	dispatchers map[string]*Dispatcher

	httpServer *nethttp.Server
	listener   net.Listener
}

// New builds a single-repo server (desktop loopback). For multi-repo
// use NewMulti — same struct, different wiring.
func New(cfg Config, target any, bus *events.MemBus) (*Server, error) {
	devices, err := NewDeviceStore(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("device store: %w", err)
	}
	disp := NewDispatcher(target, DefaultDeniedMethods())
	return &Server{
		cfg:        cfg,
		dispatcher: disp,
		bus:        bus,
		devices:    devices,
	}, nil
}

// NewMulti builds a multi-repo server. Repos is required; per-repo
// dispatchers are built lazily on first lookup and cached.
func NewMulti(cfg Config) (*Server, error) {
	if cfg.Repos == nil {
		return nil, fmt.Errorf("transport.NewMulti: Config.Repos is required")
	}
	devices, err := NewDeviceStore(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("device store: %w", err)
	}
	return &Server{
		cfg:         cfg,
		devices:     devices,
		dispatchers: make(map[string]*Dispatcher),
	}, nil
}

// dispatcherFor returns the per-repo dispatcher in multi-repo mode,
// building + caching it on first request. Returns nil when the repo
// ID isn't known to the backend.
func (s *Server) dispatcherFor(repoID string) (*Dispatcher, *RepoTarget) {
	if s.cfg.Repos == nil {
		return s.dispatcher, nil // single-repo fallback (used by desktop)
	}
	target := s.cfg.Repos.Resolve(repoID)
	if target == nil {
		return nil, nil
	}
	if d, ok := s.dispatchers[repoID]; ok {
		return d, target
	}
	d := NewDispatcher(target.Target, DefaultDeniedMethods())
	s.dispatchers[repoID] = d
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

	if s.cfg.Repos != nil {
		// Multi-repo mode (bruv-server). Routes:
		//   GET /repos              → list of {id, name}
		//   /repos/<id>/rpc         → per-repo dispatcher
		//   /repos/<id>/events      → per-repo SSE
		//   /repos/<id>/attachments → per-repo signed-URL handler
		mux.Handle("/repos", requireAuth(s.devices, listReposHandler(s.cfg.Repos)))
		mux.Handle("/repos/", requireAuth(s.devices, s.repoRouter()))
	} else {
		// Single-repo mode (desktop loopback). Flat routes — the desktop
		// only ever has one repo open at a time and the UI doesn't
		// bother with the /repos/<id>/ prefix.
		mux.Handle("/rpc", requireAuth(s.devices, s.dispatcher.Handler()))
		mux.Handle("/events", requireAuth(s.devices, sseHandler(s.bus)))
		if s.cfg.Attachments != nil {
			mux.Handle("/attachments/", attachmentHandler(s.cfg.Attachments))
		}
		// Single-repo backends still expose GET /repos so the
		// connection-tree picker on the frontend renders Local
		// symmetrically with Remote. Prefer the registry-backed
		// path when the host has wired up LocalRegistry (the
		// desktop App does — it owns repos.json the same way
		// the server does). Falls back to the legacy reflection
		// shim that emits "the currently open repo" when no
		// registry is wired (kept so transport tests + early
		// boot before the App finishes wiring still answer).
		if s.cfg.LocalRegistry != nil {
			mux.Handle("/repos", requireAuth(s.devices, localRegistryListHandler(s.cfg.LocalRegistry)))
			mux.Handle("/repos/", requireAuth(s.devices, localRegistryEnableRouter(s.cfg.LocalRegistry)))
		} else {
			mux.Handle("/repos", requireAuth(s.devices, singleRepoListHandler(s.dispatcher)))
		}
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

	return mux
}
