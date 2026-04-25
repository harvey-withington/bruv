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
	// Attachments wires the signed-URL download handler at
	// /attachments/{cardID}/{id}. Leave nil to disable attachment
	// serving (the JSON-RPC AddCardAttachment path still works for
	// upload + storage; this is only the download surface).
	Attachments *AttachmentConfig
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

// Server is the BRUV HTTP transport. Embed it in the desktop binary
// (loopback) or run it headless (future cmd/bruv-server).
type Server struct {
	cfg        Config
	dispatcher *Dispatcher
	bus        *events.MemBus
	devices    *DeviceStore

	httpServer *nethttp.Server
	listener   net.Listener
}

// New builds a server. Call Start to actually listen + serve.
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

	mux.Handle("/rpc", requireAuth(s.devices, s.dispatcher.Handler()))
	mux.Handle("/events", requireAuth(s.devices, sseHandler(s.bus)))
	mux.Handle("/auth/enrol", requireBootstrap(s.devices, enrolHandler(s.devices)))

	// Signed-URL attachment downloads. Auth is the HMAC sig+exp on
	// the URL itself rather than a bearer token, so the URL works
	// in <img src> / <a href> without leaking the device token.
	if s.cfg.Attachments != nil {
		mux.Handle("/attachments/", attachmentHandler(s.cfg.Attachments))
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
