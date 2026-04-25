package main

import (
	"encoding/hex"

	"bruv/core/events"
	"bruv/core/reposync"
	agentrt "bruv/core/runtime/agent"
	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/prompts"
	"bruv/core/runtime/tools"
	agentsvc "bruv/core/services/agentsvc"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	chatsvc "bruv/core/services/chat"
	reposvc "bruv/core/services/repository"
	transporthttp "bruv/transport/http"
	llmsvc "bruv/core/services/llm"
	"bruv/core/services/mcpsvc"
	"bruv/core/services/notify"
	projectsvc "bruv/core/services/project"
	"bruv/core/services/search"
	"bruv/core/services/settings"
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/logging"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"bruv/internal/update"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

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

// App struct
type App struct {
	ctx             context.Context
	repo            *repo.Repository
	registry        *schema.Registry
	idx             *index.Index
	savedBounds     *config.WindowBounds
	boundsRestored  bool
	lastSavedBounds *config.WindowBounds
	// llmActors tracks active LLM chat sessions by cardID → model name.
	// Set during SendChatMessage so logActivity can attribute edits to the correct actor.
	llmActors sync.Map
	// Scheduler, due-date scanner, and agent cancel map moved onto the
	// agent runtime (see core/runtime/agent). Access via a.agentRT.
	forceQuit     bool
	trayPauseItem interface{ Check(); Uncheck(); Checked() bool } // system tray "Pause Agents" menu item
	// mcpRegistry manages external MCP server subprocesses for the
	// currently open repo. Nil when no repo is open. Tools exposed by
	// MCP servers appear in the agent tool catalogue alongside
	// built-in tools and dispatch through the same executeAgentToolCall
	// path via a namespaced ID prefix. See internal/mcp for the full
	// architecture and docs/mcp-servers.md for user-facing docs.
	mcpRegistry *mcp.Registry
	// Service registry — each extracted service lives in core/services/<name>.
	// App retains domain coordination (repo lifecycle, window, tray) and
	// forwards Wails-bound methods to the matching service.
	search     *search.Service
	notify     *notify.Service
	mcpService *mcpsvc.Service
	llmService *llmsvc.Service
	settings   *settings.Service
	catalog     *catalog.Service
	project     *projectsvc.Service
	cardService  *card.Service
	chat         *chatsvc.Service
	agentService *agentsvc.Service
	repoService  *reposvc.Service

	// tools is the LLM tool dispatcher, extracted from app_tools.go
	// in the LLM-runtime-extraction stage-1 pass. See core/runtime/tools.
	tools *tools.Dispatcher

	// prompts is the LLM system-prompt builder (card / project / agent),
	// extracted from app_chat.go + app_agent.go in stage 2 of the
	// LLM-runtime extraction. See core/runtime/prompts.
	prompts *prompts.Builder

	// chatRT is the chat runtime — runChatLoop + SendCard + SendProject,
	// extracted from app_chat.go in stage 3. See core/runtime/chat.
	chatRT *chatrt.Runtime

	// agentRT is the agent execution runtime — scheduler + due-date
	// scanner + executeAgent + MCP tool bridging, extracted from
	// app_agent.go in stage 4. See core/runtime/agent.
	agentRT *agentrt.Runtime

	// bus is the transport-agnostic event bus. Domain code publishes
	// to it via a.bus.Publish; at startup a bridge goroutine subscribes
	// and forwards every event to wailsRuntime.EventsEmit. When a
	// WebSocket transport lands in phase 3 it becomes a second
	// subscriber — no domain code changes.
	bus         *events.MemBus
	busUnsub    func()
	busStopOnce sync.Once

	// httpServer is the HTTP + SSE transport. Boots on a random
	// 127.0.0.1 port from the Wails desktop in phase 3 so the
	// frontend can migrate to speaking HTTP in phase 4 without any
	// new server code. Remote deployment (Modes A/B) will bind the
	// same server to the tailnet interface instead of loopback.
	httpServer *transporthttp.Server

	// repoWatcher is the file-system watcher for the currently-open
	// repo. It publishes card:updated/project:updated/etc events when
	// files change externally (git pull, Syncthing, hand-edit), which
	// is how sync-based collaboration flows to the UI. Nil when no
	// repo is open.
	repoWatcher *reposync.Watcher
}

// repositoryDeps adapts App to reposvc.Deps.
type repositoryDeps struct{ app *App }

func (d repositoryDeps) Repo() *repo.Repository { return d.app.repo }
func (d repositoryDeps) Index() *index.Index    { return d.app.idx }

// agentServiceDeps adapts App to agentsvc.Deps.
type agentServiceDeps struct{ app *App }

func (d agentServiceDeps) Repo() *repo.Repository { return d.app.repo }
func (d agentServiceDeps) Index() *index.Index    { return d.app.idx }

// chatDeps adapts App to chatsvc.Deps.
type chatDeps struct{ app *App }

func (d chatDeps) Repo() *repo.Repository { return d.app.repo }

// cardDeps adapts App to card.Deps. LogActivity bridges to the
// user-or-LLM actor resolution that still lives on App; ApplyTypeBlocks
// bridges to the catalog service so card service doesn't need to know
// about schema.
type cardDeps struct{ app *App }

func (d cardDeps) Repo() *repo.Repository { return d.app.repo }
func (d cardDeps) Index() *index.Index    { return d.app.idx }
func (d cardDeps) ApplyTypeBlocks(cardID, cardType string) {
	d.app.catalog.ApplyTypeBlocks(cardID, cardType)
}
func (d cardDeps) LogActivity(cardID, action, field string) {
	d.app.logActivity(cardID, action, field)
}
func (d cardDeps) LogActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []card.CategoryPath) {
	d.app.logActivityWithContext(cardID, action, field, cardTitle, breadcrumbs)
}
func (d cardDeps) Publish(topic string, payload any) { d.app.bus.Publish(topic, payload) }

// projectDeps adapts App to projectsvc.Deps.
type projectDeps struct{ app *App }

func (d projectDeps) Repo() *repo.Repository         { return d.app.repo }
func (d projectDeps) Index() *index.Index            { return d.app.idx }
func (d projectDeps) Publish(topic string, p any)    { d.app.bus.Publish(topic, p) }

// catalogDeps adapts App to catalog.Deps. UpdateCardBlocks is exposed
// because catalog's template-merge path writes back to card blocks;
// it delegates to the App method, which will forward to card service
// once that lands.
type catalogDeps struct{ app *App }

func (d catalogDeps) Repo() *repo.Repository     { return d.app.repo }
func (d catalogDeps) Registry() *schema.Registry { return d.app.registry }
func (d catalogDeps) Index() *index.Index        { return d.app.idx }
func (d catalogDeps) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return d.app.UpdateCardBlocks(id, blocks)
}
func (d catalogDeps) Publish(topic string, payload any) { d.app.bus.Publish(topic, payload) }

// llmDeps adapts App to llmsvc.Deps — exposes the Wails-bound ctx
// that test-connection probes use for their 30s timeout.
type llmDeps struct{ app *App }

func (d llmDeps) Ctx() context.Context { return d.app.ctx }

// searchDeps adapts App to the search.Deps interface without exposing
// repo/index on App's public surface (which would bind them to Wails).
type searchDeps struct{ app *App }

func (d searchDeps) Repo() *repo.Repository { return d.app.repo }
func (d searchDeps) Index() *index.Index    { return d.app.idx }

// NewApp creates a new App application struct
func NewApp() *App {
	a := &App{}
	a.bus = events.NewMemBus(128)
	a.search = search.New(searchDeps{app: a})
	a.notify = notify.New()
	a.mcpService = mcpsvc.New(mcpDeps{app: a})
	a.llmService = llmsvc.New(llmDeps{app: a})
	a.settings = settings.New()
	a.catalog = catalog.New(catalogDeps{app: a})
	a.project = projectsvc.New(projectDeps{app: a})
	a.cardService = card.New(cardDeps{app: a})
	a.chat = chatsvc.New(chatDeps{app: a})
	a.agentService = agentsvc.New(agentServiceDeps{app: a})
	a.repoService = reposvc.New(repositoryDeps{app: a})
	a.tools = tools.New(toolsDeps{app: a})
	a.prompts = prompts.New(promptsDeps{app: a})
	a.chatRT = chatrt.New(chatDepsRT{app: a})
	a.agentRT = agentrt.New(agentDepsRT{app: a})
	return a
}

// chatDepsRT adapts App to chatrt.Deps — exposes repo + schema,
// context + the LLM / card services, the tool dispatcher + prompt
// builder, and the two mutable state handles (MCP registry + the
// llmActors sync.Map) the chat loop threads through.
//
// Named chatDepsRT so it doesn't collide with the existing `chatDeps`
// used for the chat-history service adapter (see core/services/chat).
type chatDepsRT struct{ app *App }

func (d chatDepsRT) Repo() *repo.Repository            { return d.app.repo }
func (d chatDepsRT) Registry() *schema.Registry        { return d.app.registry }
func (d chatDepsRT) Ctx() context.Context              { return d.app.ctx }
func (d chatDepsRT) LLM() *llmsvc.Service              { return d.app.llmService }
func (d chatDepsRT) Card() *card.Service               { return d.app.cardService }
func (d chatDepsRT) Tools() *tools.Dispatcher          { return d.app.tools }
func (d chatDepsRT) Prompts() *prompts.Builder         { return d.app.prompts }
func (d chatDepsRT) MCPRegistry() *mcp.Registry        { return d.app.mcpRegistry }
func (d chatDepsRT) LLMActors() *sync.Map              { return &d.app.llmActors }

// agentDepsRT adapts App to agentrt.Deps — exposes the full set of
// handles the agent runtime needs: repo + index + schema + ctx, the
// event bus (for agent:started/completed/card:updated/scheduler:paused
// notifications), the four services the built-in tool dispatch writes
// through, the two sub-runtimes (prompts + chat), and the mutable
// runtime state (MCP registry + llmActors).
type agentDepsRT struct{ app *App }

func (d agentDepsRT) Repo() *repo.Repository            { return d.app.repo }
func (d agentDepsRT) Index() *index.Index               { return d.app.idx }
func (d agentDepsRT) Registry() *schema.Registry        { return d.app.registry }
func (d agentDepsRT) Ctx() context.Context              { return d.app.ctx }
func (d agentDepsRT) Publish(topic string, payload any) { d.app.bus.Publish(topic, payload) }
func (d agentDepsRT) LLM() *llmsvc.Service              { return d.app.llmService }
func (d agentDepsRT) Card() *card.Service               { return d.app.cardService }
func (d agentDepsRT) Project() *projectsvc.Service      { return d.app.project }
func (d agentDepsRT) Catalog() *catalog.Service         { return d.app.catalog }
func (d agentDepsRT) Prompts() *prompts.Builder         { return d.app.prompts }
func (d agentDepsRT) ChatRT() *chatrt.Runtime           { return d.app.chatRT }
func (d agentDepsRT) MCPRegistry() *mcp.Registry        { return d.app.mcpRegistry }
func (d agentDepsRT) LLMActors() *sync.Map              { return &d.app.llmActors }

// toolsDeps adapts App to tools.Deps — exposes repo, schema registry,
// bus publish, and the three services the tool dispatcher mutates
// through. Matches the per-service-adapter pattern used elsewhere.
type toolsDeps struct{ app *App }

func (d toolsDeps) Repo() *repo.Repository                   { return d.app.repo }
func (d toolsDeps) Registry() *schema.Registry               { return d.app.registry }
func (d toolsDeps) Publish(topic string, payload any)        { d.app.bus.Publish(topic, payload) }
func (d toolsDeps) Card() *card.Service                      { return d.app.cardService }
func (d toolsDeps) Project() *projectsvc.Service             { return d.app.project }
func (d toolsDeps) Catalog() *catalog.Service                { return d.app.catalog }

// promptsDeps adapts App to prompts.Deps — prompt builders read repo
// metadata, the schema registry, and the card + search services.
// They never mutate, so no Publish or Project/Catalog handles are
// needed here.
type promptsDeps struct{ app *App }

func (d promptsDeps) Repo() *repo.Repository     { return d.app.repo }
func (d promptsDeps) Registry() *schema.Registry { return d.app.registry }
func (d promptsDeps) Card() *card.Service        { return d.app.cardService }
func (d promptsDeps) Search() *search.Service    { return d.app.search }

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

	// Load the card type schema registry
	reg, err := schema.NewRegistry()
	if err != nil {
		slog.Warn("card type schema load failed", "err", err)
	}
	a.registry = reg

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
		a.agentRT.StopScheduler()
		a.agentRT.StopDueDateScanner()
		a.stopMCPRegistry()
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
	if a.busUnsub != nil {
		a.busUnsub()
	}
	ch, unsub := a.bus.Subscribe()
	a.busUnsub = unsub
	a.busStopOnce = sync.Once{}

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
	a.busStopOnce.Do(func() {
		if a.busUnsub != nil {
			a.busUnsub()
			a.busUnsub = nil
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
	}, a, a.bus)
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
		return map[string]string{
			"addr":   addr,
			"token":  active.DeviceToken,
			"scheme": schemeFromURL(active.URL),
			"remote": "true",
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

// SetActiveConnection switches the active pointer. The frontend is
// expected to reload after this so the cloud adapter re-resolves
// transport info against the new active.
func (a *App) SetActiveConnection(id string) error {
	return config.SetActiveConnection(id)
}

// --- Attachments (signed-URL helpers + repo resolver) ---

// resolveAttachment is the bridge from the transport HTTP handler
// (which doesn't know about internal/repo) into the open repository.
// Returns ok=false when the repo isn't open, the card is gone, or the
// attachment ID isn't on the card.
func (a *App) resolveAttachment(cardID, attachmentID string) (path, mime, name string, ok bool) {
	if a.repo == nil {
		return "", "", "", false
	}
	att, err := a.repo.FindAttachment(cardID, attachmentID)
	if err != nil || att == nil {
		return "", "", "", false
	}
	return a.repo.AttachmentPath(cardID, attachmentID), att.Mime, att.Name, true
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
	a.bus.Publish("index:stale", op)
}

// idxIncrementalRefresh wraps a.idx.IncrementalRefresh so call sites
// stay one-line. The many existing "if a.idx != nil { ... }" guards
// remain in place, so this helper assumes a.idx is non-nil.
func (a *App) idxIncrementalRefresh() {
	if _, err := a.idx.IncrementalRefresh(a.repo.Root); err != nil {
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

// MarkLLMNudgeShown persists a flag so the first-run LLM-configuration
// nudge only fires once per install.
func (a *App) MarkLLMNudgeShown() error {
	return a.settings.MarkLLMNudgeShown()
}

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

// InitRepository creates a new BRUV repository under the given base path.
// A subfolder is created automatically using the slugified repo name.
// Returns the actual repo root path on success.
func (a *App) InitRepository(basePath, name string) (string, error) {
	r, err := repo.Init(basePath, name)
	if err != nil {
		return "", err
	}
	a.repo = r

	// Load any community card type schemas from the types/ directory
	if a.registry != nil {
		_ = a.registry.LoadExternalTypes(filepath.Join(r.Root, "types"))
	}

	// Open the SQLite index and do an initial (empty) rebuild
	if err := a.openIndex(r.Root); err != nil {
		slog.Warn("open index failed", "repo", r.Root, "err", err)
	}

	// Ship the sync-hygiene files so users who put this repo under
	// git / Syncthing / Dropbox don't accidentally sync derived state.
	if err := repo.EnsureSyncHygiene(r.Root); err != nil {
		slog.Warn("init: ensure sync hygiene failed", "err", err)
	}

	// Point the repo at its server-side runs directory so agent
	// run history stays out of the synced repo tree.
	if err := a.configureRunsDir(r); err != nil {
		slog.Warn("init: configure runs dir failed", "err", err)
	}

	// Acquire the repo lock so a second BRUV process can't mutate the
	// same repo concurrently. Failure here is non-fatal on a fresh
	// init — but surface it so the user knows.
	if err := a.acquireRepoLock(r.Root); err != nil {
		slog.Warn("init: repo lock failed", "err", err)
	}

	// Add to recent repos
	_ = config.AddRecent(r.Root, name)

	// Heal tag colours in the background — no-op for a fresh repo, safe to run.
	go a.healTagColors()

	// Start watching the repo for external file changes so a sync
	// tool's writes (git pull, Syncthing) surface to the UI.
	a.startRepoWatcher()

	return r.Root, nil
}

// OpenRepository opens an existing BRUV repository.
func (a *App) OpenRepository(path string) error {
	r, err := repo.Open(path)
	if err != nil {
		return err
	}
	a.repo = r

	if a.registry != nil {
		_ = a.registry.LoadExternalTypes(path + "/types")
	}

	// Revalidate repo data (remove stale pins, orphaned files, etc.)
	if repairStats, err := r.Revalidate(); err != nil {
		slog.Warn("revalidation failed", "err", err)
	} else {
		slog.Info("revalidate ok", "stats", repairStats.String())
	}

	// Open the SQLite index and do an incremental refresh
	if err := a.openIndex(path); err != nil {
		slog.Warn("open index failed", "repo", path, "err", err)
	} else if a.idx != nil {
		if _, err := a.idx.IncrementalRefresh(path); err != nil {
			slog.Warn("index refresh failed", "repo", path, "err", err)
		}
	}

	// Add to recent repos
	_ = config.AddRecent(path, r.Manifest.Name)

	// Heal tag colours in the background — no-op if already healthy.
	go a.healTagColors()

	// Start agent scheduler
	a.agentRT.StartScheduler()

	// Start due-date notification scanner
	a.agentRT.StartDueDateScanner()

	// Start MCP registry for this repo. Failures here don't block
	// opening the repo — servers that can't start are surfaced in
	// the Settings UI and their tools are simply absent from the
	// agent catalogue. A broken MCP config must never prevent
	// access to the card data.
	a.startMCPRegistry()

	// Make sure the sync-hygiene files are present (older repos
	// predate this). Best-effort.
	if err := repo.EnsureSyncHygiene(r.Root); err != nil {
		slog.Warn("open: ensure sync hygiene failed", "err", err)
	}

	// Wire up the server-side runs directory. The first
	// GetAgentConfig call on each card will migrate embedded runs
	// out of the in-repo .agent.json if necessary (one-shot per card).
	if err := a.configureRunsDir(r); err != nil {
		slog.Warn("open: configure runs dir failed", "err", err)
	}

	// Acquire the repo lock. If another BRUV process already holds
	// it, OpenRepository still proceeds (we log) — hard-refusing
	// would make concurrent same-user sessions intolerable. The lock
	// is advisory: it exists so users can detect stale-process
	// conditions and as a hook for future stronger guarantees.
	if err := a.acquireRepoLock(r.Root); err != nil {
		slog.Warn("open: repo lock failed", "err", err)
	}

	// Start watching the repo for external file changes.
	a.startRepoWatcher()

	return nil
}

// startMCPRegistry brings up the per-repo MCP registry. Reads the
// repo-scoped server config, constructs a Registry keyed by the
// repo's stable manifest ID, and kicks off every enabled server.
// Startup errors are logged per server; the registry is still
// installed so the UI can display failures and allow recovery.
func (a *App) startMCPRegistry() {
	if a.repo == nil {
		return
	}
	store, err := a.repo.LoadMCPServerStore()
	if err != nil {
		slog.Warn("mcp load server store failed", "err", err)
		return
	}
	reg := mcp.NewRegistry(a.repo.Manifest.ID, config.MCPSecretResolver{})
	// Use a generous context so slow startups (npm cold start on
	// Windows can take 10+ seconds on first run) don't all fail.
	startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errs := reg.LoadAndStart(startCtx, store.Servers)
	for name, err := range errs {
		slog.Warn("mcp server startup failed", "server", name, "err", err)
	}
	a.mcpRegistry = reg
}

// stopMCPRegistry shuts down every server subprocess owned by the
// current registry. Called as part of CloseRepository so closing a
// repo doesn't leave orphaned subprocesses running in the background.
func (a *App) stopMCPRegistry() {
	if a.mcpRegistry == nil {
		return
	}
	a.mcpRegistry.Shutdown()
	a.mcpRegistry = nil
}

// HasRepository returns true if a repository is currently open.
func (a *App) HasRepository() bool {
	return a.repo != nil
}

// CloseRepository closes the current repository and its index.
func (a *App) CloseRepository() {
	a.agentRT.StopScheduler()
	a.agentRT.StopDueDateScanner()
	a.stopMCPRegistry()
	a.stopRepoWatcher()
	a.releaseRepoLock()
	if a.idx != nil {
		a.idx.Close()
		a.idx = nil
	}
	a.repo = nil
}

// startRepoWatcher kicks off the external-change file watcher for the
// currently-open repo. Publishes into the same event bus that domain
// mutations use — subscribers don't care whether the change came from
// a local mutation or a sync-tool pull.
func (a *App) startRepoWatcher() {
	if a.repo == nil {
		return
	}
	w, err := reposync.Start(a.repo.Root, a.bus)
	if err != nil {
		slog.Warn("repo watcher: start failed", "err", err)
		return
	}
	a.repoWatcher = w
}

// stopRepoWatcher tears down the file watcher. Safe to call when the
// watcher never started.
func (a *App) stopRepoWatcher() {
	if a.repoWatcher == nil {
		return
	}
	a.repoWatcher.Stop()
	a.repoWatcher = nil
}

// ListRecentRepos returns recently opened repositories.
// --- Repository forwarders (core/services/repository) ---

func (a *App) ListRecentRepos() ([]config.RecentRepo, error) { return a.repoService.ListRecentRepos() }
func (a *App) RemoveRecentRepo(path string) error            { return a.repoService.RemoveRecentRepo(path) }
func (a *App) GetRepoDescription() (string, error)           { return a.repoService.GetDescription() }
func (a *App) UpdateRepoDescription(description string) error {
	return a.repoService.UpdateDescription(description)
}

// --- Card ---

// logActivity records a card action to the activity log asynchronously.
// logActivityWithContext is the low-level activity writer. cardTitle and breadcrumbs
// must be captured before calling (e.g. before a card is deleted). Actor resolution
// is the only work deferred to the goroutine.
func (a *App) logActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []CategoryPath) {
	if a.repo == nil {
		return
	}
	go func() {
		defer logging.Recover("logActivityWithContext")
		var actorID, actor, actorType string
		if v, ok := a.llmActors.Load(cardID); ok {
			actor = v.(string)
			// LLM model name is stable enough to use as the shard key —
			// every machine running the same model writes to the same
			// shard, but they're never running the same agent at the
			// same time (the scheduler enforces single-flight per card),
			// so no append race.
			actorID = actor
			actorType = "llm"
		} else {
			p, _ := config.LoadProfile()
			actor = p.DisplayName
			if actor == "" {
				actor = "User"
			}
			actorID = config.LoadDeviceID()
			actorType = "user"
		}

		entry := model.ActivityEntry{
			ID:        uuid.New().String(),
			Timestamp: time.Now().UTC(),
			ActorID:   actorID,
			Actor:     actor,
			ActorType: actorType,
			Action:    action,
			Field:     field,
			CardID:    cardID,
			CardTitle: cardTitle,
		}

		if len(breadcrumbs) > 0 {
			bc := breadcrumbs[0]
			entry.BrandSlug = bc.BrandSlug
			entry.StreamSlug = bc.StreamSlug
			entry.ProjectSlug = bc.ProjectSlug
			entry.BrandName = bc.BrandName
			entry.StreamName = bc.StreamName
			entry.ProjectName = bc.ProjectName
			entry.CategoryName = bc.CategoryName
		}

		a.repo.AppendActivity(entry)
	}()
}

// logActivity captures card context synchronously then logs asynchronously.
// It is safe to call from any goroutine; errors are silently swallowed.
// If a.llmActors has an entry for cardID the action is attributed to that LLM model;
// otherwise it is attributed to the signed-in user's profile.
func (a *App) logActivity(cardID, action, field string) {
	if a.repo == nil {
		return
	}
	cardTitle := ""
	if card, err := a.repo.GetCard(cardID); err == nil {
		cardTitle = card.Title
	}
	breadcrumbs, _ := a.GetCardPinBreadcrumbs(cardID)
	a.logActivityWithContext(cardID, action, field, cardTitle, breadcrumbs)
}


// --- Move / Copy / Reorder (forwarders to core/services/project) ---

func (a *App) MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) error {
	return a.project.MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream)
}
func (a *App) MoveStream(fromBrand, streamSlug, toBrand string) error {
	return a.project.MoveStream(fromBrand, streamSlug, toBrand)
}
func (a *App) CopyBrand(brandSlug string) (*model.Brand, error) {
	return a.project.CopyBrand(brandSlug)
}
func (a *App) CopyStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	return a.project.CopyStream(fromBrand, streamSlug, toBrand)
}
func (a *App) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string, position int) (*model.Project, error) {
	return a.project.CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream, position)
}
func (a *App) ReorderBrands(orderedSlugs []string) error {
	return a.project.ReorderBrands(orderedSlugs)
}
func (a *App) ReorderStreams(brandSlug string, orderedSlugs []string) error {
	return a.project.ReorderStreams(brandSlug, orderedSlugs)
}
func (a *App) ReorderProjects(brandSlug, streamSlug string, orderedSlugs []string) error {
	return a.project.ReorderProjects(brandSlug, streamSlug, orderedSlugs)
}
func (a *App) ReorderCategories(brandSlug, streamSlug, projectSlug string, orderedSlugs []string) error {
	return a.project.ReorderCategories(brandSlug, streamSlug, projectSlug, orderedSlugs)
}

// --- Tag Colors ---

// --- Settings forwarders (core/services/settings) ---

func (a *App) GetPreferences() (config.Preferences, error) { return a.settings.GetPreferences() }
func (a *App) SetPreferences(p config.Preferences) error   { return a.settings.SetPreferences(p) }
func (a *App) GetAuthInfo() config.AuthInfo                { return a.settings.GetAuthInfo() }
func (a *App) GetProfile() (config.UserProfile, error)     { return a.settings.GetProfile() }
func (a *App) SetProfile(p config.UserProfile) error       { return a.settings.SetProfile(p) }

// ListAgentCardStates returns a map of cardID → enabled for every
// card with an agent configuration on disk. Cards that have never had
// an agent configured are absent from the map; cards that were
// configured then disabled appear with value false. Scans the cards
// directory rather than the index so the result is accurate even if
// the index is stale.
func (a *App) ListAgentCardStates() (map[string]bool, error) {
	states := map[string]bool{}
	if a.repo == nil {
		return states, nil
	}
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return states, nil
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil {
			continue
		}
		states[cardID] = af.Config.Enabled
	}
	return states, nil
}

// --- Notifications (forwarders to core/services/notify) ---

func (a *App) GetNotifyConfig() (config.NotifyConfig, error) {
	return a.notify.GetConfig()
}

func (a *App) SetNotifyConfig(c config.NotifyConfig) error {
	return a.notify.SetConfig(c)
}

// GetDueDateSettings returns the current due-date notification settings.
func (a *App) GetDueDateSettings() (map[string]interface{}, error) {
	prefs, err := config.LoadPreferences()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"enabled":    prefs.DueDateNotify,
		"thresholds": prefs.DueDateThresholds,
		"channels":   prefs.DueDateChannels,
	}, nil
}

// SaveDueDateSettings updates due-date notification settings and reconfigures the live scanner.
func (a *App) SaveDueDateSettings(enabled bool, thresholds []string, channels string) error {
	prefs, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	prefs.DueDateNotify = enabled
	prefs.DueDateThresholds = thresholds
	prefs.DueDateChannels = channels
	if err := config.SavePreferences(prefs); err != nil {
		return err
	}
	// Update live scanner
	if s := a.agentRT.DueDateScanner(); s != nil {
		s.Configure(enabled, thresholds, channels)
	}
	return nil
}

func (a *App) GetNotifications() ([]config.Notification, error) {
	return a.notify.List()
}

func (a *App) MarkNotificationRead(id string) error {
	err := a.notify.MarkRead(id)
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) MarkAllNotificationsRead() error {
	err := a.notify.MarkAllRead()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) ClearAllNotifications() error {
	err := a.notify.ClearAll()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

// --- Agent ---

// --- Agent config forwarders (core/services/agentsvc) ---

func (a *App) GetAgentConfig(cardID string) (*model.AgentFile, error) {
	return a.agentService.GetConfig(cardID)
}
func (a *App) SaveAgentConfig(cardID string, cfg model.AgentConfig) error {
	return a.agentService.SaveConfig(cardID, cfg)
}
func (a *App) ValidateSchedulePreview(schedule, startDate, endDate, timezone string, count int) ([]string, error) {
	return a.agentService.ValidateSchedulePreview(schedule, startDate, endDate, timezone, count)
}

func (a *App) GetAgentRuns(cardID string) ([]model.AgentRun, error) {
	return a.agentService.GetRuns(cardID)
}

// ClearAgentRuns removes all run history for a card's agent.
func (a *App) ClearAgentRuns(cardID string) error {
	return a.agentService.ClearRuns(cardID)
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
