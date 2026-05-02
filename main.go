package main

import (
	"bruv/frontend"
	"bruv/internal/config"
	"bruv/internal/repocli"
	"bruv/internal/server"
	"bruv/mobile"
	"embed"
	"flag"
	"log/slog"
	"os"

	"github.com/kardianos/service"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// --- Service-mode dispatch ---
	//
	// One bruv.exe behaves several ways. Order matters here: SCM
	// invocation must take precedence so the service host doesn't
	// accidentally fall through to the desktop UI on a session that
	// has no display.
	//
	//   1. SCM-spawned service — kardianos detects we're not running
	//      interactively and routes through runServiceMode().
	//   2. `bruv.exe service install/uninstall/start/stop/...` —
	//      explicit subcommand for managing the registered service.
	//   3. `bruv.exe repo <subcmd>` — registry management against
	//      repos.json on disk (list, add, enable, disable, remove,
	//      rename). No HTTP / no device token; file-system access is
	//      the credential.
	//   4. `bruv.exe --server` — interactive headless backend.
	//   5. (default) — Wails desktop UI.
	if !service.Interactive() {
		runServiceMode()
		return
	}
	if isServiceCommand() {
		runServiceCommand()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "repo" {
		os.Exit(repocli.Run(os.Args[2:], os.Stdout, os.Stderr))
	}
	if isServerMode() {
		runServerMode()
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Load saved window bounds (if any)
	width, height := 1280, 800
	startHidden := false
	if wb := config.LoadWindowBounds(); wb != nil {
		app.savedBounds = wb
		width = wb.Width
		height = wb.Height
		startHidden = true // we'll show after positioning in domReady
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:       "BRUV",
		Width:       width,
		Height:      height,
		MinWidth:    800,
		MinHeight:   600,
		StartHidden: startHidden,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		// Only the shell-bridge surface is exposed to the frontend via
		// Wails IPC; the full domain API (~130 methods) is reached over
		// HTTP+SSE through core/services + transport/http. See shell_bridge.go.
		Bind: []interface{}{
			newShellAPI(app),
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// isServerMode returns true when this process should run as the
// headless backend instead of the desktop UI. Triggered by either
// the --server CLI flag or the BRUV_MODE=server env var (the env
// var is for the Windows Service install path, where the SCM
// invokes the binary with environment but the args are also handy
// for `bruv.exe --server` from a terminal).
func isServerMode() bool {
	if os.Getenv("BRUV_MODE") == "server" {
		return true
	}
	for _, a := range os.Args[1:] {
		if a == "--server" || a == "-server" {
			return true
		}
	}
	return false
}

// runServerMode parses server-specific flags and forwards to
// internal/server.Run. Exits with non-zero on failure so the SCM /
// shell sees the failed-start signal.
func runServerMode() {
	// Strip --server from the arg list before flag.Parse so it
	// doesn't trip an "undefined flag" error.
	args := make([]string, 0, len(os.Args)-1)
	for _, a := range os.Args[1:] {
		if a == "--server" || a == "-server" {
			continue
		}
		args = append(args, a)
	}
	fs := flag.NewFlagSet("bruv --server", flag.ExitOnError)
	repoPath := fs.String("repo", "", "legacy: append this path to repos.json on startup (optional; the registry is now the source of truth)")
	addr := fs.String("addr", "0.0.0.0:9870", "HTTP listen address")
	configDir := fs.String("config", "", "config directory (default: <user-config>/bruv/)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	// --repo is no longer required: the service runs against the
	// multi-repo registry at <configDir>/repos.json. The flag stays
	// for the install-time bootstrap path (legacy single-repo
	// installs that never wrote to the registry) and for ad-hoc
	// "add a repo and start the server" one-liners. server.Run
	// errors clearly when neither --repo nor a populated registry
	// is available, so the prior frontline guard here is redundant.
	if err := server.Run(server.Options{
		RepoPath:     *repoPath,
		Addr:         *addr,
		ConfigDir:    *configDir,
		Version:      AppVersion,
		BuildDate:    BuildDate,
		Assets:       frontend.Assets(),
		MobileAssets: mobile.Assets(),
	}); err != nil {
		slog.Error("bruv --server failed", "err", err)
		os.Exit(1)
	}
}
