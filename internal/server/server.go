// Package server is the headless BRUV backend — same domain code the
// desktop App wires up, but without Wails. Used by:
//
//   - The unified bruv.exe binary's `--server` mode (entry point in
//     main.go forwards here when the flag is set).
//   - The thin cmd/bruv-server wrapper, kept for `go run` ergonomics
//     during development.
//
// Single-binary deployment: the same .exe runs as the desktop client
// (no flag) or as a headless server (--server). The logic below
// builds the supervisor, wires the HTTP transport, and blocks on
// signals until shutdown. The per-repo Runtime + multi-repo Supervisor
// types live in core/supervisor — shared with the desktop App so
// Local and Remote use the same code path.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/push"
	"bruv/internal/logging"
	transporthttp "bruv/transport/http"
)

// Options captures everything the entry points need to configure
// before starting the server. Defaults are filled in by Run.
type Options struct {
	// RepoPath, when set, is appended to the registry on startup
	// (idempotent — same path won't be added twice). Optional: pass
	// it from the legacy single-repo --repo CLI flag, or omit when
	// the registry already has entries (the normal case once the
	// service has been installed once).
	RepoPath  string
	Addr      string // default 127.0.0.1:9870
	ConfigDir string // default <user-config>/bruv/
	Version   string // build-stamped, defaults to "dev"
	BuildDate string // build-stamped, defaults to "unknown"
	// Assets is the embedded Svelte bundle to serve at /app/*.
	// Pass frontend.Assets() at the call site so this package
	// stays free of the frontend embed (which would otherwise
	// import-cycle through anything that depends on it).
	Assets fs.FS
	// MobileAssets is the embedded mobile PWA bundle to serve at /m/*.
	// Pass mobile.Assets() at the call site, mirroring Assets above.
	// Optional — leave nil if the build doesn't ship the mobile UI.
	MobileAssets fs.FS
}

// Run starts the multi-repo headless server, blocks until
// SIGINT/SIGTERM, then shuts down cleanly. Reads the repo registry
// from <configDir>/repos.json and stands up one Runtime per entry,
// routed by the HTTP transport's /repos/<id>/... paths.
func Run(opts Options) error {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:9870"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.BuildDate == "" {
		opts.BuildDate = "unknown"
	}
	if opts.ConfigDir == "" {
		dir, err := config.ConfigDir()
		if err != nil {
			return fmt.Errorf("resolve config dir: %w", err)
		}
		opts.ConfigDir = dir
	}
	// Critical: route every internal/config helper at this directory
	// too. Without it, LoadRepos / AppendRepo / etc. fall back to
	// os.UserConfigDir() which on a service running as LocalSystem is
	// C:\Windows\System32\config\systemprofile\AppData\Roaming — NOT
	// the %PROGRAMDATA%\BRUV path the installer wrote repos.json to.
	// Pre-fix the service would error "no repos configured" on every
	// boot and never reach the bootstrap-token-generating code path.
	config.SetConfigDir(opts.ConfigDir)

	// File logging is critical here: the service runs as LocalSystem,
	// stderr disappears into the void, and SCM only surfaces "service
	// failed to start" without context. Writing to <configDir>/logs/
	// gives the operator (and us debugging issues) something to read
	// when boot goes sideways. Failure is non-fatal — the slog default
	// (stderr) keeps working, just invisible to anyone who isn't
	// running the binary from a terminal.
	if _, err := logging.Init(opts.ConfigDir); err != nil {
		slog.Warn("logging init failed", "err", err)
	}
	logging.InitCrashReporting(opts.ConfigDir, opts.Version, opts.BuildDate)

	slog.Info("bruv-server starting", "version", opts.Version, "build_date", opts.BuildDate, "config_dir", opts.ConfigDir)

	// Legacy single-repo mode: when --repo was passed on the CLI,
	// append it to the registry so a fresh install bootstraps with
	// one entry. Idempotent — re-running doesn't duplicate.
	if opts.RepoPath != "" {
		if _, err := config.AppendRepo(opts.RepoPath, ""); err != nil {
			slog.Warn("append --repo to registry failed", "err", err)
		}
	}

	store, err := config.LoadRepos()
	if err != nil {
		return fmt.Errorf("load repos.json: %w", err)
	}
	if len(store.Repos) == 0 {
		return fmt.Errorf("no repos configured — run `bruv.exe service install --repo <path>` first")
	}

	sup, err := supervisor.New(store.Repos, opts.ConfigDir)
	if err != nil {
		return fmt.Errorf("supervisor build: %w", err)
	}
	// Server eagerly loads every non-disabled runtime so requests
	// against any repo dispatch without a cold-start hiccup.
	sup.LoadAll()
	defer sup.Close()

	// Push subsystem: VAPID keypair (generated on first boot, persisted
	// alongside bootstrap-token.txt) and a per-device subscription
	// registry. Failures to load are non-fatal — the server keeps
	// serving non-push traffic, push RPCs return errPushNotConfigured.
	var pushVAPID *push.VAPID
	var pushRegistry *push.Registry
	if v, err := push.LoadOrCreate(opts.ConfigDir); err == nil {
		pushVAPID = v
	} else {
		slog.Warn("push: VAPID keypair unavailable, push notifications disabled", "err", err)
	}
	if r, err := push.LoadRegistry(opts.ConfigDir); err == nil {
		pushRegistry = r
	} else {
		slog.Warn("push: subscription registry unavailable, push notifications disabled", "err", err)
	}

	machineSvc := supervisor.NewMachineService()
	if pushVAPID != nil && pushRegistry != nil {
		machineSvc = machineSvc.WithPush(pushVAPID, pushRegistry)
	}

	srv, err := transporthttp.NewMulti(transporthttp.Config{
		Addr:          opts.Addr,
		ConfigDir:     opts.ConfigDir,
		Version:       opts.Version,
		BuildDate:     opts.BuildDate,
		StaticAssets:  opts.Assets,
		MobileAssets:  opts.MobileAssets,
		Repos:         supervisor.NewHTTPAdapter(sup),
		MachineTarget: machineSvc,
	})
	if err != nil {
		return fmt.Errorf("http transport construct: %w", err)
	}
	if err := srv.Start(); err != nil {
		return fmt.Errorf("http transport start: %w", err)
	}
	slog.Info("bruv-server listening",
		"addr", srv.Addr(),
		"repos", len(store.Repos),
		"bootstrap_token", filepath.Join(opts.ConfigDir, "bootstrap-token.txt"))

	// Print a ready-to-click pairing URL for the operator. Two variants:
	//
	//  - Local (loopback): works from a browser on THIS machine. Use
	//    this when you're at the home server's keyboard. Pass
	//    &host=<your-tailscale-hostname>&scheme=https so the QR
	//    encodes a phone-reachable URL rather than 127.0.0.1.
	//  - Tailscale (placeholder): for remote access from another
	//    tailnet device. Substitute your *.ts.net hostname.
	//
	// Hitting the tailscale URL FROM the same machine that runs
	// tailscale serve is a hairpin and won't work — that's why the
	// local form is here too.
	if bsBytes, err := os.ReadFile(filepath.Join(opts.ConfigDir, "bootstrap-token.txt")); err == nil {
		bs := strings.TrimSpace(string(bsBytes))
		if bs != "" {
			fmt.Fprintf(os.Stderr, "\n  Pair a phone:\n")
			fmt.Fprintf(os.Stderr, "    Local:     http://%s/pair?token=%s&host=<your-machine>.ts.net&scheme=https\n", srv.Addr(), bs)
			fmt.Fprintf(os.Stderr, "    Tailscale: https://<your-machine>.ts.net/pair?token=%s\n\n", bs)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("signal received, shutting down", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Stop()
	_ = ctx
	return nil
}

