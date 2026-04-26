package main

// Windows Service install/uninstall/control + SCM-driven runtime.
//
// One bruv.exe behaves three ways:
//
//  1. Interactive desktop (no flags) — the Wails UI.
//  2. Interactive headless (--server) — the foreground backend.
//  3. SCM-invoked service — the same backend, but the Windows
//     Service Control Manager is the parent. kardianos/service
//     hides the platform plumbing (Windows SCM, macOS launchd,
//     Linux systemd) so the same code installs everywhere.
//
// The install path bakes --server + --repo + --addr into the
// service's command line, so the SCM invocation is identical to
// what you'd type in a terminal. That keeps the runtime path
// identical between manual and managed runs.

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"bruv/frontend"
	"bruv/internal/config"
	"bruv/internal/repo"
	"bruv/internal/server"

	"github.com/kardianos/service"
)

const serviceName = "BRUV-Server"
const serviceDisplay = "BRUV Server"
const serviceDesc = "Self-hosted BRUV backend (HTTP + SSE for desktop / browser clients)."

// isServiceCommand returns true when invoked as `bruv.exe service ...`.
// Detected up front in main.go so the dispatch happens before any
// Wails / desktop-mode work.
func isServiceCommand() bool {
	return len(os.Args) >= 2 && os.Args[1] == "service"
}

// runServiceCommand handles install / uninstall / start / stop /
// status / run subcommands. Exits the process; never returns.
func runServiceCommand() {
	if len(os.Args) < 3 {
		printServiceUsage()
		os.Exit(2)
	}
	sub := os.Args[2]

	// install accepts the same --repo / --addr / --config flags as
	// --server itself; they're baked into the service's launch
	// arguments so the SCM invocation matches the manual one.
	if sub == "install" {
		runServiceInstall()
		return
	}
	if sub == "uninstall" || sub == "start" || sub == "stop" || sub == "restart" || sub == "status" {
		runServiceControl(sub)
		return
	}
	printServiceUsage()
	os.Exit(2)
}

func printServiceUsage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  bruv.exe service install --repo <path> [--addr <host:port>]")
	fmt.Fprintln(os.Stderr, "  bruv.exe service uninstall")
	fmt.Fprintln(os.Stderr, "  bruv.exe service start | stop | restart | status")
}

func runServiceInstall() {
	fs := flag.NewFlagSet("bruv service install", flag.ExitOnError)
	repoPath := fs.String("repo", "", "path to BRUV repo to open (required)")
	addr := fs.String("addr", "0.0.0.0:9870", "HTTP listen address")
	configDir := fs.String("config", "", "config directory (default: <user-config>/bruv/)")
	if err := fs.Parse(os.Args[3:]); err != nil {
		os.Exit(2)
	}
	if *repoPath == "" {
		fmt.Fprintln(os.Stderr, "error: --repo is required")
		fs.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(*repoPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: resolve --repo:", err)
		os.Exit(1)
	}

	// Default config dir for service runs lives under ProgramData
	// rather than the invoking user's AppData. Reason: the service
	// runs as LocalSystem (kardianos default), so a per-user path
	// would resolve to C:\Windows\System32\config\systemprofile\
	// AppData\Roaming\bruv — invisible to the actual user. ProgramData
	// is writable by LocalSystem, readable by all local users, and
	// is the canonical "machine-wide app data" location on Windows.
	// The user-supplied --config (if given) takes precedence.
	if *configDir == "" {
		if pd := os.Getenv("PROGRAMDATA"); pd != "" {
			*configDir = filepath.Join(pd, "BRUV")
		}
	}

	// Idempotent: if no repo exists at this path yet, create one.
	// Lets the NSIS installer call `service install --repo <path>`
	// once and have everything end-to-end ready (including a brand
	// new empty repo) without a separate init step.
	manifestPath := filepath.Join(abs, "manifest.json")
	if _, statErr := os.Stat(manifestPath); statErr != nil && os.IsNotExist(statErr) {
		if _, initErr := repo.InitAt(abs, filepath.Base(abs)); initErr != nil {
			fmt.Fprintln(os.Stderr, "error: init repo at", abs, ":", initErr)
			os.Exit(1)
		}
		fmt.Println("Created BRUV repository at", abs)
	}

	// Append (or upsert) the repo into the server's registry. The
	// service runs against repos.json — calling install twice with
	// different --repo paths grows the registry rather than replacing
	// it, which is exactly the workflow for "I want my server to
	// host a second repo".
	//
	// Note: AppendRepo writes to <configDir>/repos.json, which uses
	// the user-config dir resolved by os.UserConfigDir(). When the
	// installer runs elevated, that's the elevating account's
	// AppData. We pass --config explicitly to the service args
	// (defaulted to %PROGRAMDATA%/BRUV above) so the SERVICE itself
	// reads from the predictable system-wide location. To get
	// AppendRepo to write to that same location, we temporarily
	// point os.UserConfigDir at the install --config dir via the
	// XDG_CONFIG_HOME / APPDATA env vars before the call.
	if *configDir != "" {
		// On Windows, configDir() reads APPDATA. Override so AppendRepo
		// writes to <configDir>/repos.json rather than the elevating
		// user's AppData/bruv/repos.json.
		_ = os.Setenv("APPDATA", *configDir)
		// Strip the trailing "bruv" so configDir() can re-add it.
		// configDir() does APPDATA + "/bruv". Our --config is already
		// the bruv root (e.g. %PROGRAMDATA%/BRUV), so APPDATA must be
		// its parent.
		_ = os.Setenv("APPDATA", filepath.Dir(*configDir))
	}
	if _, regErr := config.AppendRepo(abs, ""); regErr != nil {
		fmt.Fprintln(os.Stderr, "warning: append repo to registry:", regErr)
		// Non-fatal: the service can still run with the legacy
		// single-repo --repo arg below as a fallback.
	}

	args := []string{"--server", "--addr", *addr}
	if *configDir != "" {
		args = append(args, "--config", *configDir)
	}

	svc, err := newService(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: build service:", err)
		os.Exit(1)
	}
	if err := svc.Install(); err != nil {
		fmt.Fprintln(os.Stderr, "error: install service:", err)
		os.Exit(1)
	}
	if err := svc.Start(); err != nil {
		// Install succeeded; start failure is non-fatal — surface it
		// but tell the user the service is registered.
		fmt.Fprintln(os.Stderr, "warning: service installed but failed to start:", err)
		fmt.Fprintln(os.Stderr, "you can start it manually with: bruv.exe service start")
		return
	}
	fmt.Println("BRUV Server installed and running.")
	fmt.Printf("  Repo:    %s\n", abs)
	fmt.Printf("  Address: %s\n", *addr)
}

func runServiceControl(action string) {
	svc, err := newService(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: build service:", err)
		os.Exit(1)
	}
	switch action {
	case "uninstall":
		_ = svc.Stop() // best-effort; OK if already stopped
		if err := svc.Uninstall(); err != nil {
			fmt.Fprintln(os.Stderr, "error: uninstall service:", err)
			os.Exit(1)
		}
		fmt.Println("BRUV Server uninstalled.")
	case "start":
		if err := svc.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "error: start service:", err)
			os.Exit(1)
		}
		fmt.Println("BRUV Server started.")
	case "stop":
		if err := svc.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "error: stop service:", err)
			os.Exit(1)
		}
		fmt.Println("BRUV Server stopped.")
	case "restart":
		if err := svc.Restart(); err != nil {
			fmt.Fprintln(os.Stderr, "error: restart service:", err)
			os.Exit(1)
		}
		fmt.Println("BRUV Server restarted.")
	case "status":
		st, err := svc.Status()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: query service status:", err)
			os.Exit(1)
		}
		fmt.Println(formatServiceStatus(st))
	}
}

// runServiceMode is invoked when the SCM has spawned us as a service
// (service.Interactive() == false). The kardianos library calls our
// Program.Start in a goroutine, then blocks on the SCM control loop
// until shutdown is requested. We call svc.Run() to enter that loop.
func runServiceMode() {
	svc, err := newService(nil)
	if err != nil {
		// Can't slog yet — slog setup is in startup which is
		// service-mode-only too. Print to stderr; the SCM captures
		// stdout/stderr in the service event log.
		fmt.Fprintln(os.Stderr, "error: build service:", err)
		os.Exit(1)
	}
	if err := svc.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error: service run:", err)
		os.Exit(1)
	}
}

func newService(installArgs []string) (service.Service, error) {
	prog := &serverProgram{}
	cfg := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplay,
		Description: serviceDesc,
		Arguments:   installArgs, // ignored by Run(); used by Install()
	}
	return service.New(prog, cfg)
}

// serverProgram implements kardianos/service.Interface. Start kicks
// off the headless backend in a goroutine and returns immediately so
// the SCM control thread isn't blocked. Stop sends an interrupt that
// internal/server.Run already listens for.
type serverProgram struct {
	stopOnce bool
}

func (p *serverProgram) Start(s service.Service) error {
	go func() {
		// Re-parse the args the SCM gave us. They're the same
		// --server --repo X --addr Y the install step baked in.
		args := stripServerFlag(os.Args[1:])
		fs := flag.NewFlagSet("bruv service-run", flag.ContinueOnError)
		repoPath := fs.String("repo", "", "")
		addr := fs.String("addr", "0.0.0.0:9870", "")
		configDir := fs.String("config", "", "")
		_ = fs.Parse(args)

		if *repoPath == "" {
			slog.Error("service: --repo argument missing from service config")
			return
		}
		err := server.Run(server.Options{
			RepoPath:  *repoPath,
			Addr:      *addr,
			ConfigDir: *configDir,
			Version:   AppVersion,
			BuildDate: BuildDate,
			Assets:    frontend.Assets(),
		})
		if err != nil {
			slog.Error("service: backend exited with error", "err", err)
		}
	}()
	return nil
}

// Stop signals shutdown. internal/server.Run is currently signal-driven
// (SIGINT/SIGTERM); the kardianos library on Windows translates the
// SCM stop command into our process being told to exit, which already
// triggers our signal handlers. Nothing extra needed here.
func (p *serverProgram) Stop(s service.Service) error {
	if p.stopOnce {
		return nil
	}
	p.stopOnce = true
	return nil
}

// stripServerFlag removes the --server / -server tokens (which may or
// may not appear in the SCM-passed args, depending on installation
// path) before handing the remainder to flag.Parse.
func stripServerFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--server" || a == "-server" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func formatServiceStatus(s service.Status) string {
	switch s {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}
