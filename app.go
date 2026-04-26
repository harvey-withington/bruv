package main

import (
	"encoding/hex"

	"bruv/core/events"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/logging"
	"bruv/internal/repo"
	"bruv/internal/update"
	transporthttp "bruv/transport/http"
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

// App is the desktop shell. Per-repo state + the 130+ per-repo RPC
// methods live on the embedded *supervisor.Runtime; App owns only
// what's genuinely desktop-specific: window/tray/bounds, the loopback
// HTTP server, repo lock, the supervisor itself, and connection
// management (which has to keep working when the active backend is
// unreachable, so the Wails Shell binds it directly).
//
// When no repo is open, the embedded Runtime is nil — promoted
// methods will panic if invoked, and the JSON-RPC dispatcher's
// recover() turns those into normal RPC errors. Internal Go callers
// should check `a.Runtime != nil` (or use a.HasRepository()) before
// reaching into per-repo state.
type App struct {
	*supervisor.Runtime // embedded — promotes all per-repo methods + service fields

	sup      *supervisor.Supervisor
	activeID string

	// Desktop-only lifecycle state.
	ctx             context.Context
	savedBounds     *config.WindowBounds
	boundsRestored  bool
	lastSavedBounds *config.WindowBounds
	forceQuit       bool
	trayPauseItem   interface{ Check(); Uncheck(); Checked() bool }

	// appBus is the stable host-side event bus the Wails bridge and
	// HTTP SSE handler subscribe to. Per-runtime publishers (the
	// services exposed via the embedded *Runtime) publish into the
	// runtime's own bus; bindRuntime sets up a fan-in goroutine that
	// re-publishes onto appBus. Keeping appBus stable across runtime
	// switches means the bridge + HTTP server don't have to rewire.
	appBus           *events.MemBus
	runtimeBusUnsub  func()
	wailsBridgeUnsub func()
	wailsBridgeOnce  sync.Once

	httpServer *transporthttp.Server
}

// NewApp constructs the desktop App. The supervisor is built from
// whatever's already in repos.json (post legacy-recents migration),
// but no Runtime is loaded — that happens lazily when the user opens
// a repo via OpenRepository / InitRepository / SetActiveRepo.
func NewApp() *App {
	a := &App{appBus: events.NewMemBus(128)}

	// Run the recents migration BEFORE constructing the supervisor so
	// freshly-imported entries land in the supervisor's registry view.
	config.MigrateRecentsToRegistry()

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
		// Fall back to an empty supervisor so the picker still works.
		sup, _ = supervisor.New(nil, cfgDir)
	}
	a.sup = sup
	return a
}

// bindRuntime swaps the active *Runtime, restarting the per-runtime
// fan-in goroutine that copies events from rt.Bus() into a.appBus.
// Pass rt = nil to clear (e.g. CloseRepository). Idempotent.
func (a *App) bindRuntime(rt *supervisor.Runtime) {
	if a.runtimeBusUnsub != nil {
		a.runtimeBusUnsub()
		a.runtimeBusUnsub = nil
	}
	a.Runtime = rt
	if rt == nil {
		return
	}
	ch, unsub := rt.Bus().Subscribe()
	a.runtimeBusUnsub = unsub
	go func() {
		defer logging.Recover("bindRuntime: fan-in")
		for ev := range ch {
			a.appBus.Publish(ev.Topic, ev.Payload)
		}
	}()
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Start the event-bus → Wails bridge. The goroutine drains the
	// bus subscription and forwards every published event to the
	// frontend via wailsRuntime.EventsEmit. This is the only place
	// Wails IPC touches the event stream — once an HTTP transport
	// lands, it becomes a second subscriber alongside this one.
	a.startBusBridge()

	// Start the HTTP transport on loopback. Phase 3 boots it for
	// parity testing; phase 4 migrates the frontend to call through
	// it instead of Wails IPC. Bind failures are non-fatal — the
	// Wails desktop still works without the HTTP surface.
	a.startHTTPTransport()

	// Self-enrol as a device against the newly-started server so
	// the frontend has a valid bearer token before it makes its
	// first RPC call. Runs only once per install — the resulting
	// device token is cached in clientdata/device-token.txt.
	a.ensureDesktopDeviceToken()

	// Initialise file logging to <configDir>/logs/bruv-YYYY-MM-DD.log.
	// Failure is non-fatal — stderr still gets everything — but we log
	// the failure itself via the standard log package so a dev running
	// the binary from a terminal can still see it.
	if cfgDir, err := config.ConfigDir(); err == nil {
		if _, err := logging.Init(cfgDir); err != nil {
			slog.Warn("logging init failed", "err", err)
		}
		// Crash reports land in <configDir>/crashes/ — separate from
		// the rolling daily log so they don't get pruned on retention.
		// Version/build-date go in every report so users don't have to
		// guess which build they were on when filing a bug.
		logging.InitCrashReporting(cfgDir, AppVersion, BuildDate)
	} else {
		slog.Warn("resolve config dir for logging failed", "err", err)
	}

	// Schema registry is per-runtime — loaded by buildRuntime when the
	// user opens a repo. NewApp ran the recents migration and loaded
	// the registry; nothing more to do here repo-side until the user
	// picks one.

	// Migrate legacy single-provider config to multi-account
	if err := config.MigrateLegacyLLMConfig(); err != nil {
		slog.Warn("legacy LLM config migration failed", "err", err)
	}

	// Upgrade any plaintext LLM API keys into the OS keychain. Idempotent
	// and safe on every startup — no-op if keys are already migrated or
	// the keychain is unavailable.
	if err := config.MigrateLLMKeysToKeychain(); err != nil {
		slog.Warn("LLM keychain migration failed", "err", err)
	}
}

// domReady is called after the frontend DOM is ready.
// We restore the saved window position here and show the window.
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

	// System tray icon (after window is fully ready)
	a.setupTray()
}

// startBoundsPoller starts a background goroutine that saves the window
// position and size every 3 seconds if they have changed. This ensures
// bounds are preserved even if the app is killed rather than closed normally.
func (a *App) startBoundsPoller() {
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

// saveCurrentBounds captures and persists the current window position and size.
// It skips the write if nothing has changed since the last save.
func (a *App) saveCurrentBounds() {
	maximised := wailsRuntime.WindowIsMaximised(a.ctx)
	x, y := wailsRuntime.WindowGetPosition(a.ctx)
	w, h := wailsRuntime.WindowGetSize(a.ctx)

	wb := &config.WindowBounds{
		X:         x,
		Y:         y,
		Width:     w,
		Height:    h,
		Maximised: maximised,
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

// beforeClose is called when the window is about to close.
// If agents are enabled, hide to background instead of quitting.
func (a *App) beforeClose(ctx context.Context) bool {
	a.saveCurrentBounds()

	if a.forceQuit {
		// Tear down per-repo resources via the supervisor — handles
		// scheduler, due-date scanner, MCP, watcher, index, lock.
		a.sup.Close()
		a.stopHTTPTransport()
		a.stopBusBridge()
		logging.Close()
		return false // allow quit
	}

	// Hide window instead of closing — agents keep running in background
	wailsRuntime.WindowHide(ctx)
	return true // prevent close
}

// startBusBridge begins draining the event bus and forwarding every
// published event to the frontend via wailsRuntime.EventsEmit. Must be
// called after a.ctx is populated (the Wails runtime methods require
// a valid context). Idempotent — calling twice replaces the prior
// subscription and cancels the old goroutine.
func (a *App) startBusBridge() {
	// Drop any prior subscription (e.g. on a hot reload in dev).
	if a.wailsBridgeUnsub != nil {
		a.wailsBridgeUnsub()
	}
	ch, unsub := a.appBus.Subscribe()
	a.wailsBridgeUnsub = unsub
	a.wailsBridgeOnce = sync.Once{}

	ctx := a.ctx
	go func() {
		defer logging.Recover("busBridge")
		for ev := range ch {
			// Preserve existing wire shape: frontend EventsOn
			// handlers receive the payload directly, not a wrapped
			// Event envelope. A future HTTP/WS transport will carry
			// the full envelope (ID, At) for resume-cursor support.
			wailsRuntime.EventsEmit(ctx, ev.Topic, ev.Payload)

			// Tray tooltip reflects unread notification count. The
			// agent runtime no longer refreshes the tray directly (it
			// must stay host-agnostic), so we piggy-back on the event
			// stream here — any source of notification:new keeps the
			// tooltip accurate on Windows.
			if ev.Topic == "notification:new" {
				go a.refreshTrayTooltip()
			}
		}
	}()
}

// stopBusBridge cancels the bus subscription and lets the forwarder
// goroutine exit. Safe to call multiple times.
func (a *App) stopBusBridge() {
	a.wailsBridgeOnce.Do(func() {
		if a.wailsBridgeUnsub != nil {
			a.wailsBridgeUnsub()
			a.wailsBridgeUnsub = nil
		}
	})
}

// startHTTPTransport binds the HTTP server to a random loopback port.
// Failures are logged but non-fatal — the Wails desktop still runs
// without the HTTP surface. Token + resolved addr go into slog so a
// developer tail-ing the log can hit the server from curl.
//
// The Svelte bundle is passed through as an embedded FS so the
// server can serve it at /app/* (Mode B). The desktop Wails shell
// still loads the bundle directly from its own embed; the HTTP path
// is there so a browser on the tailnet can reach the UI too.
func (a *App) startHTTPTransport() {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		slog.Warn("http transport: resolve config dir failed", "err", err)
		return
	}
	srv, err := transporthttp.New(transporthttp.Config{
		Addr:         "127.0.0.1:0",
		ConfigDir:    cfgDir,
		Version:      AppVersion,
		BuildDate:    BuildDate,
		StaticAssets: assets,
		Attachments: &transporthttp.AttachmentConfig{
			Secret:  config.LoadServerSecret(),
			Resolve: a.resolveAttachment,
		},
		LocalRegistry: &transporthttp.LocalRegistryConfig{
			List:       a.localRepoSummaries,
			SetEnabled: func(id string, enabled bool) error { return config.SetRepoDisabled(id, !enabled) },
		},
	}, a, a.appBus)
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

// GetHTTPTransportInfo returns the backend endpoint + bearer token
// the frontend should use. Resolution order (highest precedence
// first):
//
//  1. BRUV_REMOTE_URL + BRUV_REMOTE_TOKEN env vars — escape hatch
//     for dev/test; bypasses the connection store entirely.
//  2. Active connection from the persisted store (set via the
//     Connections UI). Lets the user point this device at a remote
//     server without env vars.
//  3. Local loopback HTTP server + self-enrolled device token —
//     the default when no remote is active.
//
// Returns empty strings when neither is available, which the cloud
// adapter surfaces as a "transport unavailable" error.
func (a *App) GetHTTPTransportInfo() map[string]string {
	if remoteURL := os.Getenv("BRUV_REMOTE_URL"); remoteURL != "" {
		if token := os.Getenv("BRUV_REMOTE_TOKEN"); token != "" {
			// Strip scheme if present — the cloud adapter expects
			// just host:port and adds http:// itself.
			addr := strings.TrimPrefix(strings.TrimPrefix(remoteURL, "https://"), "http://")
			return map[string]string{
				"addr":   addr,
				"token":  token,
				"scheme": schemeFromURL(remoteURL),
				"remote": "true",
			}
		}
		slog.Warn("BRUV_REMOTE_URL set without BRUV_REMOTE_TOKEN — falling back to active connection / loopback")
	}
	if active, _ := config.ActiveConnection(); active != nil {
		addr := strings.TrimPrefix(strings.TrimPrefix(active.URL, "https://"), "http://")
		// repoID is the user's last-selected repo on THIS connection,
		// from the per-device repo-recents store. Empty means "show
		// the picker" — the frontend uses this signal to skip the
		// auto-restore and render the RepoSelector instead.
		repoID := config.GetRecentRepoForConnection(active.ID)
		return map[string]string{
			"addr":   addr,
			"token":  active.DeviceToken,
			"scheme": schemeFromURL(active.URL),
			"remote": "true",
			"repoID": repoID,
		}
	}
	if a.httpServer == nil {
		return map[string]string{"addr": "", "token": ""}
	}
	return map[string]string{
		"addr":   a.httpServer.Addr(),
		"token":  desktopDeviceToken,
		"scheme": "http",
		"remote": "false",
	}
}

// --- Connections forwarders ---

// ListConnections returns the persisted set of remote connections
// plus the active pointer. The implicit "Local" connection is not
// included; callers (frontend) treat Active=="" as "use Local".
func (a *App) ListConnections() (config.ConnectionStore, error) {
	return config.LoadConnections()
}

// AddConnection persists a new remote connection. Caller is expected
// to have already exchanged the bootstrap token for a device token
// via /auth/enrol on the target server.
func (a *App) AddConnection(name, url, deviceToken string) (config.Connection, error) {
	return config.AddConnection(name, url, deviceToken)
}

// RemoveConnection drops a connection by ID. If it was active, the
// active pointer resets to "" (Local).
func (a *App) RemoveConnection(id string) error {
	return config.RemoveConnection(id)
}

// UpdateConnection edits a saved connection's name, URL, and/or
// device token. Empty fields are left unchanged.
func (a *App) UpdateConnection(id, name, url, deviceToken string) (config.Connection, error) {
	return config.UpdateConnection(id, name, url, deviceToken)
}

// SetActiveConnection switches the active pointer. The frontend is
// expected to reload after this so the cloud adapter re-resolves
// transport info against the new active.
func (a *App) SetActiveConnection(id string) error {
	return config.SetActiveConnection(id)
}

// SetActiveRepo persists the user's repo selection for the currently
// active connection. The frontend reloads after calling this so the
// cloud adapter picks up the new repo ID via GetHTTPTransportInfo
// and starts routing to /repos/<id>/...
func (a *App) SetActiveRepo(repoID string) error {
	active, err := config.ActiveConnection()
	if err != nil {
		return err
	}
	if active == nil {
		// Local: with the registry now backing the picker, repoID
		// IS meaningful — it identifies a specific entry in
		// repos.json. Open the corresponding folder; the page
		// reload that follows brings the rest of the UI into
		// sync. Silent no-op if the ID isn't registered (the
		// caller still reloads, so the user sees whatever was
		// open before — not destructive).
		store, err := config.LoadRepos()
		if err != nil {
			return err
		}
		for _, e := range store.Repos {
			if e.ID == repoID {
				return a.OpenRepository(e.Path)
			}
		}
		return nil
	}
	return config.SetRecentRepoForConnection(active.ID, repoID)
}

// --- Attachments (signed-URL helpers + repo resolver) ---

// resolveAttachment is the bridge from the transport HTTP handler
// (which doesn't know about internal/repo) into the open repository.
// Returns ok=false when the repo isn't open, the card is gone, or the
// attachment ID isn't on the card.
func (a *App) resolveAttachment(cardID, attachmentID string) (path, mime, name string, ok bool) {
	if a.Repo() == nil {
		return "", "", "", false
	}
	att, err := a.Repo().FindAttachment(cardID, attachmentID)
	if err != nil || att == nil {
		return "", "", "", false
	}
	return a.Repo().AttachmentPath(cardID, attachmentID), att.Mime, att.Name, true
}

// SignAttachmentURL builds a short-lived signed URL the frontend can
// drop straight into <img src> / <a href>. The 5-minute TTL bounds
// how long a leaked URL stays usable; download URLs are renewed on
// every page load anyway, so a short lifetime is invisible to users.
//
// The URL points at this device's local server when no remote
// connection is active, and at the active remote when there is one —
// matching how GetHTTPTransportInfo resolves the transport.
func (a *App) SignAttachmentURL(cardID, attachmentID string) (string, error) {
	if cardID == "" || attachmentID == "" {
		return "", fmt.Errorf("cardID and attachmentID are required")
	}
	const ttl = 5 * time.Minute
	exp := time.Now().Add(ttl).Unix()
	sig := transporthttp.SignAttachmentMAC(config.LoadServerSecret(), cardID, attachmentID, exp)

	info := a.GetHTTPTransportInfo()
	addr := info["addr"]
	if addr == "" {
		return "", fmt.Errorf("transport not available")
	}
	scheme := info["scheme"]
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/attachments/%s/%s?exp=%d&sig=%s",
		scheme, addr, cardID, attachmentID, exp, hex.EncodeToString(sig)), nil
}

// schemeFromURL returns "https" if the URL is https://..., else "http".
func schemeFromURL(u string) string {
	if strings.HasPrefix(u, "https://") {
		return "https"
	}
	return "http"
}

// logIdxErr logs a search-index operation failure without blocking the
// caller. Index writes are best-effort: card/repo data is already
// safely on disk by the time these call sites run, so a failed index
// update just means the in-memory search and agent-presence indexes
// are temporarily stale. We emit an "index:stale" event so the UI can
// prompt the user to rebuild via Settings → Advanced. No-op when err
// is nil so call sites can wrap unconditionally.
func (a *App) logIdxErr(op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("index update failed", "op", op, "err", err)
	a.appBus.Publish("index:stale", op)
}

// idxIncrementalRefresh wraps a.Index().IncrementalRefresh so call sites
// stay one-line. The many existing "if a.Index() != nil { ... }" guards
// remain in place, so this helper assumes a.Index() is non-nil.
func (a *App) idxIncrementalRefresh() {
	if _, err := a.Index().IncrementalRefresh(a.Repo().Root); err != nil {
		a.logIdxErr("IncrementalRefresh", err)
	}
}

// Version returns the current application version
func (a *App) Version() string {
	return AppVersion
}

// GetBuildInfo returns full version and build metadata for the About dialog.
func (a *App) GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   AppVersion,
		BuildDate: BuildDate,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}
}

// OpenConfigFolder opens the BRUV config directory in the user's native file
// manager. Users call this via About → Open config folder to find logs, back
// up their data, or inspect the JSON files BRUV stores.
func (a *App) OpenConfigFolder() error {
	dir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// OpenLogsFolder opens BRUV's logs directory in the user's native file
// manager. Attached to the About dialog so users filing bug reports
// can find and attach their recent log file.
func (a *App) OpenLogsFolder() error {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	logsDir := logging.LogsDir(cfgDir)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", logsDir).Start()
	case "darwin":
		return exec.Command("open", logsDir).Start()
	default:
		return exec.Command("xdg-open", logsDir).Start()
	}
}

// OpenBugReportURL opens a pre-filled GitHub issue in the user's default
// browser with version and OS information baked into the body so users
// don't have to hunt for it when filing bugs.
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
	fullURL := BugReportURL + "?" + params.Encode()

	wailsRuntime.BrowserOpenURL(a.ctx, fullURL)
	return nil
}

// CheckForUpdates queries GitHub Releases for the latest BRUV version and
// returns a Result the frontend can render. This is intentionally a manual
// check triggered by the About dialog, not a background poller — the app
// never phones home on its own.
func (a *App) CheckForUpdates() update.Result {
	return update.Check(AppVersion)
}

// MarkLLMNudgeShown lives in app_settings.go (per-machine, nil-safe).

// PickFolder opens a native folder picker dialog and returns the selected path.
func (a *App) PickFolder(title string) (string, error) {
	result, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

// PickFile opens a native file picker and returns the selected path. The
// filterPattern is a glob like "*.json" (space-separated for multi); filterName
// is the human-readable label shown in the dialog's type dropdown.
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

// PickSaveFile opens a native save-file dialog and returns the chosen path.
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

// --- Repository Management ---

// RepoInspectInfo is the result of InspectRepoPath — surfaces just
// enough for the UI to decide between Open (existing repo, name
// known) and Init (fresh folder, ask user for name).
type RepoInspectInfo struct {
	Exists bool   `json:"exists"`
	Name   string `json:"name"`
	ID     string `json:"id"`
}

// InspectRepoPath checks whether the given folder is already a BRUV
// repository, returning its name + ID when so. Used by the unified
// repo-add flow: the UI picks a folder once, then we tell it
// whether to show "Open this existing repo" or "Name your new repo".
// Returns Exists=false (not an error) when the path is a normal
// folder that simply isn't a BRUV repo yet.
func (a *App) InspectRepoPath(path string) (*RepoInspectInfo, error) {
	m, err := repo.InspectAt(path)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return &RepoInspectInfo{Exists: false}, nil
	}
	return &RepoInspectInfo{Exists: true, Name: m.Name, ID: m.ID}, nil
}

// InitRepository creates a new BRUV repository at exactly the given
// path, registers it in repos.json, and binds it as the active
// runtime. The unified repo-add flow uses the OS folder picker
// (including its built-in "New Folder") so the path the user picks
// IS the repo root. Returns the absolute path on success.
func (a *App) InitRepository(path, name string) (string, error) {
	r, err := repo.InitAt(path, name)
	if err != nil {
		return "", err
	}
	return a.openLoaded(r.Root, name)
}

// OpenRepository opens an existing BRUV repository, registers it (no-op
// if already in repos.json), and binds the resulting runtime.
func (a *App) OpenRepository(path string) error {
	r, err := repo.Open(path)
	if err != nil {
		return err
	}
	// Revalidate before the supervisor does its incremental index
	// refresh — keeps stale pins / orphaned files out of the rebuild.
	if repairStats, revErr := r.Revalidate(); revErr != nil {
		slog.Warn("revalidation failed", "err", revErr)
	} else {
		slog.Info("revalidate ok", "stats", repairStats.String())
	}
	_, err = a.openLoaded(r.Root, r.Manifest.Name)
	return err
}

// openLoaded is the shared lifecycle path for InitRepository +
// OpenRepository: register the path, ask the supervisor to load a
// runtime, acquire the desktop-only repo lock, and bind the runtime
// as active. Returns the absolute repo root.
func (a *App) openLoaded(path, name string) (string, error) {
	a.registerInLocalRegistry(path, name)
	rt, err := a.sup.RegisterAndLoad(path)
	if err != nil {
		return "", err
	}
	if lockErr := a.acquireRepoLock(rt.Repo().Root); lockErr != nil {
		// Lock is advisory: log and proceed. Hard-refusing would make
		// concurrent same-user sessions (e.g. tray re-open of a hidden
		// window) intolerable.
		slog.Warn("open: repo lock failed", "err", lockErr)
	}
	a.bindRuntime(rt)
	if entry, ok := a.sup.EntryByPath(rt.Repo().Root); ok {
		a.activeID = entry.ID
	}
	return rt.Repo().Root, nil
}

// HasRepository returns true if a repository is currently open.
func (a *App) HasRepository() bool {
	return a.Runtime != nil
}

// registerInLocalRegistry idempotently writes the given repo to
// <userConfigDir>/repos.json — the same registry the headless server
// uses, so Local appears in the picker as a first-class connection
// with a real list of repos. AppendRepo is a no-op when the path is
// already registered. The entry's ID is also stamped as the
// "last opened on Local" pointer so reopen_last_repo can find it
// without depending on the deleted recents list. Failures are
// logged-and-swallowed — the user's repo still opens even if the
// registry write fails (registry is a view, not a gate).
func (a *App) registerInLocalRegistry(path, name string) {
	entry, err := config.AppendRepo(path, name)
	if err != nil {
		slog.Warn("local registry: append failed", "path", path, "err", err)
		return
	}
	if err := config.SetRecentRepoForConnection("", entry.ID); err != nil {
		slog.Warn("local registry: stamp last-opened failed", "id", entry.ID, "err", err)
	}
}

// GetLastOpenedLocalRepoPath returns the filesystem path of the
// most recently opened Local repo, or "" if none is recorded. Used
// by the frontend's reopen_last_repo flow to skip the picker when
// the user has only one (or one that's reliably theirs).
func (a *App) GetLastOpenedLocalRepoPath() string {
	id := config.GetRecentRepoForConnection("")
	if id == "" {
		return ""
	}
	store, err := config.LoadRepos()
	if err != nil {
		return ""
	}
	for _, e := range store.Repos {
		if e.ID == id {
			return e.Path
		}
	}
	return ""
}

// localRepoSummaries adapts repos.json into the transport's
// RepoSummary shape, for the registry-backed GET /repos handler.
// Errors are logged-and-swallowed → empty list, on the principle
// that a broken registry shouldn't take the picker down (the user
// can still PickFolder + InitRepository to recover).
func (a *App) localRepoSummaries() []transporthttp.RepoSummary {
	store, err := config.LoadRepos()
	if err != nil {
		slog.Warn("local registry: list failed", "err", err)
		return []transporthttp.RepoSummary{}
	}
	out := make([]transporthttp.RepoSummary, 0, len(store.Repos))
	for _, e := range store.Repos {
		out = append(out, transporthttp.RepoSummary{
			ID:       e.ID,
			Name:     e.Name,
			Disabled: e.Disabled,
		})
	}
	return out
}

// ListLocalRepos returns the desktop's repos.json registry. Used by
// the picker to render Local's repo list (parity with Remote's GET
// /repos endpoint). The registry is the source of truth — everything
// the user has ever opened or initialised on this machine appears
// here, with Disabled flagging entries the user has paused.
func (a *App) ListLocalRepos() ([]config.RepoEntry, error) {
	store, err := config.LoadRepos()
	if err != nil {
		return nil, err
	}
	return store.Repos, nil
}

// RemoveLocalRepo drops an entry from <userConfigDir>/repos.json. The
// underlying folder on disk is left alone — this is a registry-only
// operation, mirroring the X button on a recent repo today.
func (a *App) RemoveLocalRepo(id string) error {
	return config.RemoveRepo(id)
}

// SetLocalRepoEnabled flips the Disabled flag on a Local registry
// entry. On Local today this only affects whether the picker shows
// the row as enabled — the desktop App still opens whichever repo
// the user explicitly selects. The flag becomes load-bearing once the
// supervisor pattern lands on desktop (future sprint).
func (a *App) SetLocalRepoEnabled(id string, enabled bool) error {
	return config.SetRepoDisabled(id, !enabled)
}

// RenameLocalRepo updates a Local repo's name in BOTH the per-machine
// registry (repos.json — controls picker display) and the in-repo
// manifest (manifest.json — the portable identity that travels with
// the repo). Editing both keeps display and identity in sync; either
// alone would drift when the repo is shared. Failure to write the
// manifest doesn't roll back the registry — we surface the error and
// let the user retry; partial-success is preferable to leaving the
// picker showing the old name.
func (a *App) RenameLocalRepo(id, name string) error {
	store, err := config.LoadRepos()
	if err != nil {
		return err
	}
	var path string
	for _, e := range store.Repos {
		if e.ID == id {
			path = e.Path
			break
		}
	}
	if path == "" {
		return fmt.Errorf("repo %q not found", id)
	}
	if err := config.SetRepoName(id, name); err != nil {
		return err
	}
	if err := repo.RewriteManifestName(path, name); err != nil {
		return fmt.Errorf("update manifest: %w", err)
	}
	return nil
}

// CloseRepository releases the active repository: shuts down the
// runtime via the supervisor (which stops scheduler, due-date scanner,
// MCP, watcher, and closes the index), releases the desktop repo lock,
// and clears the embedded *Runtime pointer. The registry entry stays
// in repos.json — close ≠ remove.
func (a *App) CloseRepository() {
	a.releaseRepoLock()
	if a.activeID != "" {
		a.sup.Unload(a.activeID)
		a.activeID = ""
	}
	a.bindRuntime(nil)
}

// GetCurrentRepo reports what repo the backend currently has open.
// Wraps the embedded Runtime's promoted version with a nil-runtime
// guard so the desktop's pre-open state returns null (legacy contract)
// instead of panicking. Frontend uses this at boot to skip the
// welcome screen when the backend is a remote (fixed repo) or when
// the desktop has auto-reopened the last repo.
func (a *App) GetCurrentRepo() *supervisor.CurrentRepoInfo {
	if a.Runtime == nil {
		return nil
	}
	return a.Runtime.GetCurrentRepo()
}

// MarkNotificationRead, MarkAllNotificationsRead, ClearAllNotifications
// override the promoted Runtime versions only to refresh the tray
// tooltip after the underlying mutation succeeds. Tray refresh is a
// desktop-only side effect; the runtime methods stay host-agnostic.

func (a *App) MarkNotificationRead(id string) error {
	if a.Runtime == nil {
		return nil
	}
	err := a.Runtime.MarkNotificationRead(id)
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) MarkAllNotificationsRead() error {
	if a.Runtime == nil {
		return nil
	}
	err := a.Runtime.MarkAllNotificationsRead()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) ClearAllNotifications() error {
	if a.Runtime == nil {
		return nil
	}
	err := a.Runtime.ClearAllNotifications()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}


// coerceBlockValue converts an LLM-provided value into the format expected
// by the given block type. LLMs may ignore the schema and send strings for
// booleans/numbers, or flat text for checklists/lists, so we handle all of
// that here. The goal is to keep the block renderable no matter what shape
// the model sent, because LLMs are inconsistent and a corrupted block is a
// silent failure the user only notices when they look at the card.
//
// This variant knows only the block type. For constraint checks that need
// block.Meta (select options, rating max, etc.), use coerceBlockValueForBlock.
