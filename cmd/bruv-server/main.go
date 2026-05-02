// Command bruv-server is a thin wrapper around internal/server.Run
// kept for `go run ./cmd/bruv-server` ergonomics during development.
//
// Production deployment uses the unified bruv.exe with a `--server`
// flag (which forwards to the same internal/server.Run). Building
// this command separately is no longer necessary; the shared code
// lives in internal/server and the desktop binary embeds it too.
//
// Flags mirror the desktop --server forwarder:
//
//	--repo      Path to the BRUV repo to open (required).
//	--addr      HTTP listen address. Default: 127.0.0.1:9870.
//	--config    Config directory. Default: <user-config>/bruv/.
//
// Signals: SIGINT / SIGTERM trigger graceful shutdown.
package main

import (
	"flag"
	"log/slog"
	"os"

	"bruv/frontend"
	"bruv/internal/repocli"
	"bruv/internal/server"
	"bruv/mobile"
)

// Version + BuildDate are stamped at build time via -ldflags.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	// `repo` subcommand: registry management against repos.json on disk
	// (no HTTP, no device token). Dispatched before flag.Parse so we
	// don't choke on positional args. See internal/repocli for the
	// available subcommands.
	if len(os.Args) > 1 && os.Args[1] == "repo" {
		os.Exit(repocli.Run(os.Args[2:], os.Stdout, os.Stderr))
	}

	// --repo is optional now: when set, it gets appended to the
	// repo registry on startup (idempotent). The normal steady-state
	// is no --repo + the registry already populated by a previous
	// service install.
	repoPath := flag.String("repo", "", "path to BRUV repo (appended to registry; optional once registry has entries)")
	addr := flag.String("addr", "127.0.0.1:9870", "HTTP listen address")
	configDir := flag.String("config", "", "config directory (default: <user-config>/bruv/)")
	flag.Parse()

	if err := server.Run(server.Options{
		RepoPath:     *repoPath,
		Addr:         *addr,
		ConfigDir:    *configDir,
		Version:      Version,
		BuildDate:    BuildDate,
		Assets:       frontend.Assets(),
		MobileAssets: mobile.Assets(),
	}); err != nil {
		slog.Error("bruv-server failed", "err", err)
		os.Exit(1)
	}
}
