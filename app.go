package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/logging"
	"bruv/internal/update"
	"bruv/mobile"
	transporthttp "bruv/transport/http"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppVersion is the user-facing release version. Overridable at build
// time via -ldflags "-X main.AppVersion=v1.0a" so releases don't require
// a source edit.
var AppVersion = "v1.0a-dev"

// BuildDate is stamped at build time via -ldflags "-X main.BuildDate=...".
// Defaults to "development" for local dev builds.
var BuildDate = "development"

// BugReportURL is the GitHub issues page used by the "Report a bug" action.
const BugReportURL = "https://github.com/harvey-withington/bruv/issues/new"

// BuildInfo is returned to the frontend for the About dialog.
type BuildInfo struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

// App is the Wails desktop shell. After the 2026-04-26 local-as-remote
// pivot, App owns ONLY desktop-specific concerns: the loopback HTTP
// server, window/tray/bounds, native dialogs, and the per-machine
// connection list (which has to stay reachable when the active backend
// is unreachable, so it lives on the Wails Shell binding rather than
// the cloud adapter).
//
// All per-repo + per-machine RPCs travel over HTTP+SSE through the
// transport — Local routes through /repos/<local-id>/rpc just like
// any Remote does, dispatching against a Runtime resolved by the
// supervisor. App is no longer a dispatcher target.
type App struct {
	// Supervisor — owns the per-repo runtimes for this device.
	sup *supervisor.Supervisor

	// Desktop-only lifecycle state.
	ctx             context.Context
	savedBounds     *config.WindowBounds
	boundsRestored  bool
	lastSavedBounds *config.WindowBounds
	forceQuit       bool
	trayPauseItem   interface{ Check(); Uncheck(); Checked() bool }
	traySetUp       bool // guard so reload-driven domReady doesn't re-run systray.Run
	boundsPolling   bool // same idempotency story for the bounds-saver goroutine

	// busBridgeUnsub cancels the supervisor.Bus() subscription used
	// for tray-tooltip refresh on notification:new events. Stable
	// across runtime swaps (the supervisor aggregates).
	busBridgeUnsub func()
	busBridgeOnce  sync.Once

	httpServer *transporthttp.Server
}

// NewApp constructs the desktop App. The supervisor is built from
// whatever's already in repos.json (post legacy-recents migration);
// no Runtime is loaded — the multi-repo HTTP transport lazy-loads on
// first request via the HTTPAdapter.
func NewApp() *App {
	a := &App{}

	// Migrations run BEFORE constructing the supervisor so freshly-
	// imported entries land in the supervisor's registry view + the
	// Local connection entry exists for GetHTTPTransportInfo to find.
	config.MigrateRecentsToRegistry()
	config.MigrateRepoIDsToManifest()
	config.MigrateLocalConnection()

	cfgDir, err := config.ConfigDir()
	if err != nil {
		slog.Warn("NewApp: resolve config dir failed", "err", err)
		cfgDir = "."
	}
	store, err := config.LoadRepos()
	if err != nil {
		slog.Warn("NewApp: load repos.json failed", "err", err)
		store = config.ReposStore{}
	}
	sup, err := supervisor.New(store.Repos, cfgDir)
	if err != nil {
		slog.Warn("NewApp: supervisor construct failed", "err", err)
		sup, _ = supervisor.New(nil, cfgDir)
	}
	a.sup = sup
	return a
}

// startup runs once when Wails boots the shell. Order matters: HTTP
// transport must be up before the device-token enrolment hits its
// loopback, and both must be done before the Local connection entry
// gets its URL+token re-stamped.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.startHTTPTransport()
	a.ensureDesktopDeviceToken()
	a.refreshLocalConnection()
	a.startBusBridge()

	// File logging to <configDir>/logs/. Failure is non-fatal; the
	// stderr default keeps working but is invisible in a GUI build.
	if cfgDir, err := config.ConfigDir(); err == nil {
		if _, err := logging.Init(cfgDir); err != nil {
			slog.Warn("logging init failed", "err", err)
		}
		logging.InitCrashReporting(cfgDir, AppVersion, BuildDate)
	} else {
		slog.Warn("resolve config dir for logging failed", "err", err)
	}

	if err := config.MigrateLegacyLLMConfig(); err != nil {
		slog.Warn("legacy LLM config migration failed", "err", err)
	}
	if err := config.MigrateLLMKeysToKeychain(); err != nil {
		slog.Warn("LLM keychain migration failed", "err", err)
	}
}

// domReady runs after the frontend DOM is ready. Idempotent — Wails
// fires it on every browser reload, so the tray + bounds-poller
// helpers each carry their own one-shot guard.
func (a *App) domReady(ctx context.Context) {
	if a.savedBounds != nil && !a.boundsRestored {
		wailsRuntime.WindowSetSize(ctx, a.savedBounds.Width, a.savedBounds.Height)
		wailsRuntime.WindowSetPosition(ctx, a.savedBounds.X, a.savedBounds.Y)
		if a.savedBounds.Maximised {
			wailsRuntime.WindowMaximise(ctx)
		}
		a.boundsRestored = true
	}
	wailsRuntime.WindowShow(ctx)
	a.startBoundsPoller()
	a.setupTray()
}

// startBoundsPoller runs a background goroutine that saves window
// bounds every 3 seconds when they change — preserves position even
// on hard kill. Idempotent: domReady fires per reload.
func (a *App) startBoundsPoller() {
	if a.boundsPolling {
		return
	}
	a.boundsPolling = true
	go func() {
		defer logging.Recover("bounds-poller")
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.saveCurrentBounds()
			}
		}
	}()
}

// saveCurrentBounds captures + persists the current window position
// and size, skipping the write when nothing has changed.
func (a *App) saveCurrentBounds() {
	maximised := wailsRuntime.WindowIsMaximised(a.ctx)
	x, y := wailsRuntime.WindowGetPosition(a.ctx)
	w, h := wailsRuntime.WindowGetSize(a.ctx)
	wb := &config.WindowBounds{
		X: x, Y: y, Width: w, Height: h, Maximised: maximised,
	}
	if a.lastSavedBounds != nil &&
		a.lastSavedBounds.X == wb.X &&
		a.lastSavedBounds.Y == wb.Y &&
		a.lastSavedBounds.Width == wb.Width &&
		a.lastSavedBounds.Height == wb.Height &&
		a.lastSavedBounds.Maximised == wb.Maximised {
		return
	}
	if err := config.SaveWindowBounds(wb); err == nil {
		a.lastSavedBounds = wb
	}
}

// beforeClose is the Wails hook for window-close. Hides to tray when
// agents may still be running; otherwise tears down + allows quit.
func (a *App) beforeClose(ctx context.Context) bool {
	a.saveCurrentBounds()
	if a.forceQuit {
		a.sup.Close()
		a.stopHTTPTransport()
		a.stopBusBridge()
		logging.Close()
		return false
	}
	wailsRuntime.WindowHide(ctx)
	return true
}

// startBusBridge subscribes to the supervisor's aggregated event bus
// for the tray-tooltip refresh side effect. Every loaded runtime's
// notification:new flows in here regardless of which repo fired —
// tray badge stays accurate cross-repo. Idempotent.
func (a *App) startBusBridge() {
	if a.busBridgeUnsub != nil {
		return
	}
	ch, unsub := a.sup.Bus().Subscribe()
	a.busBridgeUnsub = unsub
	a.busBridgeOnce = sync.Once{}
	go func() {
		defer logging.Recover("busBridge")
		for ev := range ch {
			if ev.Topic == "notification:new" {
				go a.refreshTrayTooltip()
			}
		}
	}()
}

// stopBusBridge cancels the bus subscription. Safe to call multiple times.
func (a *App) stopBusBridge() {
	a.busBridgeOnce.Do(func() {
		if a.busBridgeUnsub != nil {
			a.busBridgeUnsub()
			a.busBridgeUnsub = nil
		}
	})
}

// startHTTPTransport binds the multi-repo HTTP server to a random
// loopback port. Failures are logged but non-fatal — the desktop UI
// surfaces a "can't reach Local" error screen if the transport is
// unavailable, so the user can at least switch to a working Remote.
func (a *App) startHTTPTransport() {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		slog.Warn("http transport: resolve config dir failed", "err", err)
		return
	}
	srv, err := transporthttp.NewMulti(transporthttp.Config{
		Addr:          "127.0.0.1:0",
		ConfigDir:     cfgDir,
		Version:       AppVersion,
		BuildDate:     BuildDate,
		StaticAssets:  assets,
		MobileAssets:  mobile.Assets(),
		Repos:         supervisor.NewHTTPAdapter(a.sup),
		MachineTarget: supervisor.NewMachineService(),
	})
	if err != nil {
		slog.Warn("http transport: construct failed", "err", err)
		return
	}
	if err := srv.Start(); err != nil {
		slog.Warn("http transport: start failed", "err", err)
		return
	}
	a.httpServer = srv
}

// stopHTTPTransport shuts the HTTP server down. Safe to call when the
// server never started (no-op).
func (a *App) stopHTTPTransport() {
	if a.httpServer == nil {
		return
	}
	if err := a.httpServer.Stop(); err != nil {
		slog.Warn("http transport: shutdown failed", "err", err)
	}
	a.httpServer = nil
}

// refreshLocalConnection re-stamps the persisted Local connection
// entry's URL + DeviceToken to match this boot's loopback addr +
// resolved device token. The migration in NewApp creates the entry
// with empty fields; this fills them in once the HTTP server has
// bound a port and ensureDesktopDeviceToken has populated the token.
// Idempotent — same URL/token = no-op write.
func (a *App) refreshLocalConnection() {
	if a.httpServer == nil || desktopDeviceToken == "" {
		return
	}
	url := "http://" + a.httpServer.Addr()
	if _, err := config.UpdateConnection(config.LocalConnectionID, "", url, desktopDeviceToken); err != nil {
		slog.Warn("refresh local connection failed", "err", err)
	}
}

// GetHTTPTransportInfo returns the backend endpoint + bearer token
// the cloud adapter should use. Resolution order:
//
//  1. BRUV_REMOTE_URL + BRUV_REMOTE_TOKEN env vars — escape hatch for
//     dev/test; bypasses the connection store entirely.
//  2. The active connection from the persisted store. After the
//     local-as-remote migration this includes the Local entry with
//     ID "local" — same shape as any Remote — so there's no longer
//     a separate fallback for "Local-by-implicit-empty-active".
//
// Returns empty addr/token when neither is available; the cloud
// adapter surfaces that as a "transport unavailable" error screen.
func (a *App) GetHTTPTransportInfo() map[string]string {
	if remoteURL := os.Getenv("BRUV_REMOTE_URL"); remoteURL != "" {
		if token := os.Getenv("BRUV_REMOTE_TOKEN"); token != "" {
			addr := strings.TrimPrefix(strings.TrimPrefix(remoteURL, "https://"), "http://")
			return map[string]string{
				"addr":   addr,
				"token":  token,
				"scheme": schemeFromURL(remoteURL),
			}
		}
		slog.Warn("BRUV_REMOTE_URL set without BRUV_REMOTE_TOKEN — falling back to active connection")
	}
	active, _ := config.ActiveConnection()
	if active == nil {
		// Should never happen post-migration (Local is always present).
		// Return empties so the frontend sees "transport unavailable".
		return map[string]string{"addr": "", "token": ""}
	}
	addr := strings.TrimPrefix(strings.TrimPrefix(active.URL, "https://"), "http://")
	repoID := config.GetRecentRepoForConnection(active.ID)
	return map[string]string{
		"addr":   addr,
		"token":  active.DeviceToken,
		"scheme": schemeFromURL(active.URL),
		"repoID": repoID,
	}
}

// --- Connections (Wails-bound) ---
//
// Connection management stays on the Wails Shell binding because it's
// strictly per-device state and must keep working when the active
// connection's backend is unreachable (a misconfigured remote would
// otherwise lock the user out of the means to fix it).

// ListConnections returns the persisted connection store.
func (a *App) ListConnections() (config.ConnectionStore, error) {
	return config.LoadConnections()
}

// AddConnection persists a new remote connection. Caller is expected
// to have already exchanged the bootstrap token for a device token
// via /auth/enrol on the target server.
func (a *App) AddConnection(name, url, deviceToken string) (config.Connection, error) {
	return config.AddConnection(name, url, deviceToken)
}

// RemoveConnection drops a connection by ID. If it was active, Active
// resets to "" — the Local entry is the implicit fallback once the
// post-migration code is the only thing on disk; defensively, the
// caller should switch to "local" explicitly.
func (a *App) RemoveConnection(id string) error {
	return config.RemoveConnection(id)
}

// UpdateConnection edits a saved connection's name, URL, or device
// token. Empty fields are left unchanged.
func (a *App) UpdateConnection(id, name, url, deviceToken string) (config.Connection, error) {
	return config.UpdateConnection(id, name, url, deviceToken)
}

// SetActiveConnection switches the active pointer. The frontend
// reloads after this so the cloud adapter re-resolves transport.
func (a *App) SetActiveConnection(id string) error {
	return config.SetActiveConnection(id)
}

// SetActiveRepo persists the user's repo selection for the currently
// active connection. The frontend reloads after calling this so the
// cloud adapter picks up the new repo ID via GetHTTPTransportInfo.
// With Local being a real connection now, this is a uniform write —
// no special-case for "Local needs an actual OpenRepository call".
// The transport's lazy supervisor handles the rest at request time.
func (a *App) SetActiveRepo(repoID string) error {
	active, err := config.ActiveConnection()
	if err != nil {
		return err
	}
	if active == nil {
		return fmt.Errorf("no active connection")
	}
	return config.SetRecentRepoForConnection(active.ID, repoID)
}

// SetActiveRepoForConnection writes the per-connection "last selected
// repo" pointer for ANY connection (not just the active one). Pass
// repoID="" to clear (so reload lands on the picker). Used by the
// picker when crossing connections, and by the back chevron to clear
// the current connection's pointer.
func (a *App) SetActiveRepoForConnection(connectionID, repoID string) error {
	return config.SetRecentRepoForConnection(connectionID, repoID)
}

// schemeFromURL returns "https" if the URL is https://..., else "http".
func schemeFromURL(u string) string {
	if strings.HasPrefix(u, "https://") {
		return "https"
	}
	return "http"
}

// --- Build info / version (Wails-bound, used by About dialog) ---

func (a *App) Version() string { return AppVersion }

func (a *App) GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   AppVersion,
		BuildDate: BuildDate,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}
}

// CheckForUpdates queries GitHub Releases for the latest version.
// Manual trigger only — the app never phones home on its own.
func (a *App) CheckForUpdates() update.Result {
	return update.Check(AppVersion)
}

// --- Native dialogs (Wails-bound, must run in shell process) ---

func (a *App) PickFolder(title string) (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
}

func (a *App) PickFile(title, filterName, filterPattern string) (string, error) {
	opts := wailsRuntime.OpenDialogOptions{Title: title}
	if filterPattern != "" {
		opts.Filters = []wailsRuntime.FileFilter{{
			DisplayName: filterName,
			Pattern:     filterPattern,
		}}
	}
	return wailsRuntime.OpenFileDialog(a.ctx, opts)
}

func (a *App) PickSaveFile(title, defaultName, filterName, filterPattern string) (string, error) {
	opts := wailsRuntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
	}
	if filterPattern != "" {
		opts.Filters = []wailsRuntime.FileFilter{{
			DisplayName: filterName,
			Pattern:     filterPattern,
		}}
	}
	return wailsRuntime.SaveFileDialog(a.ctx, opts)
}

// --- Shell-open helpers (Wails-bound) ---

func (a *App) OpenConfigFolder() error {
	dir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	return openInFileManager(dir)
}

func (a *App) OpenLogsFolder() error {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	logsDir := logging.LogsDir(cfgDir)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}
	return openInFileManager(logsDir)
}

func openInFileManager(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

func (a *App) OpenBugReportURL() error {
	info := a.GetBuildInfo()
	body := fmt.Sprintf(`## What happened

<!-- Describe the bug -->

## What you expected

<!-- What should have happened instead -->

## Steps to reproduce

1.
2.
3.

---

**BRUV version:** %s
**Build date:** %s
**OS:** %s/%s
**Go version:** %s
`, info.Version, info.BuildDate, info.OS, info.Arch, info.GoVersion)

	params := url.Values{}
	params.Set("body", body)
	params.Set("labels", "bug")
	wailsRuntime.BrowserOpenURL(a.ctx, BugReportURL+"?"+params.Encode())
	return nil
}
