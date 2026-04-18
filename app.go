package main

import (
	"bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/importer"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/logging"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/notify"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"bruv/internal/update"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppVersion is the user-facing release version. Overridable at build
// time via -ldflags "-X main.AppVersion=v1.0b" so releases don't require
// a source edit.
var AppVersion = "v1.0b-dev"

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
	// Agent scheduler
	scheduler      *agent.Scheduler
	dueDateScanner *agent.DueDateScanner
	agentCancels   sync.Map // cardID → context.CancelFunc
	forceQuit      bool
	trayPauseItem  interface{ Check(); Uncheck(); Checked() bool } // system tray "Pause Agents" menu item
	// mcpRegistry manages external MCP server subprocesses for the
	// currently open repo. Nil when no repo is open. Tools exposed by
	// MCP servers appear in the agent tool catalogue alongside
	// built-in tools and dispatch through the same executeAgentToolCall
	// path via a namespaced ID prefix. See internal/mcp for the full
	// architecture and docs/mcp-servers.md for user-facing docs.
	mcpRegistry *mcp.Registry
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

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
		a.stopScheduler()
		a.stopDueDateScanner()
		a.stopMCPRegistry()
		logging.Close()
		return false // allow quit
	}

	// Hide window instead of closing — agents keep running in background
	wailsRuntime.WindowHide(ctx)
	return true // prevent close
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
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "index:stale", op)
	}
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
// nudge only fires once per install. Intentionally writes via the
// existing Preferences store so it survives across restarts but resets
// when the user wipes their config directory (desired behaviour).
func (a *App) MarkLLMNudgeShown() error {
	p, err := config.LoadPreferences()
	if err != nil {
		return err
	}
	p.LLMNudgeShown = true
	return config.SavePreferences(p)
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

	// Add to recent repos
	_ = config.AddRecent(r.Root, name)

	// Heal tag colours in the background — no-op for a fresh repo, safe to run.
	go a.healTagColors()

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

	// Pre-v1.0b portability migration: seed the repo-scoped card types
	// store from the user's old global location (if any), and move any
	// in-repo chat files out to the config folder so they stop following
	// the repo around when it's shared. Idempotent — a no-op on repos
	// that have already been migrated or are already in the new layout.
	migrateStats := r.MigrateOnOpen(
		func() ([]byte, error) {
			// Read the old global card_types.json directly. We don't
			// go through config.LoadUserTypeStore because that would
			// return a decoded struct and we want to hand the raw JSON
			// to the migration so it can be dropped in verbatim.
			dir, dErr := config.ConfigDir()
			if dErr != nil {
				return nil, dErr
			}
			return os.ReadFile(filepath.Join(dir, "card_types.json"))
		},
		func(repoID, chatID string, cf *model.ChatFile) error {
			return config.SaveChatFor(repoID, cf)
		},
	)
	slog.Info("repo migrate", "stats", migrateStats.String())

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
	a.startScheduler()

	// Start due-date notification scanner
	a.startDueDateScanner()

	// Start MCP registry for this repo. Failures here don't block
	// opening the repo — servers that can't start are surfaced in
	// the Settings UI and their tools are simply absent from the
	// agent catalogue. A broken MCP config must never prevent
	// access to the card data.
	a.startMCPRegistry()

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
	a.stopScheduler()
	a.stopDueDateScanner()
	a.stopMCPRegistry()
	if a.idx != nil {
		a.idx.Close()
		a.idx = nil
	}
	a.repo = nil
}

// ListRecentRepos returns recently opened repositories.
func (a *App) ListRecentRepos() ([]config.RecentRepo, error) {
	return config.LoadRecent()
}

// RemoveRecentRepo removes a path from the recent repos list.
func (a *App) RemoveRecentRepo(path string) error {
	return config.RemoveRecent(path)
}

// --- Repository metadata ---

func (a *App) GetRepoDescription() (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("no repository open")
	}
	return a.repo.Manifest.Description, nil
}

func (a *App) UpdateRepoDescription(description string) error {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.UpdateManifestDescription(description)
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
		var actor, actorType string
		if v, ok := a.llmActors.Load(cardID); ok {
			actor = v.(string)
			actorType = "llm"
		} else {
			p, _ := config.LoadProfile()
			actor = p.DisplayName
			if actor == "" {
				actor = "User"
			}
			actorType = "user"
		}

		entry := model.ActivityEntry{
			ID:        uuid.New().String(),
			Timestamp: time.Now().UTC(),
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

func (a *App) CreateCard(cardType, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.CreateCard(cardType, title)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), ""))
	}
	a.logActivity(card.ID, model.ActivityCreated, "")
	// Apply template blocks if a type was set at creation
	if cardType != "" {
		a.applyTypeBlocks(card.ID, cardType)
		if updated, err := a.repo.GetCard(card.ID); err == nil {
			return updated, nil
		}
	}
	return card, nil
}

func (a *App) GetCard(id string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetCard(id)
}

func (a *App) ListCards() ([]model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListCards()
}

// DuplicateCard creates a copy of a card with a new ID and pins it to the given category.
func (a *App) DuplicateCard(cardID, categoryID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	newCard, err := a.repo.DuplicateCard(cardID)
	if err != nil {
		return nil, err
	}
	// Pin with categoryID for both projectID and categoryID (frontend convention)
	if err := a.repo.PinCard(newCard.ID, categoryID, categoryID); err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return newCard, nil
}

// CopyCategory duplicates a category and all its cards within the same project.
func (a *App) CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	// Get source category
	srcCats, err := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	var srcCat *model.Category
	for _, c := range srcCats {
		if c.Slug == categorySlug {
			cc := c
			srcCat = &cc
			break
		}
	}
	if srcCat == nil {
		return nil, fmt.Errorf("category %q not found", categorySlug)
	}

	// Create new category with " Copy" suffix
	newCat, err := a.repo.CreateCategory(brandSlug, streamSlug, projectSlug, srcCat.Name+" Copy", len(srcCats))
	if err != nil {
		return nil, err
	}

	// Duplicate all cards from source to new category
	if a.idx != nil {
		cardIDs, err := a.idx.ListCardIDsInCategory(srcCat.ID, srcCat.ID)
		if err == nil {
			for i, cardID := range cardIDs {
				newCard, err := a.repo.DuplicateCard(cardID)
				if err != nil {
					continue
				}
				_ = a.repo.PinCard(newCard.ID, newCat.ID, newCat.ID)
				_ = a.repo.MoveCardInCategory(newCard.ID, newCat.ID, newCat.ID, i)
			}
		}
		a.idxIncrementalRefresh()
	}

	return newCat, nil
}

func (a *App) DeleteCard(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Capture card context before deletion — both will fail once the card is gone.
	cardTitle := ""
	if card, err := a.repo.GetCard(id); err == nil {
		cardTitle = card.Title
	}
	breadcrumbs, _ := a.GetCardPinBreadcrumbs(id)
	if err := a.repo.DeleteCard(id); err != nil {
		return err
	}
	// Chat history lives in the config folder now — the repo layer
	// doesn't know about it, so we clean it up here alongside the card.
	_ = config.DeleteChatFor(a.repo.Manifest.ID, id)
	if a.idx != nil {
		a.logIdxErr("RemoveCard", a.idx.RemoveCard(id))
	}
	a.logActivityWithContext(id, model.ActivityDeleted, "", cardTitle, breadcrumbs)
	return nil
}

// UpdateCardTitle updates a card's title.
func (a *App) UpdateCardTitle(id, title string) (*model.Card, error) {
	title = repo.SanitizeText(title)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Title = title
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedTitle, "title")
	}
	return card, err
}

// UpdateCardType sets the type on a card (e.g. "task", "feature", or "" for none).
// For types that have a schema or template, the corresponding blocks are applied
// to the card (merging existing values by key).
func (a *App) UpdateCardType(id, cardType string) (*model.Card, error) {
	cardType = repo.SanitizeText(cardType)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Type = cardType
	})
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	a.logActivity(id, model.ActivityUpdatedType, cardType)
	if cardType != "" {
		a.applyTypeBlocks(id, cardType)
	}
	// Return the updated card with blocks applied
	updated, readErr := a.repo.GetCard(id)
	if readErr != nil {
		return card, nil
	}
	return updated, nil
}

// UpdateCardFields sets the type-specific fields on a card.
func (a *App) UpdateCardFields(id string, fields map[string]any) (*model.Card, error) {
	for k, v := range fields {
		if s, ok := v.(string); ok {
			fields[k] = repo.SanitizeText(s)
		}
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Fields = fields
	})
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// UpdateCardBlocks replaces a card's ordered content blocks.
// Also syncs legacy Fields/Checklist for backward compatibility.
func (a *App) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCardBlocks(id, blocks)
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedField, "content")
	}
	return card, err
}

// AddCardAttachment adds a file attachment to a card. data is base64-encoded.
func (a *App) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.AddCardAttachment(cardID, name, data)
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// RemoveCardAttachment removes a file attachment from a card.
func (a *App) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.RemoveCardAttachment(cardID, attachmentID)
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// UpdateCardTags replaces a card's tags.
func (a *App) UpdateCardTags(id string, tags []string) (*model.Card, error) {
	for i, t := range tags {
		tags[i] = repo.SanitizeText(t)
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Tags = tags
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.syncTagsToAllPinnedProjects(id)
		a.logActivity(id, model.ActivityUpdatedTags, "tags")
	}
	return card, err
}

// syncTagsToAllPinnedProjects ensures all tags on a card exist in every project it's pinned to.
func (a *App) syncTagsToAllPinnedProjects(cardID string) {
	card, err := a.repo.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return
	}
	// Sync to each unique project the card is pinned to
	seen := make(map[string]bool)
	for _, pin := range pins {
		if seen[pin.CategoryID] {
			continue
		}
		seen[pin.CategoryID] = true
		a.syncCardTagsToProject(cardID, pin.CategoryID)
	}
}

// UpdateCardDueDate sets or clears a card's due date (ISO 8601 string, or empty to clear).
func (a *App) UpdateCardDueDate(id, dueDate string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		if dueDate == "" {
			c.DueDate = nil
		} else {
			t, err := time.Parse(time.RFC3339, dueDate)
			if err != nil {
				t, _ = time.Parse("2006-01-02", dueDate)
			}
			c.DueDate = &t
		}
	})
	if err == nil {
		if a.idx != nil {
			a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
		}
		a.logActivity(id, model.ActivityUpdatedDate, "due date")
	}
	return card, err
}

// AddChecklistItem adds a checklist item to a card.
func (a *App) AddChecklistItem(cardID, text string) (*model.Card, error) {
	text = repo.SanitizeText(text)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AddChecklistItem(cardID, text)
}

// ToggleChecklistItem toggles a checklist item's done state.
func (a *App) ToggleChecklistItem(cardID, itemID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ToggleChecklistItem(cardID, itemID)
}

// RemoveChecklistItem removes a checklist item from a card.
func (a *App) RemoveChecklistItem(cardID, itemID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.RemoveChecklistItem(cardID, itemID)
}

// --- Pin ---

// getCategoryByID resolves a category UUID to its model and hierarchy slugs
// by scanning all brands > streams > projects > categories.
func (a *App) getCategoryByID(categoryID string) (*model.Category, string, string, string, error) {
	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == categoryID {
						return &c, b.Slug, s.Slug, p.Slug, nil
					}
				}
			}
		}
	}
	return nil, "", "", "", fmt.Errorf("category %q not found", categoryID)
}

// GetCategoryAcceptedTypes returns the accepted card types for a category by its ID.
// Returns nil (all types accepted) if the category has no restrictions.
func (a *App) GetCategoryAcceptedTypes(categoryID string) ([]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, _, _, _, err := a.getCategoryByID(categoryID)
	if err != nil {
		return nil, err
	}
	return cat.AcceptedTypes, nil
}

// validateCardTypeForCategory checks that a card's type is accepted by the target category.
// Returns nil if the type is accepted or the category has no restrictions.
func (a *App) validateCardTypeForCategory(cardID, categoryID string) error {
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return err
	}
	cat, _, _, _, err := a.getCategoryByID(categoryID)
	if err != nil {
		return err
	}
	if !repo.CategoryAcceptsType(cat, card.Type) {
		return fmt.Errorf("category %q does not accept card type %q", cat.Name, card.Type)
	}
	return nil
}

func (a *App) PinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.validateCardTypeForCategory(cardID, categoryID); err != nil {
		return err
	}
	if err := a.repo.PinCard(cardID, projectID, categoryID); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	// Sync card tags to the target project's tag definitions
	a.syncCardTagsToProject(cardID, categoryID)
	a.logActivity(cardID, model.ActivityPinned, "")
	return nil
}

// syncCardTagsToProject ensures all tags on a card exist in the target project's tag definitions.
func (a *App) syncCardTagsToProject(cardID, categoryID string) {
	card, err := a.repo.GetCard(cardID)
	if err != nil || len(card.Tags) == 0 {
		return
	}
	_, brandSlug, streamSlug, projectSlug, err := a.getCategoryByID(categoryID)
	if err != nil {
		return
	}
	labels, _ := a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	existing := make(map[string]bool, len(labels))
	for _, l := range labels {
		existing[strings.ToLower(l.Name)] = true
	}
	for _, tag := range card.Tags {
		if !existing[strings.ToLower(tag)] {
			// AddProjectLabel now syncs with tags.json automatically.
			a.repo.AddProjectLabel(brandSlug, streamSlug, projectSlug, tag, "")
		}
	}
}

// healTagColors runs in the background after a repository is opened.
// It walks every card and ensures each tag appears (with a colour) in every
// project the card is pinned to, and in the global tags.json. This lazily
// repairs cards created before the tag colour system was finalised.
func (a *App) healTagColors() {
	if a.repo == nil {
		return
	}

	cards, err := a.repo.ListCards()
	if err != nil || len(cards) == 0 {
		return
	}

	// Pre-build a categoryID → (brandSlug, streamSlug, projectSlug) lookup.
	// Uses the shared flat walker so this path pays only one hierarchy
	// traversal instead of recreating the nested walk that
	// ListAllCategories already performs.
	type hierKey struct{ brand, stream, project string }
	catToHier := make(map[string]hierKey)
	flat, _ := a.repo.ListAllCategoriesFlat()
	for _, f := range flat {
		catToHier[f.Category.ID] = hierKey{f.Brand.Slug, f.Stream.Slug, f.Project.Slug}
	}

	// For each card, ensure every tag is present (with colour) in every pinned project.
	for _, card := range cards {
		if len(card.Tags) == 0 {
			continue
		}
		pins, err := a.repo.GetCardPins(card.ID)
		if err != nil {
			continue
		}
		// Collect unique projects this card is pinned to.
		seen := make(map[string]bool)
		for _, pin := range pins {
			h, ok := catToHier[pin.CategoryID]
			if !ok {
				continue
			}
			key := h.brand + "/" + h.stream + "/" + h.project
			if seen[key] {
				continue
			}
			seen[key] = true

			labels, _ := a.repo.GetProjectLabels(h.brand, h.stream, h.project)
			existing := make(map[string]bool, len(labels))
			for _, l := range labels {
				existing[strings.ToLower(l.Name)] = true
			}
			for _, tag := range card.Tags {
				if !existing[strings.ToLower(tag)] {
					// AddProjectLabel syncs colour to tags.json.
					a.repo.AddProjectLabel(h.brand, h.stream, h.project, tag, "")
				}
			}
		}
	}
}

func (a *App) UnpinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Log before unpin so the path is still resolvable
	a.logActivity(cardID, model.ActivityUnpinned, "")
	if err := a.repo.UnpinCard(cardID, projectID, categoryID); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

func (a *App) GetCardPins(cardID string) ([]model.Pin, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetCardPins(cardID)
}

// CardLocation describes where a card lives in the brand/stream/project hierarchy.
type CardLocation struct {
	BrandSlug   string `json:"brandSlug"`
	StreamSlug  string `json:"streamSlug"`
	ProjectSlug string `json:"projectSlug"`
}

// GetCardLocation resolves a card's first pin to the brand/stream/project slugs
// so the frontend can navigate to the correct board before opening the card.
func (a *App) GetCardLocation(cardID string) (*CardLocation, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil || len(pins) == 0 {
		return nil, fmt.Errorf("card %q has no pins", cardID)
	}
	targetCatID := pins[0].CategoryID

	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					if c.ID == targetCatID {
						return &CardLocation{
							BrandSlug:   b.Slug,
							StreamSlug:  s.Slug,
							ProjectSlug: p.Slug,
						}, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for card %q", cardID)
}

// GetProjectLocation resolves a project UUID to its brand/stream/project slugs
// so the frontend can navigate to the correct board.
func (a *App) GetProjectLocation(projectID string) (*CardLocation, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				if p.ID == projectID {
					return &CardLocation{
						BrandSlug:   b.Slug,
						StreamSlug:  s.Slug,
						ProjectSlug: p.Slug,
					}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("could not resolve location for project %q", projectID)
}

// CategoryPath describes a category's full position in the Brand > Stream > Project > Category
// hierarchy. Used by the frontend PinPicker to display breadcrumb results.
type CategoryPath struct {
	BrandSlug           string `json:"brandSlug"`
	StreamSlug          string `json:"streamSlug"`
	ProjectSlug         string `json:"projectSlug"`
	CategorySlug        string `json:"categorySlug"`
	BrandName           string `json:"brandName"`
	StreamName          string `json:"streamName"`
	ProjectName         string `json:"projectName"`
	CategoryName        string `json:"categoryName"`
	BrandDescription    string `json:"brandDescription,omitempty"`
	StreamDescription   string `json:"streamDescription,omitempty"`
	ProjectDescription  string `json:"projectDescription,omitempty"`
	CategoryDescription string `json:"categoryDescription,omitempty"`
	ProjectID           string   `json:"projectId"`
	CategoryID          string   `json:"categoryId"`
	Breadcrumb          string   `json:"breadcrumb"`                 // e.g. "Mandela Daze / YouTube / Narratively Speaking / Episodes"
	AcceptedTypes       []string `json:"acceptedTypes,omitempty"`    // which card types this category accepts; nil/empty = all
	PinnedProjectID     string   `json:"pinnedProjectId,omitempty"` // actual stored pin.ProjectID — set only by GetCardPinBreadcrumbs, used for UnpinCard
}

// ListAllCategories returns every category across the entire hierarchy with full breadcrumb info.
// Used by PinPicker to populate the flat searchable list of pin targets.
//
// Delegates to repo.ListAllCategoriesFlat so every call site that
// needs "every category with parent chain" shares a single walk —
// previously healTagColors and this method duplicated the nested
// iteration, doubling the filesystem traffic on every startup.
func (a *App) ListAllCategories() ([]CategoryPath, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	flat, err := a.repo.ListAllCategoriesFlat()
	if err != nil {
		return nil, err
	}
	results := make([]CategoryPath, 0, len(flat))
	for _, f := range flat {
		results = append(results, CategoryPath{
			BrandSlug:           f.Brand.Slug,
			StreamSlug:          f.Stream.Slug,
			ProjectSlug:         f.Project.Slug,
			CategorySlug:        f.Category.Slug,
			BrandName:           f.Brand.Name,
			StreamName:          f.Stream.Name,
			ProjectName:         f.Project.Name,
			CategoryName:        f.Category.Name,
			BrandDescription:    f.Brand.Description,
			StreamDescription:   f.Stream.Description,
			ProjectDescription:  f.Project.Description,
			CategoryDescription: f.Category.Description,
			ProjectID:           f.Project.ID,
			CategoryID:          f.Category.ID,
			Breadcrumb:          f.Brand.Name + " / " + f.Stream.Name + " / " + f.Project.Name + " / " + f.Category.Name,
			AcceptedTypes:       f.Category.AcceptedTypes,
		})
	}
	return results, nil
}

// GetCardPinBreadcrumbs returns a CategoryPath for every pin the card has,
// enriched with full hierarchy names. Used by CardDetail to display the location indicator.
func (a *App) GetCardPinBreadcrumbs(cardID string) ([]CategoryPath, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	pins, err := a.repo.GetCardPins(cardID)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, nil
	}
	all, err := a.ListAllCategories()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]CategoryPath, len(all))
	for _, cp := range all {
		byID[cp.CategoryID] = cp
	}
	var result []CategoryPath
	for _, pin := range pins {
		if cp, ok := byID[pin.CategoryID]; ok {
			cp.PinnedProjectID = pin.ProjectID // carry actual stored value so frontend can unpin correctly
			result = append(result, cp)
		}
	}
	return result, nil
}

// MoveCardInCategory reorders a card within its current category.
func (a *App) MoveCardInCategory(cardID, projectID, categoryID string, newPosition int) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.repo.MoveCardInCategory(cardID, projectID, categoryID, newPosition); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

// MoveCardToCategory moves a card from one category to another.
func (a *App) MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID string, newPosition int) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if err := a.validateCardTypeForCategory(cardID, toCategoryID); err != nil {
		return err
	}
	if err := a.repo.MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID, newPosition); err != nil {
		return err
	}
	if a.idx != nil {
		pins, err := a.repo.GetCardPins(cardID)
		if err == nil {
			a.logIdxErr("IndexPins", a.idx.IndexPins(cardID, pins))
		}
	}
	return nil
}

// --- Move & Copy ---

// MoveProject moves a project from one stream to another.
func (a *App) MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	_, err := a.repo.MoveProject(fromBrand, fromStream, projectSlug, toBrand, toStream)
	return err
}

// MoveStream moves a stream from one brand to another.
func (a *App) MoveStream(fromBrand, streamSlug, toBrand string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	_, err := a.repo.MoveStream(fromBrand, streamSlug, toBrand)
	return err
}

// duplicateCardsForProject duplicates all cards in source categories and pins them
// to the corresponding destination categories. Uses the index to find cards.
// oldCatIDs and newCatIDs map category slug → category ID.
func (a *App) duplicateCardsForProject(oldCatIDs, newCatIDs map[string]string) {
	if a.idx == nil || a.repo == nil {
		return
	}
	for slug, oldCatID := range oldCatIDs {
		newCatID, ok := newCatIDs[slug]
		if !ok {
			continue
		}
		// Frontend convention: projectID == categoryID in pins
		cardIDs, err := a.idx.ListCardIDsInCategory(oldCatID, oldCatID)
		if err != nil || len(cardIDs) == 0 {
			continue
		}
		for i, cardID := range cardIDs {
			newCard, err := a.repo.DuplicateCard(cardID)
			if err != nil {
				continue
			}
			// Pin with categoryID for both projectID and categoryID, preserving position
			_ = a.repo.PinCardAt(newCard.ID, newCatID, newCatID, i)
		}
	}
}

// snapshotCatIDs returns a map of category slug → category ID for a project.
func (a *App) snapshotCatIDs(brand, stream, project string) map[string]string {
	cats, _ := a.repo.ListCategories(brand, stream, project)
	m := make(map[string]string, len(cats))
	for _, c := range cats {
		m[c.Slug] = c.ID
	}
	return m
}

// CopyBrand deep-copies a brand and all its contents, including cards.
func (a *App) CopyBrand(brandSlug string) (*model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Snapshot all source category IDs before copy
	type projCatSnapshot struct {
		streamSlug  string
		projectSlug string
		catIDs      map[string]string
	}
	var snapshots []projCatSnapshot
	srcStreams, _ := a.repo.ListStreams(brandSlug)
	for _, s := range srcStreams {
		projects, _ := a.repo.ListProjects(brandSlug, s.Slug)
		for _, p := range projects {
			snapshots = append(snapshots, projCatSnapshot{
				streamSlug:  s.Slug,
				projectSlug: p.Slug,
				catIDs:      a.snapshotCatIDs(brandSlug, s.Slug, p.Slug),
			})
		}
	}

	result, err := a.repo.CopyBrand(brandSlug)
	if err != nil {
		return nil, err
	}

	// Duplicate cards for each project using new category IDs
	for _, snap := range snapshots {
		newCatIDs := a.snapshotCatIDs(result.Slug, snap.streamSlug, snap.projectSlug)
		a.duplicateCardsForProject(snap.catIDs, newCatIDs)
	}

	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return result, nil
}

// CopyStream deep-copies a stream into the target brand, including cards.
func (a *App) CopyStream(fromBrand, streamSlug, toBrand string) (*model.Stream, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Snapshot source category IDs
	type projCatSnapshot struct {
		projectSlug string
		catIDs      map[string]string
	}
	var snapshots []projCatSnapshot
	srcProjects, _ := a.repo.ListProjects(fromBrand, streamSlug)
	for _, p := range srcProjects {
		snapshots = append(snapshots, projCatSnapshot{
			projectSlug: p.Slug,
			catIDs:      a.snapshotCatIDs(fromBrand, streamSlug, p.Slug),
		})
	}

	result, err := a.repo.CopyStream(fromBrand, streamSlug, toBrand)
	if err != nil {
		return nil, err
	}

	// Duplicate cards for each project
	for _, snap := range snapshots {
		newCatIDs := a.snapshotCatIDs(toBrand, result.Slug, snap.projectSlug)
		a.duplicateCardsForProject(snap.catIDs, newCatIDs)
	}

	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return result, nil
}

// CopyProject deep-copies a project into the target stream, including cards.
func (a *App) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string, position int) (*model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Snapshot source category IDs
	oldCatIDs := a.snapshotCatIDs(fromBrand, fromStream, projectSlug)

	result, err := a.repo.CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream, position)
	if err != nil {
		return nil, err
	}

	// Duplicate cards
	newCatIDs := a.snapshotCatIDs(toBrand, toStream, result.Slug)
	a.duplicateCardsForProject(oldCatIDs, newCatIDs)

	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return result, nil
}

// --- Reorder ---

// ReorderBrands updates brand positions based on the given ordered slug list.
func (a *App) ReorderBrands(orderedSlugs []string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.ReorderBrands(orderedSlugs)
}

// ReorderStreams updates stream positions within a brand based on the given ordered slug list.
func (a *App) ReorderStreams(brandSlug string, orderedSlugs []string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.ReorderStreams(brandSlug, orderedSlugs)
}

// ReorderProjects updates project positions within a stream based on the given ordered slug list.
func (a *App) ReorderProjects(brandSlug, streamSlug string, orderedSlugs []string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.ReorderProjects(brandSlug, streamSlug, orderedSlugs)
}

// ReorderCategories updates category positions based on the given ordered slug list.
func (a *App) ReorderCategories(brandSlug, streamSlug, projectSlug string, orderedSlugs []string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.ReorderCategories(brandSlug, streamSlug, projectSlug, orderedSlugs)
}

// --- Tag Colors ---

func (a *App) GetTagColors() (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetTagColors()
}

func (a *App) SetTagColor(tag, color string) (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.SetTagColor(tag, color)
}

func (a *App) AssignTagColor(tag string) (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AssignTagColor(tag)
}

// --- Labels (per-project) ---

func (a *App) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
}

func (a *App) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color)
}

func (a *App) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID)
}

func (a *App) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	if name != "" {
		name = repo.SanitizeText(name)
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color)
}

func (a *App) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon)
}

// UpdateCardLabels replaces a card's label IDs.
func (a *App) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Labels = labelIDs
	})
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}

// --- Schema ---

// CardTypeInfo is the rich card type metadata returned to the frontend.
type CardTypeInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description"`
	AIHint      string `json:"ai_hint,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
	Builtin     bool   `json:"builtin"`
}

// builtinTypes defines the built-in card types in display order.
var builtinTypes = []CardTypeInfo{
	{ID: "brainstorm", Label: "Brainstorm", Color: "#84cc16", Builtin: true},  // lime green
	{ID: "task", Label: "Task", Color: "#38bdf8", Builtin: true},              // light blue
	{ID: "reference", Label: "Reference", Color: "#fb923c", Builtin: true},    // orange
	{ID: "agent", Label: "Agent", Color: "#ef4444", Builtin: true},            // red
}

// seedTypes are pre-installed as user types on first run so they can be
// fully edited or deleted.
var seedTypes = []config.UserCardType{
	{ID: "feature", Label: "Feature", Color: "#6366f1"},
	{ID: "episode", Label: "Episode", Color: "#ec4899"},
}

// ensureSeeded adds the pre-installed seed types on first run.
func (a *App) ensureSeeded(store *config.UserTypeStore) bool {
	if store.Seeded {
		return false
	}
	store.Seeded = true
	for _, seed := range seedTypes {
		store.Types = append(store.Types, seed)
	}
	return true
}

// ensureStarterTemplates runs once (guarded by StarterTemplatesSeeded) to create
// starter templates from built-in schemas and link them to their card types.
// Runs for all users including those who were already seeded before this feature.
func (a *App) ensureStarterTemplates(store *config.UserTypeStore) bool {
	if store.StarterTemplatesSeeded || a.registry == nil {
		return false
	}
	store.StarterTemplatesSeeded = true

	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}

	// Create one starter template per schema.
	schemaTemplateIDs := make(map[string]string) // typeID → templateID
	for _, typeName := range a.registry.List() {
		blocks := a.registry.SchemaToBlocks(typeName)
		if len(blocks) == 0 {
			continue
		}
		name := typeName
		if s := a.registry.Get(typeName); s != nil && s.Name != "" {
			name = s.Name
		}
		tmpl := config.CardTemplate{
			ID:     uuid.New().String(),
			Name:   name,
			Blocks: blocks,
		}
		store.Templates = append(store.Templates, tmpl)
		schemaTemplateIDs[typeName] = tmpl.ID
	}

	// Link user types that have no template yet.
	for i, ut := range store.Types {
		if ut.TemplateID == "" {
			if tid, ok := schemaTemplateIDs[ut.ID]; ok {
				store.Types[i].TemplateID = tid
			}
		}
	}

	// Link built-in types that have no override template yet.
	for _, bt := range builtinTypes {
		if tid, ok := schemaTemplateIDs[bt.ID]; ok {
			ov := store.BuiltinOverrides[bt.ID]
			if ov.TemplateID == "" {
				ov.TemplateID = tid
				store.BuiltinOverrides[bt.ID] = ov
			}
		}
	}

	return true
}

// ensureMissingBuiltinTemplates creates templates for any built-in types
// that were added after the initial StarterTemplatesSeeded run.
func (a *App) ensureMissingBuiltinTemplates(store *config.UserTypeStore) bool {
	if a.registry == nil {
		return false
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	changed := false
	for _, bt := range builtinTypes {
		ov := store.BuiltinOverrides[bt.ID]
		if ov.TemplateID != "" {
			continue // already has a template
		}
		blocks := a.registry.SchemaToBlocks(bt.ID)
		if len(blocks) == 0 {
			continue
		}
		name := bt.Label
		if s := a.registry.Get(bt.ID); s != nil && s.Name != "" {
			name = s.Name
		}
		tmpl := config.CardTemplate{
			ID:     uuid.New().String(),
			Name:   name,
			Blocks: blocks,
		}
		store.Templates = append(store.Templates, tmpl)
		ov.TemplateID = tmpl.ID
		store.BuiltinOverrides[bt.ID] = ov
		changed = true
	}
	return changed
}

// ListCardTypes returns all card types (built-in first, then user-defined).
//
// Called early in the frontend boot sequence (from loadCardTypes() in
// App.svelte) — often BEFORE tryReopenLastRepo() has finished, so a.repo
// may be nil. In that case we fall back to returning only the built-ins
// so the UI has something to render; as soon as a repo opens the
// frontend re-fetches and picks up the user-defined types.
func (a *App) ListCardTypes() []CardTypeInfo {
	var store config.UserTypeStore
	if a.repo != nil {
		store, _ = a.repo.LoadUserTypeStore()
		dirty := a.ensureSeeded(&store)
		dirty = a.ensureStarterTemplates(&store) || dirty
		dirty = a.ensureMissingBuiltinTemplates(&store) || dirty
		if dirty {
			_ = a.repo.SaveUserTypeStore(store)
		}
	}

	result := make([]CardTypeInfo, 0, len(builtinTypes)+len(store.Types))
	for _, b := range builtinTypes {
		info := b
		if a.registry != nil {
			if s := a.registry.Get(b.ID); s != nil {
				info.Description = s.Description
			}
		}
		if ov, ok := store.BuiltinOverrides[b.ID]; ok {
			if ov.Color != "" {
				info.Color = ov.Color
			}
			if ov.TemplateID != "" {
				info.TemplateID = ov.TemplateID
			}
		}
		result = append(result, info)
	}
	for _, t := range store.Types {
		result = append(result, CardTypeInfo{
			ID:          t.ID,
			Label:       t.Label,
			Color:       t.Color,
			Icon:        t.Icon,
			Description: t.Description,
			AIHint:      t.AIHint,
			TemplateID:  t.TemplateID,
			Builtin:     false,
		})
	}
	return result
}

func (a *App) ValidateCardFields(cardType string, fields map[string]any) []string {
	if a.registry == nil {
		return []string{"schema registry not loaded"}
	}
	return a.registry.Validate(cardType, fields)
}

// applyTypeBlocks non-destructively merges a type's template blocks into a card.
// Existing blocks are NEVER removed or overwritten. Template blocks are only
// appended when no existing block shares the same key. Legacy field values
// (card.Fields) are carried forward into matching template blocks so content
// is never lost during a type change.
// For built-in types it uses the schema registry; for user types it uses the
// associated CardTemplate.
func (a *App) applyTypeBlocks(cardID, cardType string) {
	templateBlocks := a.resolveTemplateBlocks(cardType)
	if len(templateBlocks) == 0 {
		return
	}
	a.mergeTemplateBlocks(cardID, templateBlocks)
}

// resolveTemplateBlocks returns the template/schema blocks for a card type.
// Safe to call with a.repo nil — falls through to the built-in schema
// registry, which is always available.
func (a *App) resolveTemplateBlocks(cardType string) []model.Block {
	var store config.UserTypeStore
	if a.repo != nil {
		store, _ = a.repo.LoadUserTypeStore()
	}

	// Priority 1: user-defined type template
	for _, ut := range store.Types {
		if ut.ID == cardType && ut.TemplateID != "" {
			for _, tmpl := range store.Templates {
				if tmpl.ID == ut.TemplateID {
					return cloneBlocksWithFreshIDs(tmpl.Blocks)
				}
			}
			break
		}
	}

	// Priority 2: builtin override template
	if ov, ok := store.BuiltinOverrides[cardType]; ok && ov.TemplateID != "" {
		for _, tmpl := range store.Templates {
			if tmpl.ID == ov.TemplateID {
				return cloneBlocksWithFreshIDs(tmpl.Blocks)
			}
		}
	}

	// Priority 3: built-in schema
	if a.registry != nil {
		blocks := a.registry.SchemaToBlocks(cardType)
		if len(blocks) > 0 {
			return blocks
		}
	}

	return nil
}

// mergeTemplateBlocks merges template blocks into an existing card non-destructively.
// - Existing blocks are never removed or reordered.
// - If a template block's key matches an existing block, the existing value is kept.
// - Template blocks whose key matches an intrinsic field are skipped entirely.
// - Template blocks with keys not present in the card are appended.
func (a *App) mergeTemplateBlocks(cardID string, templateBlocks []model.Block) {
	existingCard, _ := a.repo.GetCard(cardID)
	if existingCard == nil {
		return
	}

	// Intrinsic fields are managed outside the block system (description, due_date,
	// labels, etc.). Template blocks with these keys must never be added as blocks.
	intrinsicKeys := map[string]bool{
		"description": true,
	}

	// Index existing blocks by key for lookup
	existingByKey := make(map[string]int) // key → index in Blocks slice
	for i, b := range existingCard.Blocks {
		if b.Key != "" {
			existingByKey[b.Key] = i
		}
	}

	// Start from the card's current blocks (preserving everything)
	merged := make([]model.Block, len(existingCard.Blocks))
	copy(merged, existingCard.Blocks)

	for _, tb := range templateBlocks {
		if tb.Key != "" && intrinsicKeys[tb.Key] {
			continue
		}
		if idx, exists := existingByKey[tb.Key]; exists {
			// Block already present — only fill in the value if the user hasn't set one
			if isBlockValueEmpty(merged[idx].Value) && !isBlockValueEmpty(tb.Value) {
				merged[idx].Value = tb.Value
			}
			continue
		}
		merged = append(merged, tb)
	}

	a.UpdateCardBlocks(cardID, merged)
}

// isBlockValueEmpty returns true if a block value is nil, empty string, empty slice, or zero.
func isBlockValueEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case []map[string]any:
		return len(val) == 0
	case float64:
		return val == 0
	case bool:
		return false // false is a valid user-set value
	}
	return false
}

// RefreshTypeBlocks re-merges the current card type's template blocks into the card.
// Missing template blocks are added (with empty values), existing blocks are untouched.
// This is the "refresh" action — safe to call any time.
func (a *App) RefreshTypeBlocks(cardID string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	if card.Type == "" {
		return card, nil
	}
	templateBlocks := a.resolveTemplateBlocks(card.Type)
	if len(templateBlocks) == 0 {
		return card, nil
	}
	a.mergeTemplateBlocks(cardID, templateBlocks)
	return a.repo.GetCard(cardID)
}

// cloneBlocksWithFreshIDs returns a copy of blocks with new UUIDs to avoid
// ID collisions when a template is applied to multiple cards.
func cloneBlocksWithFreshIDs(blocks []model.Block) []model.Block {
	cloned := make([]model.Block, len(blocks))
	for i, b := range blocks {
		cloned[i] = b
		cloned[i].ID = uuid.New().String()
	}
	return cloned
}

// --- User Card Types & Templates ---

// CreateUserCardType creates a new user-defined card type.
func (a *App) CreateUserCardType(label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	if label == "" {
		return config.UserCardType{}, fmt.Errorf("label is required")
	}
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	id := repo.Slugify(label)
	if id == "" {
		id = uuid.New().String()
	}
	// Ensure ID is unique; append suffix if needed
	base := id
	for i := 2; isTypeIDTaken(store, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	t := config.UserCardType{
		ID: id, Label: label, Color: color,
		Description: description, AIHint: aiHint, TemplateID: templateID,
	}
	store.Types = append(store.Types, t)
	return t, a.repo.SaveUserTypeStore(store)
}

// UpdateUserCardType updates an existing user-defined card type by ID.
func (a *App) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Label = label
			store.Types[i].Color = color
			store.Types[i].Description = description
			store.Types[i].AIHint = aiHint
			store.Types[i].TemplateID = templateID
			return store.Types[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// UpdateUserCardTypeIcon sets or clears the icon on a user-defined card type.
func (a *App) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	if a.repo == nil {
		return config.UserCardType{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Icon = icon
			return store.Types[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// DeleteUserCardType removes a user-defined card type by ID.
func (a *App) DeleteUserCardType(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types = append(store.Types[:i], store.Types[i+1:]...)
			return a.repo.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("card type %q not found", id)
}

// UpdateBuiltinCardType updates the color and/or template of a built-in card type.
func (a *App) UpdateBuiltinCardType(id, color, templateID string) error {
	isBuiltin := false
	for _, b := range builtinTypes {
		if b.ID == id {
			isBuiltin = true
			break
		}
	}
	if !isBuiltin {
		return fmt.Errorf("card type %q is not a built-in type", id)
	}
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	if store.BuiltinOverrides == nil {
		store.BuiltinOverrides = make(map[string]config.BuiltinOverride)
	}
	store.BuiltinOverrides[id] = config.BuiltinOverride{
		Color:      color,
		TemplateID: templateID,
	}
	return a.repo.SaveUserTypeStore(store)
}

// ListCardTemplates returns all user-defined card templates.
// Safe to call before a repo is open — returns an empty list so the
// frontend's early-boot loadCardTypes() call doesn't nil-panic.
func (a *App) ListCardTemplates() ([]config.CardTemplate, error) {
	if a.repo == nil {
		return []config.CardTemplate{}, nil
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return nil, err
	}
	if store.Templates == nil {
		return []config.CardTemplate{}, nil
	}
	return store.Templates, nil
}

// CreateCardTemplate creates a new card template.
func (a *App) CreateCardTemplate(name string, blocks []model.Block) (config.CardTemplate, error) {
	if name == "" {
		return config.CardTemplate{}, fmt.Errorf("name is required")
	}
	if a.repo == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	tmpl := config.CardTemplate{
		ID:     uuid.New().String(),
		Name:   name,
		Blocks: blocks,
	}
	store.Templates = append(store.Templates, tmpl)
	return tmpl, a.repo.SaveUserTypeStore(store)
}

// UpdateCardTemplate updates an existing card template by ID.
func (a *App) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	if a.repo == nil {
		return config.CardTemplate{}, fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates[i].Name = name
			store.Templates[i].Blocks = blocks
			return store.Templates[i], a.repo.SaveUserTypeStore(store)
		}
	}
	return config.CardTemplate{}, fmt.Errorf("template %q not found", id)
}

// DeleteCardTemplate removes a card template by ID.
func (a *App) DeleteCardTemplate(id string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates = append(store.Templates[:i], store.Templates[i+1:]...)
			return a.repo.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("template %q not found", id)
}

// --- Card type export / import ---
//
// The portable export format is a JSON file that mirrors UserTypeStore but
// also carries a format marker so we can evolve the schema safely. Import
// supports three merge modes so users can choose between a full replace
// (dangerous, confirm in UI), merging only non-colliding types, or merging
// and overwriting on ID collisions.

// CardTypesExport is the on-wire shape for exported card types. It's the
// same data as UserTypeStore minus the per-repo seeding flags, plus a
// format version for forward compatibility.
type CardTypesExport struct {
	Format           string                            `json:"format"`
	Version          int                               `json:"version"`
	Types            []config.UserCardType             `json:"types"`
	Templates        []config.CardTemplate             `json:"templates"`
	BuiltinOverrides map[string]config.BuiltinOverride `json:"builtin_overrides,omitempty"`
}

// CardTypesImportResult reports what an import actually did so the UI can
// show an accurate summary toast.
type CardTypesImportResult struct {
	TypesAdded         int `json:"types_added"`
	TypesOverwritten   int `json:"types_overwritten"`
	TypesSkipped       int `json:"types_skipped"`
	TemplatesAdded     int `json:"templates_added"`
	TemplatesOverwritten int `json:"templates_overwritten"`
	TemplatesSkipped   int `json:"templates_skipped"`
}

// ExportCardTypesToFile writes the current repo's card types + templates +
// built-in overrides to a portable JSON file the user can share.
func (a *App) ExportCardTypesToFile(filePath string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	store, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return fmt.Errorf("load card types: %w", err)
	}
	exp := CardTypesExport{
		Format:           "bruv-card-types",
		Version:          1,
		Types:            store.Types,
		Templates:        store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	return os.WriteFile(filePath, data, 0o644)
}

// ImportCardTypesFromFile reads a portable export JSON and merges it into
// the current repo's card types store. mode is one of:
//   - "replace"         — overwrite the current store entirely
//   - "merge"           — add non-colliding entries, skip collisions
//   - "merge_overwrite" — add everything, overwrite on ID collisions
func (a *App) ImportCardTypesFromFile(filePath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if a.repo == nil {
		return result, fmt.Errorf("no repository open")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return result, fmt.Errorf("read import file: %w", err)
	}
	var exp CardTypesExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return result, fmt.Errorf("parse import file: %w", err)
	}
	if exp.Format != "bruv-card-types" {
		return result, fmt.Errorf("not a BRUV card types export (format=%q)", exp.Format)
	}
	return a.applyCardTypesImport(exp, mode)
}

// ImportCardTypesFromRepo reads another BRUV repo's .bruv/card_types.json
// directly — no portable export file needed. The caller passes the path
// to the other repo's root folder. This is the "steal types from my
// other repo" workflow: no export step, no intermediate file.
func (a *App) ImportCardTypesFromRepo(otherRepoPath, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult
	if a.repo == nil {
		return result, fmt.Errorf("no repository open")
	}
	src := filepath.Join(otherRepoPath, ".bruv", "card_types.json")
	data, err := os.ReadFile(src)
	if err != nil {
		// Not a valid repo, or a legacy repo that predates the move —
		// surface a clear error so the UI can say so.
		if os.IsNotExist(err) {
			return result, fmt.Errorf("no card types found in %q (not a BRUV repo, or a legacy repo without repo-scoped types)", otherRepoPath)
		}
		return result, fmt.Errorf("read source repo types: %w", err)
	}
	var store config.UserTypeStore
	if err := json.Unmarshal(data, &store); err != nil {
		return result, fmt.Errorf("parse source repo types: %w", err)
	}
	exp := CardTypesExport{
		Format:           "bruv-card-types",
		Version:          1,
		Types:            store.Types,
		Templates:        store.Templates,
		BuiltinOverrides: store.BuiltinOverrides,
	}
	return a.applyCardTypesImport(exp, mode)
}

// applyCardTypesImport is shared between the file and repo import paths.
// It loads the current store, merges per the requested mode, saves, and
// returns a count of what happened.
func (a *App) applyCardTypesImport(exp CardTypesExport, mode string) (CardTypesImportResult, error) {
	var result CardTypesImportResult

	current, err := a.repo.LoadUserTypeStore()
	if err != nil {
		return result, fmt.Errorf("load current card types: %w", err)
	}

	switch mode {
	case "replace":
		// Count everything being added as new, since the previous state
		// is being discarded entirely.
		result.TypesAdded = len(exp.Types)
		result.TemplatesAdded = len(exp.Templates)
		current.Types = append([]config.UserCardType(nil), exp.Types...)
		current.Templates = append([]config.CardTemplate(nil), exp.Templates...)
		if exp.BuiltinOverrides != nil {
			copied := make(map[string]config.BuiltinOverride, len(exp.BuiltinOverrides))
			for k, v := range exp.BuiltinOverrides {
				copied[k] = v
			}
			current.BuiltinOverrides = copied
		}

	case "merge", "merge_overwrite":
		overwrite := mode == "merge_overwrite"

		typeIdx := make(map[string]int, len(current.Types))
		for i, t := range current.Types {
			typeIdx[t.ID] = i
		}
		for _, t := range exp.Types {
			if existing, ok := typeIdx[t.ID]; ok {
				if overwrite {
					current.Types[existing] = t
					result.TypesOverwritten++
				} else {
					result.TypesSkipped++
				}
				continue
			}
			current.Types = append(current.Types, t)
			result.TypesAdded++
		}

		tmplIdx := make(map[string]int, len(current.Templates))
		for i, tmpl := range current.Templates {
			tmplIdx[tmpl.ID] = i
		}
		for _, tmpl := range exp.Templates {
			if existing, ok := tmplIdx[tmpl.ID]; ok {
				if overwrite {
					current.Templates[existing] = tmpl
					result.TemplatesOverwritten++
				} else {
					result.TemplatesSkipped++
				}
				continue
			}
			current.Templates = append(current.Templates, tmpl)
			result.TemplatesAdded++
		}

		// Built-in overrides merge additively — overwrite-mode replaces
		// existing per-ID overrides, merge-mode leaves them alone.
		if exp.BuiltinOverrides != nil {
			if current.BuiltinOverrides == nil {
				current.BuiltinOverrides = make(map[string]config.BuiltinOverride)
			}
			for k, v := range exp.BuiltinOverrides {
				if _, exists := current.BuiltinOverrides[k]; exists && !overwrite {
					continue
				}
				current.BuiltinOverrides[k] = v
			}
		}

	default:
		return result, fmt.Errorf("unknown import mode %q (expected replace, merge, or merge_overwrite)", mode)
	}

	if err := a.repo.SaveUserTypeStore(current); err != nil {
		return result, fmt.Errorf("save merged card types: %w", err)
	}
	return result, nil
}

func isTypeIDTaken(store config.UserTypeStore, id string) bool {
	for _, t := range store.Types {
		if t.ID == id {
			return true
		}
	}
	// Also check against built-in IDs
	for _, b := range builtinTypes {
		if b.ID == id {
			return true
		}
	}
	return false
}

// humanizeBlockKey converts "recording_status" → "Recording Status".
func humanizeBlockKey(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// --- Index / Search ---

func (a *App) openIndex(repoPath string) error {
	if a.idx != nil {
		a.idx.Close()
	}
	dbPath := filepath.Join(repoPath, ".bruv", "index.db")
	idx, err := index.Open(dbPath)
	if err != nil {
		return err
	}
	a.idx = idx
	return nil
}

// GetCardProjectContext returns the stored project hierarchy path for a card (e.g. "Brand > Stream > Project").
func (a *App) GetCardProjectContext(cardID string) string {
	if a.idx == nil {
		return ""
	}
	return a.idx.GetCardProjectContext(cardID)
}

// SearchCards performs a full-text search across all indexed cards.
func (a *App) SearchCards(query string, limit int) ([]index.SearchResult, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.Search(query, limit)
}

// SearchOrphanedCards performs a full-text search limited to orphaned (inbox) cards.
func (a *App) SearchOrphanedCards(query string, limit int) ([]index.SearchResult, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.SearchOrphanedCards(query, limit)
}

// RebuildIndex drops and rebuilds the entire SQLite index from disk.
func (a *App) RebuildIndex() (*index.RebuildStats, error) {
	if a.idx == nil || a.repo == nil {
		return nil, fmt.Errorf("no repository or index open")
	}
	return a.idx.FullRebuild(a.repo.Root)
}

// RefreshIndex incrementally updates the index for changed/new/deleted cards.
func (a *App) RefreshIndex() (*index.RebuildStats, error) {
	if a.idx == nil || a.repo == nil {
		return nil, fmt.Errorf("no repository or index open")
	}
	return a.idx.IncrementalRefresh(a.repo.Root)
}

// ListCardIDsInCategory returns card IDs pinned to a project/category via the index.
func (a *App) ListCardIDsInCategory(projectID, categoryID string) ([]string, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.ListCardIDsInCategory(projectID, categoryID)
}

// ListOrphanedCardIDs returns IDs of cards that have no pins (Inbox cards).
func (a *App) ListOrphanedCardIDs() ([]string, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.ListOrphanedCardIDs()
}

// ListCardIDsByTag returns card IDs with a given tag via the index.
func (a *App) ListCardIDsByTag(tag string) ([]string, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.ListCardIDsByTag(tag)
}

// ListActivityLog returns the most-recent limit activity entries, newest first.
func (a *App) ListActivityLog(limit int) ([]model.ActivityEntry, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 50
	}
	return a.repo.ListActivity(limit)
}

// RecentCard is a card summary enriched with its first-pin path, used by the inbox.
type RecentCard struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	UpdatedAt    time.Time `json:"updated_at"`
	Tags         []string  `json:"tags"`
	DueDate      string    `json:"due_date,omitempty"`
	BrandSlug    string    `json:"brand_slug,omitempty"`
	StreamSlug   string    `json:"stream_slug,omitempty"`
	ProjectSlug  string    `json:"project_slug,omitempty"`
	BrandName    string    `json:"brand_name,omitempty"`
	StreamName   string    `json:"stream_name,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	CategoryName string    `json:"category_name,omitempty"`
	Breadcrumb   string    `json:"breadcrumb,omitempty"`
}

// ListRecentlyUpdatedCards returns up to limit cards sorted by UpdatedAt descending.
// Orphaned cards (no pins) are excluded so every result has a navigable path.
func (a *App) ListRecentlyUpdatedCards(limit int) ([]RecentCard, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 21
	}

	all, err := a.repo.ListCards()
	if err != nil {
		return nil, err
	}

	// Pre-build a breadcrumb lookup by categoryID for fast resolution
	allCats, _ := a.ListAllCategories()
	catByID := make(map[string]CategoryPath, len(allCats))
	for _, cp := range allCats {
		catByID[cp.CategoryID] = cp
	}

	// Sort all cards newest-first by UpdatedAt
	sorted := make([]model.Card, len(all))
	copy(sorted, all)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
	})

	result := make([]RecentCard, 0, limit)
	for _, card := range sorted {
		if len(result) >= limit {
			break
		}

		// Resolve pins; skip orphaned cards
		pins, err := a.repo.GetCardPins(card.ID)
		if err != nil || len(pins) == 0 {
			continue
		}

		rc := RecentCard{
			ID:        card.ID,
			Title:     card.Title,
			Type:      card.Type,
			UpdatedAt: card.UpdatedAt,
			Tags:      card.Tags,
		}
		if card.DueDate != nil {
			rc.DueDate = card.DueDate.Format("2006-01-02")
		}

		// Enrich with first-pin path
		if cp, ok := catByID[pins[0].CategoryID]; ok {
			rc.BrandSlug = cp.BrandSlug
			rc.StreamSlug = cp.StreamSlug
			rc.ProjectSlug = cp.ProjectSlug
			rc.BrandName = cp.BrandName
			rc.StreamName = cp.StreamName
			rc.ProjectName = cp.ProjectName
			rc.CategoryName = cp.CategoryName
			rc.Breadcrumb = cp.Breadcrumb
		}

		result = append(result, rc)
	}
	return result, nil
}

// --- User Preferences ---

func (a *App) GetPreferences() (config.Preferences, error) {
	return config.LoadPreferences()
}

func (a *App) SetPreferences(p config.Preferences) error {
	return config.SavePreferences(p)
}

// --- Auth Info ---

func (a *App) GetAuthInfo() config.AuthInfo {
	return config.GetLocalAuthInfo()
}

// --- User Profile ---

func (a *App) GetProfile() (config.UserProfile, error) {
	return config.LoadProfile()
}

func (a *App) SetProfile(p config.UserProfile) error {
	return config.SaveProfile(p)
}

// --- LLM Config ---

func (a *App) GetLLMConfig() (config.LLMConfig, error) {
	return config.LoadLLMConfig()
}

func (a *App) SetLLMConfig(c config.LLMConfig) error {
	return config.SaveLLMConfig(c)
}

// ListAgentCardIDs returns IDs of all cards that have agents enabled.
// Scans agent config files on disk rather than relying on the index,
// to ensure accuracy even if the index is stale.
func (a *App) ListAgentCardIDs() ([]string, error) {
	if a.repo == nil {
		return []string{}, nil
	}
	// Scan for .agent.json files in the cards directory
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return []string{}, nil
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err == nil && af.Config.Enabled {
			ids = append(ids, cardID)
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// --- Notifications ---

func (a *App) GetNotifyConfig() (config.NotifyConfig, error) {
	return config.LoadNotifyConfig()
}

func (a *App) SetNotifyConfig(c config.NotifyConfig) error {
	return config.SaveNotifyConfig(c)
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
	if a.dueDateScanner != nil {
		a.dueDateScanner.Configure(enabled, thresholds, channels)
	}
	return nil
}

func (a *App) GetNotifications() ([]config.Notification, error) {
	return config.LoadNotifications()
}

func (a *App) MarkNotificationRead(id string) error {
	err := config.MarkNotificationRead(id)
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) MarkAllNotificationsRead() error {
	err := config.MarkAllNotificationsRead()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

func (a *App) ClearAllNotifications() error {
	err := config.ClearAllNotifications()
	if err == nil {
		go a.refreshTrayTooltip()
	}
	return err
}

// --- Agent ---

func (a *App) GetAgentConfig(cardID string) (*model.AgentFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetAgentConfig(cardID)
}

func (a *App) SaveAgentConfig(cardID string, cfg model.AgentConfig) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Never accept 'running' status from the frontend — only the executor sets that
	if cfg.Status == model.AgentStatusRunning {
		cfg.Status = model.AgentStatusIdle
	}
	// Calculate NextRunAt when enabled with a schedule
	if cfg.Enabled && cfg.Schedule != "" {
		opts := agent.ScheduleOpts{
			StartDate:         cfg.StartDate,
			EndDate:           cfg.EndDate,
			ActiveWindowStart: cfg.ActiveWindowStart,
			ActiveWindowEnd:   cfg.ActiveWindowEnd,
			OneShot:           cfg.OneShot,
			LastRunAt:         cfg.LastRunAt,
			Timezone:          cfg.Timezone,
		}
		if next, err := agent.NextRunTimeWithOpts(cfg.Schedule, time.Now(), opts); err == nil {
			cfg.NextRunAt = &next
		}
		if cfg.Status == model.AgentStatusDisabled {
			cfg.Status = model.AgentStatusIdle
		}
	} else if !cfg.Enabled {
		cfg.Status = model.AgentStatusDisabled
		cfg.NextRunAt = nil
	}
	if err := a.repo.SaveAgentConfig(cardID, cfg); err != nil {
		return err
	}
	// Update index with agent state
	if a.idx != nil {
		nextRun := ""
		if cfg.NextRunAt != nil {
			nextRun = cfg.NextRunAt.Format(time.RFC3339)
		}
		a.logIdxErr("UpdateAgentIndex", a.idx.UpdateAgentIndex(cardID, cfg.Enabled, string(cfg.Status), nextRun))
	}
	return nil
}

// ValidateSchedulePreview returns the next N run times for a given schedule config.
func (a *App) ValidateSchedulePreview(schedule string, startDate string, endDate string, timezone string, count int) ([]string, error) {
	if schedule == "" {
		return nil, fmt.Errorf("empty schedule")
	}
	if count <= 0 || count > 10 {
		count = 5
	}

	var sd, ed *time.Time
	if startDate != "" {
		t, err := time.Parse(time.RFC3339, startDate)
		if err == nil {
			sd = &t
		}
	}
	if endDate != "" {
		t, err := time.Parse(time.RFC3339, endDate)
		if err == nil {
			ed = &t
		}
	}

	opts := agent.ScheduleOpts{
		StartDate: sd,
		EndDate:   ed,
		Timezone:  timezone,
	}

	var result []string
	from := time.Now()
	for i := 0; i < count; i++ {
		next, err := agent.NextRunTimeWithOpts(schedule, from, opts)
		if err != nil {
			break
		}
		result = append(result, next.Format(time.RFC3339))
		from = next.Add(time.Second) // advance past this run
	}
	return result, nil
}

func (a *App) GetAgentRuns(cardID string) ([]model.AgentRun, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetAgentRuns(cardID)
}

// ClearAgentRuns removes all run history for a card's agent.
func (a *App) ClearAgentRuns(cardID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.ClearAgentRuns(cardID)
}

// --- Chat ---

func (a *App) LoadChatHistory(cardID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return config.LoadChatFor(a.repo.Manifest.ID,cardID)
}

// projectChatID returns the virtual chat ID used to store project-level chat messages.
func projectChatID(projectID string) string {
	return "__project__" + projectID
}

// LoadProjectChatHistory returns the chat history for a project.
func (a *App) LoadProjectChatHistory(brandSlug, streamSlug, projectSlug string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	return config.LoadChatFor(a.repo.Manifest.ID,projectChatID(project.ID))
}

// ClearProjectChatHistory deletes all messages in a project's AI chat.
func (a *App) ClearProjectChatHistory(brandSlug, streamSlug, projectSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return err
	}
	chatID := projectChatID(project.ID)
	return config.SaveChatFor(a.repo.Manifest.ID,&model.ChatFile{CardID: chatID, Messages: []model.ChatMessage{}})
}

// ClearCardChatHistory deletes all messages in a card's AI chat.
func (a *App) ClearCardChatHistory(cardID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return config.SaveChatFor(a.repo.Manifest.ID,&model.ChatFile{CardID: cardID, Messages: []model.ChatMessage{}})
}

// SendProjectChatMessage sends a message in the project-level AI chat.
// Context is assembled from all cards pinned to the project, grouped by category.
// The LLM has tools to create cards, bulk-tag, move cards, and update cards.
// --- Chat loop infrastructure ---

// chatLoopConfig holds the per-call configuration for runChatLoop.
type chatLoopConfig struct {
	chatID       string
	systemPrompt string
	tools        []llm.ToolDef
	maxIter      int

	// allowDuplicateTool: tool names in this set bypass dedup (e.g. "create_card")
	allowDuplicateTool map[string]bool

	// executeTool runs a single tool call. Returns (result, action, pinSuggestion).
	// Project chat returns nil for pinSuggestion.
	executeTool func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion)

	// stageTool is non-nil only for card chat in suggest mode.
	stageTool func(tc llm.ToolCall) (string, []model.PendingEdit)

	// afterToolsRun is called after each iteration's tool calls (e.g. reload card, rebuild tools, nudge).
	// It receives the tool calls from the current iteration and the accumulated state,
	// and returns updated tools + optional system nudge messages to inject.
	afterToolsRun func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message)

	// suggest mode: if true, executeTool is ignored and stageTool is used instead.
	suggestMode bool

	// minConfidence filters pin suggestions below this threshold.
	minConfidence string

	// fallbackContent is the assistant message if max iterations are reached.
	fallbackContent string

	// tokenBudget is the maximum total tokens allowed across all iterations (0 = unlimited).
	tokenBudget int
	// totalTokensUsed is written back with the cumulative token count after the loop finishes.
	totalTokensUsed *int
}

// runChatLoop runs the shared LLM tool-calling loop for both card and project chat.
func (a *App) runChatLoop(ctx context.Context, provider llm.Provider, modelName string, cf *model.ChatFile, lc chatLoopConfig) (*model.ChatFile, error) {
	// Convert chat history to LLM messages
	var llmMessages []llm.Message
	for _, m := range cf.Messages {
		if m.Role == model.RoleUser || m.Role == model.RoleAssistant {
			llmMessages = append(llmMessages, llm.Message{Role: m.Role, Content: m.Content})
		}
	}

	var allToolActions []model.ToolAction
	var pinSuggestion *model.PinSuggestion
	var allPendingEdits []model.PendingEdit
	var cumulativeTokens int
	toolDefs := lc.tools

	for iteration := 0; iteration < lc.maxIter; iteration++ {
		resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
			SystemPrompt: lc.systemPrompt,
			Messages:     llmMessages,
			Model:        modelName,
			Tools:        toolDefs,
		})
		if err != nil {
			errMsg := model.ChatMessage{
				ID:        uuid.New().String(),
				Role:      model.RoleSystem,
				Content:   "Error: " + err.Error(),
				Timestamp: time.Now().UTC(),
			}
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, errMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
			}
			return cf, nil
		}

		// Accumulate token usage
		if resp.Usage != nil {
			cumulativeTokens += resp.Usage.TotalTokens
		}

		// Check token budget
		if lc.tokenBudget > 0 && cumulativeTokens > lc.tokenBudget {
			budgetMsg := model.ChatMessage{
				ID:        uuid.New().String(),
				Role:      model.RoleSystem,
				Content:   fmt.Sprintf("Token budget exceeded (%d / %d). Stopping.", cumulativeTokens, lc.tokenBudget),
				Timestamp: time.Now().UTC(),
			}
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, budgetMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
			}
			return cf, fmt.Errorf("token budget exceeded (%d / %d)", cumulativeTokens, lc.tokenBudget)
		}

		// No tool calls — final text response
		if len(resp.ToolCalls) == 0 {
			assistantMsg := model.ChatMessage{
				ID:            uuid.New().String(),
				Role:          model.RoleAssistant,
				Content:       resp.Content,
				Timestamp:     time.Now().UTC(),
				ToolActions:   allToolActions,
				PinSuggestion: pinSuggestion,
				PendingEdits:  allPendingEdits,
			}
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, assistantMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
			}
			return cf, nil
		}

		// Add assistant message with tool calls to conversation
		llmMessages = append(llmMessages, llm.Message{
			Role:      model.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Deduplicate tool calls within same response
		seenCalls := make(map[string]bool)
		for _, tc := range resp.ToolCalls {
			if seenCalls[tc.Name] && !lc.allowDuplicateTool[tc.Name] {
				llmMessages = append(llmMessages, llm.Message{
					Role:       "tool",
					Content:    "Skipped — duplicate call",
					ToolCallID: tc.ID,
				})
				continue
			}
			seenCalls[tc.Name] = true

			var result string
			if lc.suggestMode && lc.stageTool != nil {
				var edits []model.PendingEdit
				result, edits = lc.stageTool(tc)
				allPendingEdits = append(allPendingEdits, edits...)
			} else {
				var action *model.ToolAction
				var ps *model.PinSuggestion
				result, action, ps = lc.executeTool(tc)
				if action != nil {
					allToolActions = append(allToolActions, *action)
				}
				if ps != nil && config.ConfidenceMeetsThreshold(ps.Confidence, lc.minConfidence) {
					pinSuggestion = ps
				}
			}

			llmMessages = append(llmMessages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// Post-iteration hook: rebuild tools, inject nudge messages
		if lc.afterToolsRun != nil {
			var nudges []llm.Message
			toolDefs, nudges = lc.afterToolsRun(resp.ToolCalls, toolDefs, pinSuggestion, allPendingEdits)
			llmMessages = append(llmMessages, nudges...)
		}
	}

	// Max iterations reached — save what we have
	assistantMsg := model.ChatMessage{
		ID:            uuid.New().String(),
		Role:          model.RoleAssistant,
		Content:       lc.fallbackContent,
		Timestamp:     time.Now().UTC(),
		ToolActions:   allToolActions,
		PinSuggestion: pinSuggestion,
		PendingEdits:  allPendingEdits,
	}
	cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, assistantMsg)
	if lc.totalTokensUsed != nil {
		*lc.totalTokensUsed = cumulativeTokens
	}
	return cf, nil
}

// --- Project chat ---

func (a *App) SendProjectChatMessage(brandSlug, streamSlug, projectSlug, userMessage, contextLevel string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := projectChatID(project.ID)

	// Save user message
	cf, err := a.saveUserMessage(chatID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := a.loadLLMProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Build system prompt with project context
	categories, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	brand, _ := a.repo.GetBrand(brandSlug)
	stream, _ := a.repo.GetStream(brandSlug, streamSlug)
	systemPrompt := a.buildProjectSystemPrompt(brandSlug, streamSlug, projectSlug, brand, stream, project, categories, cfg, model.ProjectChatContextLevel(contextLevel))

	// Build tool definitions
	var catMaps []map[string]string
	for _, cat := range categories {
		catMaps = append(catMaps, map[string]string{"id": cat.ID, "breadcrumb": cat.Name})
	}
	cardTypes := a.listCardTypeIDs()
	toolDefs := llm.ProjectTools(cardTypes, catMaps)

	// Build the per-call scope for tool callbacks. Both staging and execute
	// callbacks use the cardIDs set to reject IDs the LLM hallucinated or
	// copied from a different project; the slugs let project-metadata tools
	// (update_project, *_label, *_category) target the right project.
	scope := projectChatScope{
		brandSlug:   brandSlug,
		streamSlug:  streamSlug,
		projectSlug: projectSlug,
		cardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			scope.cardIDs[p.CardID] = true
		}
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	suggestMode := cfg.AIMode == "suggest"
	return a.runChatLoop(ctx, provider, modelName, cf, chatLoopConfig{
		chatID:             chatID,
		systemPrompt:       systemPrompt,
		tools:              toolDefs,
		maxIter:            5,
		// Per-entity mutating tools must be callable multiple times in one
		// iteration so the LLM can act on several distinct targets in a single
		// turn (e.g. set an icon on every category, move several cards). The
		// bulk variant `update_cards` exists for the most common case but the
		// LLM doesn't always reach for it. Query/read tools are deliberately
		// NOT whitelisted — those should be deduped to stop runaway loops.
		allowDuplicateTool: map[string]bool{
			"create_card":          true,
			"update_card":          true,
			"move_card":            true,
			"add_tags_to_cards":    true,
			"configure_agent":      true,
			"create_category":      true,
			"update_category":      true,
			"delete_category":      true,
			"create_project_tag":   true,
			"update_project_tag":   true,
			"delete_project_tag":   true,
		},
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := a.executeProjectToolCall(tc, scope)
			return result, action, nil
		},
		stageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return a.stageProjectToolCall(tc, scope)
		},
		suggestMode:     suggestMode,
		fallbackContent: "I've made the requested changes to your project.",
	})
}

// buildProjectSystemPrompt builds the system prompt for project-level chat.
// Slugs are passed so the prompt can fetch project-scoped data (labels) and
// reference the project unambiguously to the LLM.
func (a *App) buildProjectSystemPrompt(brandSlug, streamSlug, projectSlug string, brand *model.Brand, stream *model.Stream, project *model.Project, categories []model.Category, cfg config.LLMConfig, level model.ProjectChatContextLevel) string {
	// Default to full context if empty/unrecognised.
	if level != model.ProjectChatContextMetadata && level != model.ProjectChatContextNone {
		level = model.ProjectChatContextAll
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are BRUV AI, a project assistant. Today is %s.\n\n", time.Now().Format("2006-01-02 (Monday)")))

	// Hierarchy context (always included — this is small and gives the LLM its bearings)
	if a.repo != nil && a.repo.Manifest.Description != "" {
		sb.WriteString(fmt.Sprintf("## Repository: %s\n%s\n\n", a.repo.Manifest.Name, a.repo.Manifest.Description))
	}
	if brand != nil {
		sb.WriteString(fmt.Sprintf("## Brand: %s\n", brand.Name))
		if brand.Description != "" {
			sb.WriteString(brand.Description + "\n")
		}
		if brand.SystemPrompt != "" {
			sb.WriteString("\nBrand instructions:\n" + brand.SystemPrompt + "\n")
		}
		sb.WriteString("\n")
	}
	if stream != nil {
		sb.WriteString(fmt.Sprintf("## Stream: %s\n", stream.Name))
		if stream.Description != "" {
			sb.WriteString(stream.Description + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("## Project: %s\n", project.Name))
	if project.Description != "" {
		sb.WriteString(project.Description + "\n")
	}
	if project.Icon != "" {
		sb.WriteString(fmt.Sprintf("Icon: `%s`\n", project.Icon))
	}
	sb.WriteString(fmt.Sprintf("project_id: `%s`\n", project.ID))
	sb.WriteString("\n")

	if cfg.Context != "" {
		sb.WriteString("## User context\n" + cfg.Context + "\n\n")
	}

	// Project tag vocabulary. Each line includes the tag's name, current
	// color, optional icon, ID, and a usage count computed by walking
	// ListCardIDsByTag — this is what lets the LLM answer "find unused tags"
	// without needing a dedicated tool. (The underlying Go type is named
	// `model.Label` for historical reasons; user-facing terminology is "tag".)
	if a.repo != nil {
		labels, _ := a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
		if len(labels) > 0 {
			sb.WriteString("## Project tags (the tag vocabulary)\n")
			sb.WriteString("These are the tags defined for this project. Each line shows usage count.\n")
			for _, l := range labels {
				count := 0
				if ids, err := a.ListCardIDsByTag(l.Name); err == nil {
					count = len(ids)
				}
				line := fmt.Sprintf("- `%s` (id: `%s`, color: %s", l.Name, l.ID, l.Color)
				if l.Icon != "" {
					line += ", icon: `" + l.Icon + "`"
				}
				line += fmt.Sprintf(", used by %d card", count)
				if count != 1 {
					line += "s"
				}
				if count == 0 {
					line += " — UNUSED"
				}
				line += ")"
				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Card enumeration is gated by the context level.
	switch level {
	case model.ProjectChatContextNone:
		sb.WriteString("_Card details are hidden for this conversation. Use tools to query cards if needed._\n\n")

	case model.ProjectChatContextMetadata:
		// Titles + types only — no tags, descriptions, or due dates.
		totalCards := 0
		seenCards := make(map[string]bool)
		if len(categories) > 0 {
			sb.WriteString("## Categories and cards (titles only)\n\n")
			for _, cat := range categories {
				sb.WriteString(renderCategoryHeader(cat))
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := a.repo.GetCard(pin.CardID)
					if err != nil {
						continue
					}
					totalCards++
					sb.WriteString(fmt.Sprintf("- **%s** (id: `%s`, type: %s)\n", card.Title, card.ID, card.Type))
				}
				sb.WriteString("\n")
			}
			if totalCards == 0 {
				sb.WriteString("All categories are empty — no cards yet.\n\n")
			}
		} else {
			sb.WriteString("This project has no categories yet.\n\n")
		}

	case model.ProjectChatContextAll:
		fallthrough
	default:
		// Full enumeration: titles, types, tags, due dates, content snippet, agent config.
		totalCards := 0
		seenCards := make(map[string]bool)
		if len(categories) > 0 {
			sb.WriteString("## Categories and cards\n\n")
			for _, cat := range categories {
				sb.WriteString(renderCategoryHeader(cat))
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := a.repo.GetCard(pin.CardID)
					if err != nil {
						continue
					}
					totalCards++
					sb.WriteString(serializeCardForProjectPrompt(a, card))
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("This project has no categories yet.\n\n")
		}
		if totalCards == 0 && len(categories) > 0 {
			sb.WriteString("All categories are empty — no cards yet.\n\n")
		}
	}

	sb.WriteString("## Your capabilities\n")
	sb.WriteString("You can read and modify any property of this project, its cards, its categories, and its tag vocabulary. ")
	sb.WriteString("USE THE TOOLS to make changes — do not just describe what would be done.\n\n")
	sb.WriteString("Cards:\n")
	sb.WriteString("- `create_card` — create a new card and optionally pin it to a category.\n")
	sb.WriteString("- `update_card` / `update_cards` — change title, type, tags, due date, description, blocks. Prefer the plural for bulk edits.\n")
	sb.WriteString("- For tags: use `tags_to_add` to append, `tags_to_remove` to drop specific tags, and `tags` only when the user explicitly wants to replace the whole tag list.\n")
	sb.WriteString("- `add_tags_to_cards` — bulk-tag many cards in one call.\n")
	sb.WriteString("- `move_card` — move a card between categories.\n")
	sb.WriteString("- `configure_agent` — set a card's agent schedule, goal, enabled state, or tool whitelist.\n\n")
	sb.WriteString("Web:\n")
	sb.WriteString("- `web_fetch` — fetch a URL and read its text. Use for known links.\n")
	sb.WriteString("- `web_search` — search the web via DuckDuckGo. Use for open-ended lookups; follow up with `web_fetch` on the best result if you need full content.\n")
	sb.WriteString("- YOU HAVE WEB ACCESS. If the user asks about current events, prices, news, or anything external to the project, CALL `web_search` — do NOT tell the user to look it up themselves. Cite source URLs in your reply.\n\n")
	sb.WriteString("Project metadata:\n")
	sb.WriteString("- `update_project` — change the project's name, description, or icon.\n\n")
	sb.WriteString("Project tags (the tag vocabulary, listed above):\n")
	sb.WriteString("- `create_project_tag` — define a new tag.\n")
	sb.WriteString("- `update_project_tag` — rename, recolor, or set an icon for an existing tag. Identify by `tag_id` or `tag_name`.\n")
	sb.WriteString("- `delete_project_tag` — remove a tag from the project's vocabulary. The tag list above shows usage counts; tags marked UNUSED can usually be deleted directly.\n\n")
	sb.WriteString("Categories (columns):\n")
	sb.WriteString("- `create_category` / `update_category` / `delete_category` — manage the columns. update_category can set name, description, icon, and accepted_types.\n")
	sb.WriteString("- `update_category` and `delete_category` accept either `category_id` (preferred) or `category_name`. Use the name when referring to a category you just created in the same conversation, since its ID won't be known yet.\n")
	sb.WriteString("- When chaining `create_category` with `move_card` or `create_card` in the same turn, use the destination's NAME (`to_category_name` / `category_name`) — the apply phase resolves the name after the create runs.\n")
	sb.WriteString("- `move_card` only requires `card_id` and the destination. The source (`from_category_id`) is auto-detected from the card's current pin in this project, so you usually don't need to supply it.\n\n")
	sb.WriteString("Icon names (for `icon` parameters on `update_project`, `update_category`, `create_project_tag`, `update_project_tag`):\n")
	sb.WriteString("Use ONLY these names — anything else will display as a placeholder. Names use kebab-case.\n")
	sb.WriteString(availableIconList())
	sb.WriteString("\n\n")
	sb.WriteString("When creating cards, always pin them to the most appropriate category.\n")
	return sb.String()
}

// availableIconList returns the icon name list embedded in the system prompt.
// MUST be kept in sync with `ICON_MAP` in `frontend/src/lib/icons.ts`. The
// frontend's DynamicIcon component renders an unknown-icon placeholder for
// any name not in that map, so picking one from outside this list silently
// breaks the display.
//
// When you add an icon to ICON_MAP, add it here too. (And vice versa.)
func availableIconList() string {
	icons := []string{
		// General
		"folder", "folder-open", "star", "heart", "zap", "rocket", "globe", "home", "flag", "bookmark",
		"tag", "tags", "lightbulb", "puzzle", "circle-dot", "crown", "trophy", "diamond", "gem", "sparkles",
		// People & reactions
		"user", "users", "hand-metal", "thumbs-up", "thumbs-down", "smile", "frown", "meh", "angry", "laugh",
		// Communication
		"bell", "mail", "message-square", "message-circle", "megaphone", "phone", "smartphone", "send", "inbox",
		// Media & entertainment
		"image", "video", "music", "music-2", "camera", "mic", "tv", "tv-2", "radio", "podcast", "headphones",
		"gamepad2", "film", "clapperboard", "popcorn", "drama", "newspaper",
		"disc", "monitor-play", "play-circle", "pause-circle", "stop-circle", "volume-2", "volume-x",
		// Files & writing
		"file-text", "file", "book", "book-open", "library", "pen", "pen-tool", "brush", "scissors", "palette",
		"edit", "copy", "save", "archive", "box", "package", "boxes",
		// Dev
		"code", "terminal", "database", "server", "cloud", "hash", "at-sign", "binary", "github", "gitlab", "layers",
		// Business / money
		"briefcase", "building", "shopping-cart", "dollar-sign", "credit-card", "bar-chart", "pie-chart",
		"award", "target", "crosshair", "circle-dollar-sign", "wallet", "piggy-bank", "banknote", "receipt",
		"calculator", "medal",
		// Time
		"calendar", "calendar-days", "calendar-clock", "calendar-check", "clock", "timer", "alarm-clock", "hourglass", "history",
		// Nature & weather
		"sun", "moon", "flame", "leaf", "tree-pine", "tree-deciduous", "mountain", "cloud-rain", "cloud-snow",
		"cloud-lightning", "snowflake", "wind", "rainbow", "sunrise", "sunset",
		// Animals
		"dog", "cat", "bird", "fish", "rabbit", "squirrel", "bug", "turtle",
		// Food & drink
		"coffee", "pizza", "utensils", "wine", "apple", "cookie", "ice-cream", "soup", "beer",
		// Transport
		"plane", "car", "bus", "train", "bike", "ship", "truck", "fuel",
		// Science & education
		"microscope", "atom", "flask-conical", "beaker", "dna", "brain", "brain-circuit", "graduation-cap", "school",
		// Health & fitness
		"activity", "heart-pulse", "stethoscope", "pill", "syringe", "bandage", "dumbbell",
		// Tools
		"wrench", "hammer", "drill", "pickaxe", "ruler", "hardhat", "plug",
		// Navigation
		"map", "map-pin", "compass", "navigation", "arrow-right", "arrow-up", "link", "external-link",
		// Security
		"lock", "unlock", "key", "shield", "shield-check", "shield-alert", "eye",
		// Devices
		"monitor", "laptop", "settings",
		// Status & alerts
		"alert-circle", "check-circle", "alert-triangle", "info", "help-circle", "badge-check", "badge-alert",
		// Lists & filter
		"list-checks", "list-todo", "list-filter", "filter",
		// Search
		"search", "zoom-in", "zoom-out",
		// Social
		"twitter", "youtube", "twitch", "linkedin", "facebook", "instagram", "slack",
		// Shapes
		"circle", "triangle", "octagon", "hexagon", "pentagon",
		// Editing
		"refresh-cw", "check", "plus", "minus",
		// Fun
		"gift", "party-popper", "percent",
	}
	return strings.Join(icons, ", ")
}

// renderCategoryHeader produces the markdown header line(s) for a category in
// the project chat system prompt. Includes name + ID, optional description,
// optional icon, and accepted_types restriction (if any).
func renderCategoryHeader(cat model.Category) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s (category_id: %s)\n", cat.Name, cat.ID))
	if cat.Icon != "" {
		sb.WriteString(fmt.Sprintf("Icon: `%s`\n", cat.Icon))
	}
	if cat.Description != "" {
		sb.WriteString(cat.Description + "\n")
	}
	if len(cat.AcceptedTypes) > 0 {
		sb.WriteString("Accepted card types: " + strings.Join(cat.AcceptedTypes, ", ") + "\n")
	}
	return sb.String()
}

// serializeCardForProjectPrompt renders one card into the markdown form used by
// the project chat system prompt. Includes title, id, type, tags, due date,
// description / block content snippets, and agent config when present.
//
// Total content per card is bounded so a project with hundreds of cards
// doesn't blow the context window:
//   - Description / block snippets are capped at ~500 chars combined.
//   - Agent goal is capped at ~200 chars.
//   - Each text/markdown block is included in order, separated by spaces.
func serializeCardForProjectPrompt(a *App, card *model.Card) string {
	const maxContentChars = 500
	const maxAgentGoalChars = 200

	var sb strings.Builder
	line := fmt.Sprintf("- **%s** (id: `%s`, type: %s", card.Title, card.ID, card.Type)
	if len(card.Tags) > 0 {
		line += ", tags: " + strings.Join(card.Tags, ", ")
	}
	if card.DueDate != nil {
		line += ", due: " + card.DueDate.Format("2006-01-02")
	}
	line += ")"
	sb.WriteString(line + "\n")

	// Aggregate text content from description field + every text/markdown block,
	// in document order, capped at maxContentChars overall.
	var content strings.Builder
	if desc, ok := card.Fields["description"].(string); ok && desc != "" {
		content.WriteString(desc)
	}
	for _, b := range card.Blocks {
		if b.Type != "text" && b.Type != "markdown" {
			continue
		}
		s, ok := b.Value.(string)
		if !ok || s == "" {
			continue
		}
		if content.Len() > 0 {
			content.WriteString(" \n")
		}
		content.WriteString(s)
		if content.Len() >= maxContentChars {
			break
		}
	}
	if content.Len() > 0 {
		snippet := content.String()
		if len(snippet) > maxContentChars {
			snippet = snippet[:maxContentChars] + "…"
		}
		sb.WriteString("  > " + strings.ReplaceAll(snippet, "\n", "\n  > ") + "\n")
	}

	// Agent config — only show if there's anything meaningful configured.
	if a.repo != nil {
		af, err := a.repo.GetAgentConfig(card.ID)
		if err == nil && af != nil {
			cfg := af.Config
			hasConfig := cfg.Enabled || cfg.Schedule != "" || cfg.Goal != "" || cfg.Status != "" || cfg.LastRunAt != nil || cfg.NextRunAt != nil
			if hasConfig {
				sb.WriteString("  agent:")
				sb.WriteString(fmt.Sprintf(" enabled=%t", cfg.Enabled))
				if cfg.Schedule != "" {
					sb.WriteString(", schedule=`" + cfg.Schedule + "`")
				}
				if cfg.Status != "" {
					sb.WriteString(", status=" + string(cfg.Status))
				}
				if cfg.LastRunAt != nil {
					sb.WriteString(", last_run=" + cfg.LastRunAt.Format("2006-01-02 15:04"))
				}
				if cfg.NextRunAt != nil {
					sb.WriteString(", next_run=" + cfg.NextRunAt.Format("2006-01-02 15:04"))
				}
				sb.WriteString("\n")
				if cfg.Goal != "" {
					goal := cfg.Goal
					if len(goal) > maxAgentGoalChars {
						goal = goal[:maxAgentGoalChars] + "…"
					}
					sb.WriteString("  agent goal: " + strings.ReplaceAll(goal, "\n", " ") + "\n")
				}
				// Surface the most recent failed run's error so the LLM can help debug.
				if len(af.Runs) > 0 {
					last := af.Runs[len(af.Runs)-1]
					if last.Status == "failed" && last.Error != "" {
						errMsg := last.Error
						if len(errMsg) > 200 {
							errMsg = errMsg[:200] + "…"
						}
						sb.WriteString("  agent last error: " + strings.ReplaceAll(errMsg, "\n", " ") + "\n")
					}
				}
			}
		}
	}
	return sb.String()
}

// --- Card chat ---

func (a *App) SendChatMessage(cardID, userMessage string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Save user message
	cf, err := a.saveUserMessage(cardID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := a.loadLLMProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Attribute all card edits in this chat turn to the LLM model
	a.llmActors.Store(cardID, cfg.Model)
	defer a.llmActors.Delete(cardID)

	// Load card for context
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return cf, nil
	}

	systemPrompt := a.buildSystemPrompt(card, cfg)

	// Build tool definitions
	cardTypes := a.listCardTypeIDs()
	allCats, _ := a.ListAllCategories()
	var catMaps []map[string]string
	for _, c := range allCats {
		catMaps = append(catMaps, map[string]string{"id": c.CategoryID, "breadcrumb": c.Breadcrumb})
	}

	buildToolDefs := func(c *model.Card) []llm.ToolDef {
		// Collect MCP tool IDs so configure_agent's allowed_tools enum
		// includes them — lets the LLM add MCP tools to agents via chat.
		var mcpToolIDs []string
		if a.mcpRegistry != nil {
			for _, t := range a.mcpRegistry.Tools() {
				mcpToolIDs = append(mcpToolIDs, t.NamespaceID)
			}
		}
		tools := llm.CardTools(cardTypes, catMaps, mcpToolIDs)
		if c != nil && len(c.Blocks) > 0 {
			fieldProps := make(map[string]any)
			for _, b := range c.Blocks {
				if b.Key == "" {
					continue
				}
				var prop map[string]any
				switch b.Type {
				case model.BlockChecklist:
					prop = map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of checklist item texts. Each string becomes an unchecked item.",
					}
				case model.BlockCheckbox:
					prop = map[string]any{"type": "boolean"}
				case model.BlockNumber:
					prop = map[string]any{"type": "number"}
				default:
					prop = map[string]any{"type": "string"}
				}
				if b.Meta != nil {
					if desc, ok := b.Meta["description"].(string); ok && desc != "" {
						prop["description"] = desc
					}
					if opts, ok := b.Meta["options"].([]string); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
					if opts, ok := b.Meta["options"].([]any); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
				}
				fieldProps[b.Key] = prop
			}
			if len(fieldProps) > 0 {
				for i, t := range tools {
					if t.Name == "set_fields" {
						tools[i] = llm.ToolDef{
							Name:        "set_fields",
							Description: "Fill in field values on the card. ALWAYS call this to write content into fields.",
							Parameters:  map[string]any{"type": "object", "properties": fieldProps},
						}
						break
					}
				}
			}
		}
		return tools
	}

	// Chat mode: the user has opted out of card-mutating tools, but
	// web research is still fair game (doesn't touch the card) — else
	// "can you look this up" is permanently broken in chat mode.
	// Edit/suggest modes get the full card-tool surface.
	var toolDefs []llm.ToolDef
	if cfg.AIMode == "chat" {
		toolDefs = llm.WebTools()
	} else {
		toolDefs = buildToolDefs(card)
	}
	slog.Info("card chat tools assembled",
		"cardID", cardID, "ai_mode", cfg.AIMode, "tool_count", len(toolDefs))

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	return a.runChatLoop(ctx, provider, modelName, cf, chatLoopConfig{
		chatID:       cardID,
		systemPrompt: systemPrompt,
		tools:        toolDefs,
		// 6 iterations comfortably covers web_search → web_fetch →
		// summarise, or a couple of card-tool rounds plus a final
		// message. Previously 3, which was too tight for research
		// flows — the loop would exhaust before the model got to
		// speak, triggering the fallbackContent lie below.
		maxIter: 6,
		suggestMode:  cfg.AIMode == "suggest",
		stageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return a.stageToolCall(tc, allCats)
		},
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			return a.executeToolCall(cardID, card, tc, allCats, cfg.AutoPin)
		},
		minConfidence: cfg.MinConfidence,
		afterToolsRun: func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message) {
			var nudges []llm.Message

			// Reload card after tool execution (it may have changed)
			if cfg.AIMode != "suggest" {
				card, _ = a.repo.GetCard(cardID)
				tools = buildToolDefs(card)
			}

			// Nudge to fill empty fields
			calledSetFields := false
			for _, tc := range calls {
				if tc.Name == "set_fields" || tc.Name == "update_blocks" {
					calledSetFields = true
					break
				}
			}
			if !calledSetFields && card != nil && len(card.Blocks) > 0 {
				var emptyKeys []string
				for _, b := range card.Blocks {
					if b.Key != "" {
						if v, ok := b.Value.(string); ok && v == "" {
							emptyKeys = append(emptyKeys, b.Key)
						}
					}
				}
				if len(emptyKeys) > 0 {
					nudges = append(nudges, llm.Message{
						Role:    model.RoleUser,
						Content: fmt.Sprintf("[System: The card has empty fields that need content: %s. Use set_fields to fill them based on the conversation.]", strings.Join(emptyKeys, ", ")),
					})
				}
			}

			// Nudge for pin
			calledPin := false
			for _, tc := range calls {
				if tc.Name == "suggest_pin" {
					calledPin = true
					break
				}
			}
			suggestPinStaged := false
			for _, e := range edits {
				if e.Tool == "suggest_pin" {
					suggestPinStaged = true
					break
				}
			}
			if !calledPin && pin == nil && !suggestPinStaged {
				existingPins, _ := a.repo.GetCardPins(cardID)
				if len(existingPins) == 0 {
					nudges = append(nudges, llm.Message{
						Role:    model.RoleUser,
						Content: "[System: This card has no pin location yet. Use suggest_pin to pin it to the best-fit category.]",
					})
				}
			}

			return tools, nudges
		},
		// Fallback is only reached when the model keeps calling tools
		// without ever producing a final text reply. Previously this
		// said "I've made the changes to your card." — actively
		// misleading when the model was researching, not editing.
		// Honest + generic: the user can see the tool actions that
		// fired above this message and ask a follow-up if needed.
		fallbackContent: "I hit my tool-call limit before I could write a reply. The tools above show what ran — ask again or narrow the request if you'd like a summary.",
	})
}

// --- Chat helpers ---

// saveUserMessage saves a user message to a chat and returns the updated ChatFile.
func (a *App) saveUserMessage(chatID, userMessage string) (*model.ChatFile, error) {
	userMsg := model.ChatMessage{
		ID:        uuid.New().String(),
		Role:      model.RoleUser,
		Content:   repo.SanitizeText(userMessage),
		Timestamp: time.Now().UTC(),
	}
	return config.AppendChatMessage(a.repo.Manifest.ID,chatID, userMsg)
}

// loadLLMProvider loads config and creates a provider. Returns (cfg, provider, err).
// If LLM is not configured, provider is nil and err is nil.
func (a *App) loadLLMProvider() (config.LLMConfig, llm.Provider, error) {
	return a.loadLLMProviderForAccount("", "")
}

// loadLLMProviderForAccount resolves the LLM provider from:
// 1. Specific account (if accountID is set)
// 2. Default account from llm_accounts.json
// 3. Legacy fields in llm_config.json (backward compat)
func (a *App) loadLLMProviderForAccount(accountID, modelOverride string) (config.LLMConfig, llm.Provider, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return cfg, nil, nil
	}

	// Try accounts-based resolution
	accounts, _ := config.LoadLLMAccounts()
	var acct *config.LLMAccount

	if accountID != "" {
		acct = config.FindAccountByID(accounts, accountID)
	}
	if acct == nil && cfg.DefaultAccountID != "" {
		acct = config.FindAccountByID(accounts, cfg.DefaultAccountID)
	}
	if acct == nil {
		acct = config.GetDefaultAccount(accounts)
	}

	if acct != nil {
		// Use account credentials
		provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
		if err != nil {
			return cfg, nil, nil
		}
		// Determine model: override > account default > provider default
		model := modelOverride
		if model == "" {
			model = acct.Model
		}
		if model == "" {
			model = defaultModelForProvider(acct.Provider)
		}
		cfg.Model = model
		cfg.Provider = acct.Provider
		return cfg, provider, nil
	}

	// Legacy fallback: use fields directly from llm_config.json
	if cfg.Provider == "" {
		return cfg, nil, nil
	}
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return cfg, nil, nil
	}
	return cfg, provider, nil
}

// GetLLMAccounts returns all configured AI accounts.
func (a *App) GetLLMAccounts() ([]config.LLMAccount, error) {
	return config.LoadLLMAccounts()
}

// SaveLLMAccounts persists the AI accounts list.
func (a *App) SaveLLMAccounts(accounts []config.LLMAccount) error {
	return config.SaveLLMAccounts(accounts)
}

// TestLLMAccountConnection tests connectivity for a specific account by ID.
func (a *App) TestLLMAccountConnection(accountID string) (string, error) {
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return "", err
	}
	acct := config.FindAccountByID(accounts, accountID)
	if acct == nil {
		return "", fmt.Errorf("account not found")
	}
	provider, err := llm.NewProvider(acct.Provider, acct.APIKey, acct.BaseURL)
	if err != nil {
		return "", err
	}
	modelName := acct.Model
	if modelName == "" {
		modelName = defaultModelForProvider(acct.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a test. Reply with exactly: OK",
		Messages:     []llm.Message{{Role: "user", Content: "Hello"}},
		Model:        modelName,
	})
	if err != nil {
		return "", err
	}
	return resp.Model, nil
}

// listCardTypeIDs returns all registered card type IDs.
func (a *App) listCardTypeIDs() []string {
	if a.registry != nil {
		return a.registry.List()
	}
	return nil
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
func coerceBlockValue(blockType string, val any) any {
	switch blockType {
	case model.BlockChecklist:
		return coerceChecklist(val)
	case model.BlockList:
		return coerceList(val)
	case model.BlockCheckbox:
		return coerceCheckbox(val)
	case model.BlockNumber:
		return coerceNumber(val)
	default:
		// text, select, radio, date, url, image, video — all string, pass through
		return val
	}
}

// coerceBlockValueForBlock is the meta-aware variant: given a full block,
// apply the same type coercion AND additional constraints that need the
// block's Meta (select/radio allowed options, rating max). Used when the
// caller has the whole block in hand, such as update_self targeting an
// existing block on the card.
//
// Returns the coerced value and optionally an error describing a
// constraint violation. On a constraint violation we return the coerced
// value anyway (best effort — a bad select value is rendered as plain
// text, not corruption) so callers can still write it if they choose.
func coerceBlockValueForBlock(b *model.Block, val any) (any, error) {
	coerced := coerceBlockValue(b.Type, val)

	switch b.Type {
	case model.BlockDate:
		// Normalise whatever date/timestamp the LLM sent into a shape
		// the frontend input can render. LLMs produce all sorts of
		// inputs — "2026-04-12", "2026-04-12T01:45:09+07:00", "April 12
		// 2026", "now" — and passing any of those through verbatim to
		// an <input type="date"> leaves the field empty.
		s, ok := coerced.(string)
		if !ok || s == "" {
			return coerced, nil
		}
		format := ""
		if b.Meta != nil {
			format, _ = b.Meta["format"].(string)
		}
		normalised, err := normaliseDateValue(s, format)
		if err != nil {
			return coerced, fmt.Errorf("date block: could not parse %q: %v", s, err)
		}
		return normalised, nil

	case model.BlockSelect, model.BlockRadio:
		// If the block has an options list, the value must be one of
		// them. LLMs frequently invent options that aren't configured.
		s, ok := coerced.(string)
		if !ok {
			return coerced, nil
		}
		opts := extractBlockOptions(b.Meta)
		if len(opts) == 0 {
			return coerced, nil // no constraint configured
		}
		for _, o := range opts {
			if o == s {
				return coerced, nil
			}
		}
		return coerced, fmt.Errorf("value %q is not in the allowed options %v", s, opts)

	case model.BlockRating:
		// Clamp to [0, max] where max defaults to 5. Also coerces
		// string → float64 via the existing number path.
		n, ok := coerced.(float64)
		if !ok {
			// coerceBlockValue only converts BlockNumber; rating goes
			// through the passthrough branch. Try harder here.
			if parsed := coerceNumber(val); parsed != 0 || val == float64(0) || val == "0" {
				n = parsed
				ok = true
			}
		}
		if !ok {
			return coerced, nil
		}
		maxRating := 5.0
		if b.Meta != nil {
			if m, ok := b.Meta["max"].(float64); ok && m > 0 {
				maxRating = m
			} else if m, ok := b.Meta["max"].(int); ok && m > 0 {
				maxRating = float64(m)
			}
		}
		if n < 0 {
			n = 0
		}
		if n > maxRating {
			n = maxRating
		}
		return n, nil

	case model.BlockProgress:
		// Progress is conceptually 0–100. Same clamping treatment.
		n, ok := coerced.(float64)
		if !ok {
			if parsed := coerceNumber(val); parsed != 0 || val == float64(0) || val == "0" {
				n = parsed
				ok = true
			}
		}
		if !ok {
			return coerced, nil
		}
		if n < 0 {
			n = 0
		}
		if n > 100 {
			n = 100
		}
		return n, nil
	}

	return coerced, nil
}

// normaliseDateValue takes an LLM-supplied date/timestamp string and
// returns it in a form the frontend DateBlock can parse:
//
//   - format == "date-time": full ISO 8601 with timezone (RFC 3339)
//   - format == "" or "date": YYYY-MM-DD only
//
// Accepts a wide range of inputs — full RFC 3339, just a date,
// Go's RFC3339Nano, or a Unix-ish "2006-01-02 15:04:05" — and fails
// loudly for anything it can't parse so the caller can surface a
// useful error to the LLM.
func normaliseDateValue(raw, format string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	// Try the common formats in order of specificity. time.Parse returns
	// on the first match.
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	var parsed time.Time
	var parseErr error
	for _, layout := range layouts {
		parsed, parseErr = time.Parse(layout, raw)
		if parseErr == nil {
			break
		}
	}
	if parseErr != nil {
		return "", parseErr
	}

	if format == "date-time" {
		return parsed.Format(time.RFC3339), nil
	}
	return parsed.Format("2006-01-02"), nil
}

// extractBlockOptions pulls the string options list out of a block's
// Meta map. Options are stored as []any of strings; this flattens that
// into a plain []string for easy comparison.
func extractBlockOptions(meta map[string]any) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta["options"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, o := range raw {
		if s, ok := o.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// coerceChecklist converts []any of strings, []any of {text, done} maps,
// or a single newline-separated string into [{id, text, done}].
func coerceChecklist(val any) []map[string]any {
	type entry struct {
		text string
		done bool
	}
	var entries []entry
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					entries = append(entries, entry{text: it})
				}
			case map[string]any:
				text, _ := it["text"].(string)
				if text == "" {
					continue
				}
				done, _ := it["done"].(bool)
				entries = append(entries, entry{text: text, done: done})
			}
		}
	case string:
		for _, line := range strings.Split(v, "\n") {
			line = stripListPrefix(line)
			if line != "" {
				entries = append(entries, entry{text: line})
			}
		}
	}
	items := make([]map[string]any, len(entries))
	for i, e := range entries {
		items[i] = map[string]any{
			"id":   fmt.Sprintf("cli-%s", uuid.New().String()[:8]),
			"text": e.text,
			"done": e.done,
		}
	}
	return items
}

// coerceList converts []any of strings, []any of {id?, text} maps, or a
// single newline-separated string into [{id, text}]. This is the fix for
// the bug where agent `update_self` was writing plain strings directly to
// list blocks, which the frontend renderer couldn't parse.
func coerceList(val any) []map[string]any {
	var texts []string
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if it != "" {
					texts = append(texts, it)
				}
			case map[string]any:
				if text, ok := it["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
	case string:
		// Fallback for LLMs that send a formatted string instead of an
		// array. Newline-split and strip markdown bullets so the result
		// is a clean list.
		for _, line := range strings.Split(v, "\n") {
			line = stripListPrefix(line)
			if line != "" {
				texts = append(texts, line)
			}
		}
	}
	items := make([]map[string]any, len(texts))
	for i, t := range texts {
		items[i] = map[string]any{
			"id":   fmt.Sprintf("li-%s", uuid.New().String()[:8]),
			"text": t,
		}
	}
	return items
}

// stripListPrefix normalises a single line by trimming whitespace and
// removing leading markdown list markers like "- ", "* ", "• ", or
// "1. " / "2) ". Shared by coerceList and coerceChecklist.
func stripListPrefix(line string) string {
	line = strings.TrimSpace(line)
	// Numbered prefix: "1. " or "12) "
	if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
		for i := 0; i < len(line); i++ {
			c := line[i]
			if c >= '0' && c <= '9' {
				continue
			}
			if (c == '.' || c == ')') && i+1 < len(line) && line[i+1] == ' ' {
				line = strings.TrimSpace(line[i+2:])
			}
			break
		}
	}
	line = strings.TrimPrefix(line, "- ")
	line = strings.TrimPrefix(line, "* ")
	line = strings.TrimPrefix(line, "• ")
	return strings.TrimSpace(line)
}

// coerceCheckbox converts string representations ("true", "yes", "1") to bool.
func coerceCheckbox(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "true" || v == "yes" || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

// coerceNumber converts string representations to float64.
func coerceNumber(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// executeToolCall runs a single tool and returns (result string, action record, pin suggestion).
func (a *App) executeToolCall(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath, autoPinMode string) (string, *model.ToolAction, *model.PinSuggestion) {
	switch tc.Name {
	case "set_title":
		title, _ := tc.Arguments["title"].(string)
		if title == "" {
			return "error: title is required", nil, nil
		}
		if card != nil && card.Title == title {
			return "Title is already " + title + " — no change needed", nil, nil
		}
		_, err := a.UpdateCardTitle(cardID, title)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}
		action := &model.ToolAction{Tool: "set_title", Input: tc.Arguments, Result: "Title set to " + title}
		return "Card title set to " + title, action, nil

	case "set_due_date":
		dueDate, _ := tc.Arguments["due_date"].(string)
		if dueDate != "" && card != nil && card.DueDate != nil && card.DueDate.Format("2006-01-02") == dueDate {
			return "Due date is already " + dueDate + " — no change needed", nil, nil
		}
		_, err := a.UpdateCardDueDate(cardID, dueDate)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}
		result := "Due date cleared"
		if dueDate != "" {
			result = "Due date set to " + dueDate
		}
		action := &model.ToolAction{Tool: "set_due_date", Input: tc.Arguments, Result: result}
		return result, action, nil

	case "set_card_type":
		cardType, _ := tc.Arguments["card_type"].(string)
		if cardType == "" {
			return "error: card_type is required", nil, nil
		}
		if card != nil && card.Type == cardType {
			return "Type is already " + cardType + " — no change needed. Use update_blocks to fill in block values.", nil, nil
		}
		_, err := a.UpdateCardType(cardID, cardType)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}
		// Block application is handled by UpdateCardType via applyTypeBlocks
		// Build a helpful result listing available block keys
		resultMsg := "Card type set to " + cardType + ". "
		updatedCard, _ := a.repo.GetCard(cardID)
		if updatedCard != nil && len(updatedCard.Blocks) > 0 {
			var blockKeys []string
			for _, b := range updatedCard.Blocks {
				if b.Key != "" {
					blockKeys = append(blockKeys, b.Key)
				}
			}
			if len(blockKeys) > 0 {
				resultMsg += "NOW call set_fields to fill these field keys: " + strings.Join(blockKeys, ", ")
			}
		}
		action := &model.ToolAction{Tool: "set_card_type", Input: tc.Arguments, Result: "Set type to " + cardType}
		return resultMsg, action, nil

	case "set_fields", "update_blocks":
		// Accept nested "fields"/"blocks" key OR flat top-level arguments
		// (dynamic tool schema puts block keys at the top level)
		fieldsMap, _ := tc.Arguments["fields"].(map[string]any)
		if len(fieldsMap) == 0 {
			fieldsMap, _ = tc.Arguments["blocks"].(map[string]any)
		}
		if len(fieldsMap) == 0 {
			// Try flat arguments: the dynamic schema puts block keys directly in tc.Arguments.
			// Match against existing blocks AND schema fields for the card's type so that
			// the LLM can set fields that haven't been created yet.
			currentCard2, err2 := a.repo.GetCard(cardID)
			if err2 == nil {
				knownKeys := make(map[string]bool)
				for _, b := range currentCard2.Blocks {
					if b.Key != "" {
						knownKeys[b.Key] = true
					}
				}
				if a.registry != nil && currentCard2.Type != "" {
					if s := a.registry.Get(currentCard2.Type); s != nil {
						for k := range s.Properties {
							knownKeys[k] = true
						}
					}
				}
				flat := make(map[string]any)
				for k, v := range tc.Arguments {
					if knownKeys[k] {
						flat[k] = v
					}
				}
				if len(flat) > 0 {
					fieldsMap = flat
				}
			}
		}
		if len(fieldsMap) == 0 {
			return "error: fields map is empty", nil, nil
		}
		currentCard, err := a.repo.GetCard(cardID)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}

		// Auto-create blocks for schema fields that don't exist on the card yet
		existingKeys := make(map[string]bool)
		for _, b := range currentCard.Blocks {
			if b.Key != "" {
				existingKeys[b.Key] = true
			}
		}
		if a.registry != nil && currentCard.Type != "" {
			schemaBlocks := a.registry.SchemaToBlocks(currentCard.Type)
			for _, sb := range schemaBlocks {
				if _, wantSet := fieldsMap[sb.Key]; wantSet && !existingKeys[sb.Key] {
					currentCard.Blocks = append(currentCard.Blocks, sb)
					existingKeys[sb.Key] = true
				}
			}
		}

		updated := false
		var updatedKeys []string
		for i, b := range currentCard.Blocks {
			if val, ok := fieldsMap[b.Key]; ok {
				val = coerceBlockValue(b.Type, val)
				currentCard.Blocks[i].Value = val
				updated = true
				updatedKeys = append(updatedKeys, b.Key)
			}
		}
		if !updated {
			var available []string
			for _, b := range currentCard.Blocks {
				if b.Key != "" {
					available = append(available, b.Key)
				}
			}
			return "error: no matching field keys found. Available keys: " + strings.Join(available, ", "), nil, nil
		}
		a.UpdateCardBlocks(cardID, currentCard.Blocks)
		result := "Updated fields: " + strings.Join(updatedKeys, ", ")
		action := &model.ToolAction{Tool: "set_fields", Input: tc.Arguments, Result: result}
		return result, action, nil

	case "add_tags":
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		if len(tagsRaw) == 0 {
			return "error: tags array is empty", nil, nil
		}
		var newTags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok && s != "" {
				newTags = append(newTags, s)
			}
		}
		currentCard, err := a.repo.GetCard(cardID)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}
		existing := make(map[string]bool)
		for _, t := range currentCard.Tags {
			existing[strings.ToLower(t)] = true
		}
		merged := currentCard.Tags
		var added []string
		for _, t := range newTags {
			if !existing[strings.ToLower(t)] {
				merged = append(merged, t)
				existing[strings.ToLower(t)] = true
				added = append(added, t)
			}
		}
		if len(added) > 0 {
			a.UpdateCardTags(cardID, merged)
		}
		result := "Added tags: " + strings.Join(added, ", ")
		action := &model.ToolAction{Tool: "add_tags", Input: tc.Arguments, Result: result}
		return result, action, nil

	case "add_field":
		key, _ := tc.Arguments["key"].(string)
		label, _ := tc.Arguments["label"].(string)
		fieldType, _ := tc.Arguments["field_type"].(string)
		if key == "" || label == "" || fieldType == "" {
			return "error: key, label, and field_type are required", nil, nil
		}
		// Validate field_type
		validTypes := map[string]bool{"text": true, "checklist": true, "checkbox": true, "number": true, "date": true, "url": true}
		if !validTypes[fieldType] {
			return "error: invalid field_type " + fieldType + ". Must be one of: text, checklist, checkbox, number, date, url", nil, nil
		}
		currentCard, err := a.repo.GetCard(cardID)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}
		// Check for duplicate key
		for _, b := range currentCard.Blocks {
			if b.Key == key {
				return "Field with key " + key + " already exists — use set_fields to update it.", nil, nil
			}
		}
		// Build default value for the type
		var defaultVal any
		switch fieldType {
		case "checklist":
			defaultVal = []any{}
		case "checkbox":
			defaultVal = false
		case "number":
			defaultVal = 0.0
		default:
			defaultVal = ""
		}
		// If the LLM provided an initial value, coerce and use it
		if rawVal, hasVal := tc.Arguments["value"]; hasVal && rawVal != nil {
			defaultVal = coerceBlockValue(fieldType, rawVal)
		}
		newBlock := model.Block{
			ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
			Type:  fieldType,
			Label: label,
			Key:   key,
			Value: defaultVal,
		}
		currentCard.Blocks = append(currentCard.Blocks, newBlock)
		a.UpdateCardBlocks(cardID, currentCard.Blocks)
		resultMsg := fmt.Sprintf("Added %s field '%s' (key: %s). Use set_fields with key '%s' to update its value.", fieldType, label, key, key)
		action := &model.ToolAction{Tool: "add_field", Input: tc.Arguments, Result: fmt.Sprintf("Added field: %s (%s)", label, fieldType)}
		return resultMsg, action, nil

	case "suggest_pin":
		catID, _ := tc.Arguments["category_id"].(string)
		reason, _ := tc.Arguments["reason"].(string)
		confidence, _ := tc.Arguments["confidence"].(string)

		var catName, breadcrumb string

		if catID != "" {
			// Existing category — look it up
			for _, c := range allCats {
				if c.CategoryID == catID {
					catName = c.CategoryName
					breadcrumb = c.Breadcrumb
					break
				}
			}
			if catName == "" {
				return "error: category not found", nil, nil
			}
		} else {
			// Create new hierarchy from brand/stream/project/category names
			brandName, _ := tc.Arguments["brand"].(string)
			streamName, _ := tc.Arguments["stream"].(string)
			projectName, _ := tc.Arguments["project"].(string)
			categoryName, _ := tc.Arguments["category"].(string)
			if brandName == "" || streamName == "" || projectName == "" || categoryName == "" {
				return "error: provide either category_id OR all of brand, stream, project, category names", nil, nil
			}
			resolvedCatID, resolvedBreadcrumb, err := a.resolveOrCreateHierarchy(brandName, streamName, projectName, categoryName)
			if err != nil {
				return "error creating hierarchy: " + err.Error(), nil, nil
			}
			catID = resolvedCatID
			catName = categoryName
			breadcrumb = resolvedBreadcrumb
		}

		// Check if card is already pinned to this category — skip if duplicate
		existingPins, _ := a.repo.GetCardPins(cardID)
		for _, p := range existingPins {
			if p.CategoryID == catID {
				return "Card is already pinned to " + breadcrumb, nil, nil
			}
		}

		// Auto-pin mode: pin directly without user confirmation
		if autoPinMode == "auto" {
			// Pin convention: projectID == categoryID
			if err := a.PinCard(cardID, catID, catID); err != nil {
				return "error pinning card: " + err.Error(), nil, nil
			}
			ps := &model.PinSuggestion{
				CategoryID:   catID,
				CategoryName: catName,
				Breadcrumb:   breadcrumb,
				Reason:       reason,
				Confidence:   confidence,
				Status:       "accepted",
			}
			action := &model.ToolAction{Tool: "suggest_pin", Input: tc.Arguments, Result: "Pinned to " + breadcrumb}
			return "Card pinned to " + breadcrumb, action, ps
		}

		// Suggest mode: create suggestion for user to accept/reject
		ps := &model.PinSuggestion{
			CategoryID:   catID,
			CategoryName: catName,
			Breadcrumb:   breadcrumb,
			Reason:       reason,
			Confidence:   confidence,
			Status:       "pending",
		}
		action := &model.ToolAction{Tool: "suggest_pin", Input: tc.Arguments, Result: "Suggested pin to " + breadcrumb}
		return "Pin suggestion created for " + breadcrumb, action, ps

	case "configure_agent":
		enabled, _ := tc.Arguments["enabled"].(bool)
		goal, _ := tc.Arguments["goal"].(string)
		schedule, _ := tc.Arguments["schedule"].(string)

		var allowedTools []string
		if tools, ok := tc.Arguments["allowed_tools"].([]any); ok {
			for _, t := range tools {
				if s, ok := t.(string); ok {
					allowedTools = append(allowedTools, s)
				}
			}
		}

		var notifyOn []string
		if triggers, ok := tc.Arguments["notify_on"].([]any); ok {
			for _, t := range triggers {
				if s, ok := t.(string); ok {
					notifyOn = append(notifyOn, s)
				}
			}
		}
		notifyChannel, _ := tc.Arguments["notify_channel"].(string)

		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil {
			return "error: " + err.Error(), nil, nil
		}

		af.Config.Enabled = enabled
		if goal != "" {
			af.Config.Goal = goal
		}
		if schedule != "" {
			af.Config.Schedule = schedule
		}
		if len(allowedTools) > 0 {
			af.Config.AllowedTools = allowedTools
		}
		if len(notifyOn) > 0 {
			af.Config.NotifyOn = notifyOn
		}
		if notifyChannel != "" {
			af.Config.NotifyChannel = notifyChannel
		}

		// Handle dynamic rescheduling: next_run_at and new_schedule
		if nextRunAtStr, ok := tc.Arguments["next_run_at"].(string); ok && nextRunAtStr != "" {
			if t, err := time.Parse(time.RFC3339, nextRunAtStr); err == nil {
				af.Config.NextRunAt = &t
			}
		}
		if newSchedule, ok := tc.Arguments["new_schedule"].(string); ok && newSchedule != "" {
			af.Config.Schedule = newSchedule
			schedule = newSchedule
		}

		// Set status and calculate next run
		if enabled {
			af.Config.Status = model.AgentStatusIdle
			if af.Config.NextRunAt == nil && af.Config.Schedule != "" {
				now := time.Now()
				opts := agent.ScheduleOpts{
					StartDate:         af.Config.StartDate,
					EndDate:           af.Config.EndDate,
					ActiveWindowStart: af.Config.ActiveWindowStart,
					ActiveWindowEnd:   af.Config.ActiveWindowEnd,
					OneShot:           af.Config.OneShot,
					LastRunAt:         af.Config.LastRunAt,
					Timezone:          af.Config.Timezone,
				}
				if next, err := agent.NextRunTimeWithOpts(af.Config.Schedule, now, opts); err == nil {
					af.Config.NextRunAt = &next
				}
			}
		} else {
			af.Config.Status = model.AgentStatusDisabled
		}

		if err := a.repo.SaveAgentConfig(cardID, af.Config); err != nil {
			return "error: " + err.Error(), nil, nil
		}

		// Notify any open CardDetail so the Agent tab re-fetches and
		// shows the updated goal / schedule / tools immediately. Without
		// this, the user sees the chat say "I updated the goal" but the
		// Agent tab still shows the old value until they close + reopen
		// the card — which is exactly the bug Harvey reported on
		// 2026-04-12.
		a.emitCardUpdated(cardID)

		summary := fmt.Sprintf("Agent %s — schedule: %s, tools: %s", map[bool]string{true: "enabled", false: "disabled"}[enabled], schedule, strings.Join(allowedTools, ", "))
		action := &model.ToolAction{Tool: "configure_agent", Input: tc.Arguments, Result: summary}
		return summary, action, nil

	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "error: " + err.Error()}, nil
		}
		return result, &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "fetched " + url}, nil

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "error: " + err.Error()}, nil
		}
		return result, &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "searched: " + query}, nil

	default:
		return "error: unknown tool " + tc.Name, nil, nil
	}
}

// projectChatScope bundles the per-call context that project chat tool
// callbacks need: which project we're scoped to, plus the set of card IDs that
// legitimately belong to it. The struct lets executeProjectToolCall and
// stageProjectToolCall share a single parameter without growing a long
// argument list each time we add a tool that needs project context.
type projectChatScope struct {
	brandSlug, streamSlug, projectSlug string
	cardIDs                            map[string]bool
}

// executeProjectToolCall runs a single project-level tool and returns (result, action).
//
// When `scope.cardIDs` is non-nil, any tool call referencing a card_id outside
// the set is rejected with a clear error so the LLM can correct itself. Pass
// a nil cardIDs map to disable scope checking (e.g. for ApplyProjectPendingEdits
// where we recompute scope at apply time).
func (a *App) executeProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, *model.ToolAction) {
	// Validate every card_id mentioned in the call against the project scope.
	// Single id, plural ids, and per-update entries are all checked.
	if scope.cardIDs != nil {
		var bad []string
		check := func(id string) {
			if id != "" && !scope.cardIDs[id] {
				bad = append(bad, id)
			}
		}
		if id, ok := tc.Arguments["card_id"].(string); ok {
			check(id)
		}
		if raw, ok := tc.Arguments["card_ids"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					check(s)
				}
			}
		}
		if raw, ok := tc.Arguments["updates"].([]any); ok {
			for _, item := range raw {
				if m, ok := item.(map[string]any); ok {
					if s, ok := m["card_id"].(string); ok {
						check(s)
					}
				}
			}
		}
		if len(bad) > 0 {
			return "error: card(s) not in current project: " + strings.Join(bad, ", "), nil
		}
	}
	switch tc.Name {
	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		if title == "" {
			return "error: title is required", nil
		}
		cardType, _ := tc.Arguments["card_type"].(string)
		if cardType == "" {
			cardType = "idea"
		}
		card, err := a.CreateCard(cardType, title)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		cardID := card.ID
		// Pin to category if specified. Accept either category_id or
		// category_name — the latter lets the LLM chain create_card after
		// create_category in the same conversation, since the new category
		// won't have a known ID until apply time.
		categoryID, _ := tc.Arguments["category_id"].(string)
		categoryName, _ := tc.Arguments["category_name"].(string)
		if categoryID != "" || categoryName != "" {
			resolvedID, err := a.resolveCategoryID(scope, categoryID, categoryName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			_ = a.PinCard(cardID, resolvedID, resolvedID)
			categoryID = resolvedID
		}
		// Add tags if specified
		if tagsRaw, ok := tc.Arguments["tags"].([]any); ok && len(tagsRaw) > 0 {
			var tags []string
			for _, t := range tagsRaw {
				if s, ok := t.(string); ok && s != "" {
					tags = append(tags, s)
				}
			}
			if len(tags) > 0 {
				a.UpdateCardTags(cardID, tags)
			}
		}
		// Set description if specified
		if desc, ok := tc.Arguments["description"].(string); ok && desc != "" {
			c, _ := a.repo.GetCard(cardID)
			if c != nil {
				// Find first text block or create one
				found := false
				for i, b := range c.Blocks {
					if b.Type == "text" {
						c.Blocks[i].Value = desc
						found = true
						break
					}
				}
				if !found {
					c.Blocks = append(c.Blocks, model.Block{
						ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
						Type:  "text",
						Label: "Description",
						Key:   "description",
						Value: desc,
					})
				}
				a.UpdateCardBlocks(cardID, c.Blocks)
			}
		}
		result := fmt.Sprintf("Created card '%s' (ID: %s)", title, cardID)
		if categoryID != "" {
			result += " and pinned to category"
		}
		action := &model.ToolAction{Tool: "create_card", Input: tc.Arguments, Result: result}
		return result, action

	case "add_tags_to_cards":
		cardIDsRaw, _ := tc.Arguments["card_ids"].([]any)
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		if len(cardIDsRaw) == 0 || len(tagsRaw) == 0 {
			return "error: card_ids and tags are required", nil
		}
		var newTags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok && s != "" {
				newTags = append(newTags, s)
			}
		}
		var updated int
		for _, raw := range cardIDsRaw {
			cid, ok := raw.(string)
			if !ok || cid == "" {
				continue
			}
			c, err := a.repo.GetCard(cid)
			if err != nil {
				continue
			}
			existing := make(map[string]bool)
			for _, t := range c.Tags {
				existing[strings.ToLower(t)] = true
			}
			merged := c.Tags
			for _, t := range newTags {
				if !existing[strings.ToLower(t)] {
					merged = append(merged, t)
					existing[strings.ToLower(t)] = true
				}
			}
			if len(merged) > len(c.Tags) {
				a.UpdateCardTags(cid, merged)
				updated++
			}
		}
		result := fmt.Sprintf("Added tags [%s] to %d cards", strings.Join(newTags, ", "), updated)
		action := &model.ToolAction{Tool: "add_tags_to_cards", Input: tc.Arguments, Result: result}
		return result, action

	case "move_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		// Destination: accept ID or name. Name lets the LLM chain after a
		// just-staged create_category — apply order ensures the category
		// exists by the time this resolves.
		toCatID, _ := tc.Arguments["to_category_id"].(string)
		toCatName, _ := tc.Arguments["to_category_name"].(string)
		if toCatID == "" && toCatName == "" {
			return "error: to_category_id or to_category_name is required", nil
		}
		toCat, err := a.resolveCategoryID(scope, toCatID, toCatName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		// Source: optional. Auto-detect from the card's current pin if missing.
		fromCat, _ := tc.Arguments["from_category_id"].(string)
		if fromCat == "" {
			detected, err := a.findCardCurrentCategory(scope, cardID)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			fromCat = detected
		}
		if fromCat == toCat {
			return "error: source and destination categories are the same", nil
		}
		if err := a.MoveCardToCategory(cardID, fromCat, fromCat, toCat, 0); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Card moved to new category"
		action := &model.ToolAction{Tool: "move_card", Input: tc.Arguments, Result: result}
		return result, action

	case "update_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		changes, err := a.applyCardUpdate(cardID, tc.Arguments)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_card", Input: tc.Arguments, Result: result}
		return result, action

	case "update_cards":
		updatesRaw, _ := tc.Arguments["updates"].([]any)
		if len(updatesRaw) == 0 {
			return "error: updates array is required", nil
		}
		type cardResult struct {
			cardID  string
			changes []string
			err     error
		}
		var results []cardResult
		var totalChanges int
		var failures int
		for _, raw := range updatesRaw {
			entry, ok := raw.(map[string]any)
			if !ok {
				failures++
				continue
			}
			cardID, _ := entry["card_id"].(string)
			if cardID == "" {
				failures++
				continue
			}
			changes, err := a.applyCardUpdate(cardID, entry)
			results = append(results, cardResult{cardID: cardID, changes: changes, err: err})
			if err != nil {
				failures++
			} else {
				totalChanges += len(changes)
			}
		}
		var summary strings.Builder
		successes := len(results) - failures
		summary.WriteString(fmt.Sprintf("Updated %d cards (%d field changes total)", successes, totalChanges))
		if failures > 0 {
			summary.WriteString(fmt.Sprintf("; %d failed", failures))
		}
		action := &model.ToolAction{Tool: "update_cards", Input: tc.Arguments, Result: summary.String()}
		return summary.String(), action

	case "configure_agent":
		cardID, _ := tc.Arguments["card_id"].(string)
		if cardID == "" {
			return "error: card_id is required", nil
		}
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		var changes []string
		if v, ok := tc.Arguments["enabled"].(bool); ok {
			af.Config.Enabled = v
			changes = append(changes, fmt.Sprintf("enabled=%t", v))
		}
		if v, ok := tc.Arguments["schedule"].(string); ok {
			af.Config.Schedule = v
			if v == "" {
				changes = append(changes, "schedule=cleared")
			} else {
				changes = append(changes, "schedule="+v)
			}
		}
		if v, ok := tc.Arguments["goal"].(string); ok {
			af.Config.Goal = v
			changes = append(changes, "goal")
		}
		if raw, ok := tc.Arguments["allowed_tools"].([]any); ok {
			tools := make([]string, 0, len(raw))
			for _, t := range raw {
				if s, ok := t.(string); ok && s != "" {
					tools = append(tools, s)
				}
			}
			af.Config.AllowedTools = tools
			changes = append(changes, "allowed_tools")
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		if err := a.repo.SaveAgentConfig(cardID, af.Config); err != nil {
			return "error: " + err.Error(), nil
		}
		a.emitCardUpdated(cardID)
		result := "Configured agent: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "configure_agent", Input: tc.Arguments, Result: result}
		return result, action

	// --- Project metadata ---
	case "update_project":
		var changes []string
		if name, ok := tc.Arguments["name"].(string); ok && name != "" {
			if _, err := a.RenameProject(scope.brandSlug, scope.streamSlug, scope.projectSlug, name); err != nil {
				return "error: " + err.Error(), nil
			}
			changes = append(changes, "name")
			// Slug may have changed after rename — refresh it for subsequent calls.
			if p, err := a.repo.GetProject(scope.brandSlug, scope.streamSlug, name); err == nil {
				scope.projectSlug = p.Slug
			}
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			if _, err := a.UpdateProjectDescription(scope.brandSlug, scope.streamSlug, scope.projectSlug, v); err == nil {
				changes = append(changes, "description")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := a.UpdateProjectIcon(scope.brandSlug, scope.streamSlug, scope.projectSlug, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated project: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_project", Input: tc.Arguments, Result: result}
		return result, action

	// --- Project tags ---
	case "create_project_tag":
		name, _ := tc.Arguments["name"].(string)
		if name == "" {
			return "error: name is required", nil
		}
		color, _ := tc.Arguments["color"].(string)
		// Underlying type is model.Label / AddProjectLabel — that's just the
		// historical persistence name. The user-facing concept is "tag".
		labels, err := a.AddProjectLabel(scope.brandSlug, scope.streamSlug, scope.projectSlug, name, color)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		// Optional icon — set in a follow-up call once we know the new ID.
		if icon, ok := tc.Arguments["icon"].(string); ok && icon != "" {
			for _, l := range labels {
				if strings.EqualFold(l.Name, name) {
					_, _ = a.SetProjectLabelIcon(scope.brandSlug, scope.streamSlug, scope.projectSlug, l.ID, icon)
					break
				}
			}
		}
		result := "Created tag: " + name
		action := &model.ToolAction{Tool: "create_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	case "update_project_tag":
		tagID, _ := tc.Arguments["tag_id"].(string)
		if tagID == "" {
			tagName, _ := tc.Arguments["tag_name"].(string)
			if tagName == "" {
				return "error: tag_id or tag_name is required", nil
			}
			id, err := a.findProjectTagID(scope, tagName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			tagID = id
		}
		var changes []string
		// Name + color go through UpdateProjectLabel together. Empty strings
		// preserve the existing value (per repo.UpdateProjectLabel semantics).
		newName, hasName := tc.Arguments["name"].(string)
		newColor, hasColor := tc.Arguments["color"].(string)
		if hasName || hasColor {
			passName := ""
			if hasName {
				passName = newName
			}
			passColor := ""
			if hasColor {
				passColor = newColor
			}
			if _, err := a.UpdateProjectLabel(scope.brandSlug, scope.streamSlug, scope.projectSlug, tagID, passName, passColor); err != nil {
				return "error: " + err.Error(), nil
			}
			if hasName {
				changes = append(changes, "name")
			}
			if hasColor {
				changes = append(changes, "color")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := a.SetProjectLabelIcon(scope.brandSlug, scope.streamSlug, scope.projectSlug, tagID, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated tag: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	case "delete_project_tag":
		tagID, _ := tc.Arguments["tag_id"].(string)
		if tagID == "" {
			tagName, _ := tc.Arguments["tag_name"].(string)
			if tagName == "" {
				return "error: tag_id or tag_name is required", nil
			}
			id, err := a.findProjectTagID(scope, tagName)
			if err != nil {
				return "error: " + err.Error(), nil
			}
			tagID = id
		}
		if _, err := a.RemoveProjectLabel(scope.brandSlug, scope.streamSlug, scope.projectSlug, tagID); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Deleted tag"
		action := &model.ToolAction{Tool: "delete_project_tag", Input: tc.Arguments, Result: result}
		return result, action

	// --- Categories ---
	case "create_category":
		name, _ := tc.Arguments["name"].(string)
		if name == "" {
			return "error: name is required", nil
		}
		position := 0
		if v, ok := tc.Arguments["position"].(float64); ok {
			position = int(v)
		}
		cat, err := a.CreateCategory(scope.brandSlug, scope.streamSlug, scope.projectSlug, name, position)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		result := fmt.Sprintf("Created category '%s' (id: %s)", name, cat.ID)
		action := &model.ToolAction{Tool: "create_category", Input: tc.Arguments, Result: result}
		return result, action

	case "update_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		resolvedID, err := a.resolveCategoryID(scope, catID, catName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		catID = resolvedID
		// Find the category's slug — the existing app methods all key by slug.
		catSlug, err := a.findCategorySlug(scope, catID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		var changes []string
		if name, ok := tc.Arguments["name"].(string); ok && name != "" {
			if _, err := a.RenameCategory(scope.brandSlug, scope.streamSlug, scope.projectSlug, catSlug, name); err != nil {
				return "error: " + err.Error(), nil
			}
			changes = append(changes, "name")
			// Slug may have changed after rename.
			if newSlug, err := a.findCategorySlug(scope, catID); err == nil {
				catSlug = newSlug
			}
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			if _, err := a.UpdateCategoryDescription(scope.brandSlug, scope.streamSlug, scope.projectSlug, catSlug, v); err == nil {
				changes = append(changes, "description")
			}
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			if _, err := a.UpdateCategoryIcon(scope.brandSlug, scope.streamSlug, scope.projectSlug, catSlug, v); err == nil {
				changes = append(changes, "icon")
			}
		}
		if raw, ok := tc.Arguments["accepted_types"].([]any); ok {
			types := make([]string, 0, len(raw))
			for _, t := range raw {
				if s, ok := t.(string); ok {
					types = append(types, s)
				}
			}
			if _, err := a.UpdateCategoryAcceptedTypes(scope.brandSlug, scope.streamSlug, scope.projectSlug, catSlug, types); err == nil {
				changes = append(changes, "accepted_types")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated category: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_category", Input: tc.Arguments, Result: result}
		return result, action

	case "delete_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		resolvedID, err := a.resolveCategoryID(scope, catID, catName)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		catID = resolvedID
		catSlug, err := a.findCategorySlug(scope, catID)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if err := a.DeleteCategory(scope.brandSlug, scope.streamSlug, scope.projectSlug, catSlug); err != nil {
			return "error: " + err.Error(), nil
		}
		result := "Deleted category"
		action := &model.ToolAction{Tool: "delete_category", Input: tc.Arguments, Result: result}
		return result, action

	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "error: " + err.Error()}
		}
		return result, &model.ToolAction{Tool: "web_fetch", Input: tc.Arguments, Result: "fetched " + url}

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "error: " + err.Error()}
		}
		return result, &model.ToolAction{Tool: "web_search", Input: tc.Arguments, Result: "searched: " + query}

	default:
		return "error: unknown tool " + tc.Name, nil
	}
}

// findProjectTagID looks up a tag by name (case-insensitive) within the
// current project and returns its ID. Used by the tag tools when they accept
// `tag_name` as a fallback to `tag_id`.
//
// (Underlying repo type is `model.Label` for historical persistence reasons,
// but the user-facing concept is "tag" — see also feedback_tags_not_labels.)
func (a *App) findProjectTagID(scope projectChatScope, name string) (string, error) {
	labels, err := a.GetProjectLabels(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return "", err
	}
	for _, l := range labels {
		if strings.EqualFold(l.Name, name) {
			return l.ID, nil
		}
	}
	return "", fmt.Errorf("no tag named %q in this project", name)
}

// findCategorySlug looks up a category's slug by its ID within the current
// project. The existing repo methods key categories by slug rather than ID,
// so this bridges the gap when tools accept category_id.
func (a *App) findCategorySlug(scope projectChatScope, catID string) (string, error) {
	cats, err := a.repo.ListCategories(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return "", err
	}
	for _, c := range cats {
		if c.ID == catID {
			return c.Slug, nil
		}
	}
	return "", fmt.Errorf("category %s not found in current project", catID)
}

// resolveCategoryID resolves either an ID or a name (case-insensitive) into a
// canonical category ID for the current project. Used by tools that accept
// `category_id` and `category_name` as alternatives — this lets the LLM refer
// to a category it just created in the same conversation by name, since the ID
// won't be known until apply time.
//
// If both `id` and `name` are supplied, ID takes precedence. Returns an error
// if neither resolves to a category in this project.
func (a *App) resolveCategoryID(scope projectChatScope, id, name string) (string, error) {
	cats, err := a.repo.ListCategories(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return "", err
	}
	if id != "" {
		for _, c := range cats {
			if c.ID == id {
				return c.ID, nil
			}
		}
		// ID supplied but not in this project — fall through to try name lookup
		// in case the LLM mixed them up.
	}
	if name != "" {
		for _, c := range cats {
			if strings.EqualFold(c.Name, name) {
				return c.ID, nil
			}
		}
	}
	if id == "" && name == "" {
		return "", fmt.Errorf("category_id or category_name is required")
	}
	if id != "" && name == "" {
		return "", fmt.Errorf("category %s not found in current project", id)
	}
	return "", fmt.Errorf("no category named %q in current project", name)
}

// findCardCurrentCategory returns the category ID a card is currently pinned
// to within the given project. Used by `move_card` to auto-detect the source
// category when the LLM doesn't supply `from_category_id`.
//
// Walks the project's categories looking for a pin matching the card. If the
// card is pinned to multiple categories in this project (rare), returns the
// first one found. Returns an error if the card isn't pinned anywhere here.
func (a *App) findCardCurrentCategory(scope projectChatScope, cardID string) (string, error) {
	cats, err := a.repo.ListCategories(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return "", err
	}
	for _, cat := range cats {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			if p.CardID == cardID {
				return cat.ID, nil
			}
		}
	}
	return "", fmt.Errorf("card %s is not pinned to any category in this project", cardID)
}

// applyCardUpdate applies a partial update to a single card. Used by both
// update_card (single) and update_cards (plural). Returns the list of fields
// that actually changed (used to build the tool-action result string).
//
// Supported keys in args:
//   - title (string)
//   - card_type (string)
//   - tags ([]string)              — REPLACE the card's tags
//   - tags_to_add ([]string)       — APPEND to existing tags (deduped)
//   - due_date (string)            — ISO 8601 date or datetime; "" clears
//   - description (string)         — replaces the first text block, or creates one
//   - blocks ([]map)               — REPLACE the card's blocks entirely
//
// Unknown keys are ignored.
func (a *App) applyCardUpdate(cardID string, args map[string]any) ([]string, error) {
	var changes []string

	if title, ok := args["title"].(string); ok && title != "" {
		if _, err := a.UpdateCardTitle(cardID, title); err == nil {
			changes = append(changes, "title")
		}
	}
	if cardType, ok := args["card_type"].(string); ok && cardType != "" {
		if _, err := a.UpdateCardType(cardID, cardType); err == nil {
			changes = append(changes, "type")
		}
	}

	// Tag handling: `tags` REPLACES, `tags_to_add` APPENDS. Both can be present.
	if tagsRaw, ok := args["tags"].([]any); ok {
		var newTags []string
		for _, raw := range tagsRaw {
			if s, ok := raw.(string); ok && s != "" {
				newTags = append(newTags, s)
			}
		}
		if _, err := a.UpdateCardTags(cardID, newTags); err == nil {
			changes = append(changes, "tags")
		}
	}
	if tagsRaw, ok := args["tags_to_add"].([]any); ok && len(tagsRaw) > 0 {
		c, err := a.repo.GetCard(cardID)
		if err == nil {
			existing := make(map[string]bool)
			for _, t := range c.Tags {
				existing[strings.ToLower(t)] = true
			}
			merged := c.Tags
			added := false
			for _, raw := range tagsRaw {
				if s, ok := raw.(string); ok && s != "" && !existing[strings.ToLower(s)] {
					merged = append(merged, s)
					existing[strings.ToLower(s)] = true
					added = true
				}
			}
			if added {
				a.UpdateCardTags(cardID, merged)
				if !contains(changes, "tags") {
					changes = append(changes, "tags")
				}
			}
		}
	}
	if tagsRaw, ok := args["tags_to_remove"].([]any); ok && len(tagsRaw) > 0 {
		c, err := a.repo.GetCard(cardID)
		if err == nil {
			remove := make(map[string]bool, len(tagsRaw))
			for _, raw := range tagsRaw {
				if s, ok := raw.(string); ok && s != "" {
					remove[strings.ToLower(s)] = true
				}
			}
			filtered := make([]string, 0, len(c.Tags))
			removed := false
			for _, t := range c.Tags {
				if remove[strings.ToLower(t)] {
					removed = true
					continue
				}
				filtered = append(filtered, t)
			}
			if removed {
				a.UpdateCardTags(cardID, filtered)
				if !contains(changes, "tags") {
					changes = append(changes, "tags")
				}
			}
		}
	}

	// Due date: empty string clears, non-empty parses as ISO date or datetime.
	if v, ok := args["due_date"].(string); ok {
		if v == "" {
			if _, err := a.UpdateCardDueDate(cardID, ""); err == nil {
				changes = append(changes, "due_date")
			}
		} else {
			if _, err := a.UpdateCardDueDate(cardID, v); err == nil {
				changes = append(changes, "due_date")
			}
		}
	}

	// Description shorthand: replace the first text/markdown block, or create one.
	if desc, ok := args["description"].(string); ok {
		c, err := a.repo.GetCard(cardID)
		if err == nil {
			found := false
			for i, b := range c.Blocks {
				if b.Type == "text" || b.Type == "markdown" {
					c.Blocks[i].Value = desc
					found = true
					break
				}
			}
			if !found {
				c.Blocks = append(c.Blocks, model.Block{
					ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
					Type:  "text",
					Label: "Description",
					Key:   "description",
					Value: desc,
				})
			}
			if _, err := a.UpdateCardBlocks(cardID, c.Blocks); err == nil {
				changes = append(changes, "description")
			}
		}
	}

	// Block-level replacement (full restructure).
	if raw, ok := args["blocks"].([]any); ok {
		blocks := make([]model.Block, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			b := model.Block{
				Type:  asString(m["type"]),
				Label: asString(m["label"]),
				Key:   asString(m["key"]),
				Value: m["value"],
			}
			if id := asString(m["id"]); id != "" {
				b.ID = id
			} else {
				b.ID = fmt.Sprintf("blk-%s", uuid.New().String()[:8])
			}
			blocks = append(blocks, b)
		}
		if _, err := a.UpdateCardBlocks(cardID, blocks); err == nil {
			changes = append(changes, "blocks")
		}
	}

	return changes, nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// resolveOrCreateHierarchy finds or creates brand/stream/project/category by name.
// Returns (categoryID, breadcrumb, error).
func (a *App) resolveOrCreateHierarchy(brandName, streamName, projectName, categoryName string) (string, string, error) {
	// Check if provided names match an existing category path.
	// The LLM sometimes scrambles the hierarchy order (e.g. puts card title as brand),
	// so we check if any existing path contains all provided names regardless of position.
	allCats, _ := a.ListAllCategories()
	inputNames := []string{brandName, streamName, projectName, categoryName}

	// Exact positional match first (brand=brand, stream=stream, etc.)
	for _, c := range allCats {
		if strings.EqualFold(c.BrandName, brandName) &&
			strings.EqualFold(c.StreamName, streamName) &&
			strings.EqualFold(c.ProjectName, projectName) &&
			strings.EqualFold(c.CategoryName, categoryName) {
			return c.CategoryID, c.Breadcrumb, nil
		}
	}

	// Fuzzy match: if >=3 of the 4 provided names appear somewhere in an existing path
	// (regardless of position), use that path instead of creating new hierarchy
	for _, c := range allCats {
		pathNames := []string{c.BrandName, c.StreamName, c.ProjectName, c.CategoryName}
		matches := 0
		for _, input := range inputNames {
			for _, pn := range pathNames {
				if strings.EqualFold(input, pn) {
					matches++
					break
				}
			}
		}
		if matches >= 3 {
			return c.CategoryID, c.Breadcrumb, nil
		}
	}

	// 1. Find or create brand
	brandSlug := ""
	brands, _ := a.ListBrands()
	for _, b := range brands {
		if strings.EqualFold(b.Name, brandName) {
			brandSlug = b.Slug
			brandName = b.Name // use canonical name
			break
		}
	}
	if brandSlug == "" {
		b, err := a.CreateBrand(brandName)
		if err != nil {
			return "", "", fmt.Errorf("creating brand %q: %w", brandName, err)
		}
		brandSlug = b.Slug
	}

	// 2. Find or create stream
	streamSlug := ""
	streams, _ := a.ListStreams(brandSlug)
	for _, s := range streams {
		if strings.EqualFold(s.Name, streamName) {
			streamSlug = s.Slug
			streamName = s.Name
			break
		}
	}
	if streamSlug == "" {
		s, err := a.CreateStream(brandSlug, streamName)
		if err != nil {
			return "", "", fmt.Errorf("creating stream %q: %w", streamName, err)
		}
		streamSlug = s.Slug
	}

	// 3. Find or create project
	projectSlug := ""
	projects, _ := a.ListProjects(brandSlug, streamSlug)
	for _, p := range projects {
		if strings.EqualFold(p.Name, projectName) {
			projectSlug = p.Slug
			projectName = p.Name
			break
		}
	}
	if projectSlug == "" {
		p, err := a.CreateProject(brandSlug, streamSlug, projectName)
		if err != nil {
			return "", "", fmt.Errorf("creating project %q: %w", projectName, err)
		}
		projectSlug = p.Slug
	}

	// 4. Find or create category
	var catID string
	cats, _ := a.ListCategories(brandSlug, streamSlug, projectSlug)
	for _, c := range cats {
		if strings.EqualFold(c.Name, categoryName) {
			catID = c.ID
			categoryName = c.Name
			break
		}
	}
	if catID == "" {
		c, err := a.CreateCategory(brandSlug, streamSlug, projectSlug, categoryName, len(cats))
		if err != nil {
			return "", "", fmt.Errorf("creating category %q: %w", categoryName, err)
		}
		catID = c.ID
	}

	breadcrumb := brandName + " / " + streamName + " / " + projectName + " / " + categoryName
	return catID, breadcrumb, nil
}

// stageToolCall builds a PendingEdit record for Suggest mode without applying any changes.
// It returns a fake result string (fed back to the LLM so the conversation continues naturally)
// and the PendingEdit to be stored on the message.
func (a *App) stageToolCall(tc llm.ToolCall, allCats []CategoryPath) (string, []model.PendingEdit) {
	one := func(tool string, input map[string]any, label, detail string) []model.PendingEdit {
		return []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tool, Input: input,
			Label: label, Detail: detail, Status: "pending",
		}}
	}

	switch tc.Name {
	case "set_title":
		title, _ := tc.Arguments["title"].(string)
		return "Title will be set to " + title, one(tc.Name, tc.Arguments, "Set title", `"`+title+`"`)

	case "set_due_date":
		dueDate, _ := tc.Arguments["due_date"].(string)
		label, detail := "Set due date", dueDate
		if dueDate == "" {
			label, detail = "Clear due date", "Remove existing due date"
		}
		return "Due date staged", one(tc.Name, tc.Arguments, label, detail)

	case "set_card_type":
		cardType, _ := tc.Arguments["card_type"].(string)
		fakeResult := "Card type will be set to " + cardType + "."
		var previewBlocks []model.Block
		var store config.UserTypeStore
		if a.repo != nil {
			store, _ = a.repo.LoadUserTypeStore()
		}
		for _, ut := range store.Types {
			if ut.ID == cardType && ut.TemplateID != "" {
				for _, tmpl := range store.Templates {
					if tmpl.ID == ut.TemplateID {
						previewBlocks = tmpl.Blocks
						break
					}
				}
				break
			}
		}
		if len(previewBlocks) == 0 {
			if ov, ok := store.BuiltinOverrides[cardType]; ok && ov.TemplateID != "" {
				for _, tmpl := range store.Templates {
					if tmpl.ID == ov.TemplateID {
						previewBlocks = tmpl.Blocks
						break
					}
				}
			}
		}
		if len(previewBlocks) == 0 && a.registry != nil {
			previewBlocks = a.registry.SchemaToBlocks(cardType)
		}
		if len(previewBlocks) > 0 {
			var keys []string
			for _, b := range previewBlocks {
				if b.Key != "" {
					keys = append(keys, b.Key)
				}
			}
			if len(keys) > 0 {
				fakeResult += " NOW call set_fields to fill these field keys: " + strings.Join(keys, ", ")
			}
		}
		return fakeResult, one(tc.Name, tc.Arguments, "Set type", cardType)

	case "set_fields", "update_blocks":
		fieldsMap, _ := tc.Arguments["fields"].(map[string]any)
		if len(fieldsMap) == 0 {
			fieldsMap, _ = tc.Arguments["blocks"].(map[string]any)
		}
		if len(fieldsMap) == 0 {
			fieldsMap = make(map[string]any)
			for k, v := range tc.Arguments {
				fieldsMap[k] = v
			}
		}
		// One PendingEdit per field so the user can review each individually
		var edits []model.PendingEdit
		var keys []string
		for k, v := range fieldsMap {
			keys = append(keys, k)
			detail := fmt.Sprintf("%v", v)
			if s, ok := v.(string); ok && len(s) > 120 {
				detail = s[:120] + "…"
			}
			edits = append(edits, model.PendingEdit{
				ID:     uuid.New().String(),
				Tool:   tc.Name,
				Input:  map[string]any{k: v},
				Label:  humanizeBlockKey(k),
				Detail: detail,
				Status: "pending",
			})
		}
		return "Fields staged: " + strings.Join(keys, ", "), edits

	case "add_tags":
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		var tags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, "+"+s)
			}
		}
		return "Tags staged", one(tc.Name, tc.Arguments, "Add tags", strings.Join(tags, ", "))

	case "add_field":
		label, _ := tc.Arguments["label"].(string)
		fieldType, _ := tc.Arguments["field_type"].(string)
		return "Field staged: " + label, one(tc.Name, tc.Arguments, "Add field: "+label, "Type: "+fieldType)

	case "suggest_pin":
		reason, _ := tc.Arguments["reason"].(string)
		catID, _ := tc.Arguments["category_id"].(string)
		var breadcrumb string
		if catID != "" {
			for _, c := range allCats {
				if c.CategoryID == catID {
					breadcrumb = c.Breadcrumb
					break
				}
			}
		} else {
			brand, _ := tc.Arguments["brand"].(string)
			stream, _ := tc.Arguments["stream"].(string)
			project, _ := tc.Arguments["project"].(string)
			category, _ := tc.Arguments["category"].(string)
			var parts []string
			for _, p := range []string{brand, stream, project, category} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			breadcrumb = strings.Join(parts, " / ")
		}
		detail := breadcrumb
		if reason != "" {
			detail += "\n" + reason
		}
		return "Pin suggestion staged for " + breadcrumb, one(tc.Name, tc.Arguments, "Pin to "+breadcrumb, detail)

	case "configure_agent":
		enabled, _ := tc.Arguments["enabled"].(bool)
		goal, _ := tc.Arguments["goal"].(string)
		schedule, _ := tc.Arguments["schedule"].(string)
		label := "Configure agent"
		detail := fmt.Sprintf("Enabled: %v, Schedule: %s\nGoal: %s", enabled, schedule, goal)
		return "Agent configuration staged", one(tc.Name, tc.Arguments, label, detail)

	// Read-only tools bypass suggest-mode staging: there's nothing to
	// preview or approve — fetching a page or searching the web can't
	// mutate the card. Execute directly and feed the real content back
	// to the model so it can reason over it on the next iteration.
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// stageProjectToolCall builds PendingEdit records for project chat in suggest mode.
//
// Strategy: each "logical edit" gets its own PendingEdit so the user can
// approve or reject them individually. For tools that touch multiple cards or
// fields, we expand them into one edit per (card, field) pair. This is what
// gives the review UI a flat list of "this card, this field, this preview"
// rows that the user can mouse-hover for full detail.
//
// `scope.cardIDs` is the set of valid card IDs for the current project.
// Any card_id outside the set is dropped from staging and reported back to
// the LLM via the result string so it can correct itself on the next turn.
// Pass a nil cardIDs map to disable scope checking.
//
// The result string fed back to the LLM acknowledges the staging (and lists
// any rejected IDs) so the conversation continues naturally without the LLM
// thinking the call silently failed.
func (a *App) stageProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, []model.PendingEdit) {
	inScope := func(id string) bool {
		if scope.cardIDs == nil {
			return true
		}
		return scope.cardIDs[id]
	}
	switch tc.Name {
	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		cardType, _ := tc.Arguments["card_type"].(string)
		if cardType == "" {
			cardType = "idea"
		}
		label := "Create card: " + title
		detail := fmt.Sprintf("Type: %s", cardType)
		// Pin destination — prefer name (more meaningful in the row), fall
		// back to resolving the ID to a name, finally raw ID.
		catName, _ := tc.Arguments["category_name"].(string)
		catID, _ := tc.Arguments["category_id"].(string)
		pinDisplay := catName
		if pinDisplay == "" && catID != "" {
			pinDisplay = a.categoryDisplayName(scope, catID)
		}
		if pinDisplay != "" {
			detail += "\nPin to category: " + pinDisplay
		}
		if desc, _ := tc.Arguments["description"].(string); desc != "" {
			detail += "\n\n" + desc
		}
		return "Card creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tc.Name, Input: tc.Arguments,
			Label: label, Detail: detail, Status: "pending",
		}}

	case "update_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project. Use only the card IDs listed in the system prompt.", nil
		}
		return formatStageResult(tc.Name), a.stageProjectCardUpdates(cardID, tc.Arguments)

	case "update_cards":
		updatesRaw, _ := tc.Arguments["updates"].([]any)
		var allEdits []model.PendingEdit
		var rejected []string
		for _, raw := range updatesRaw {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			cardID, _ := entry["card_id"].(string)
			if cardID == "" {
				continue
			}
			if !inScope(cardID) {
				rejected = append(rejected, cardID)
				continue
			}
			// Stage as singular update_card edits regardless of which tool the
			// LLM called. The plural-vs-singular distinction only matters at
			// LLM-call time; on apply, each pending edit is one card / one
			// field, so the singular executor branch is the right path.
			edits := a.stageProjectCardUpdates(cardID, entry)
			allEdits = append(allEdits, edits...)
		}
		if len(rejected) > 0 && len(allEdits) == 0 {
			return "error: none of the supplied card_ids belong to the current project: " + strings.Join(rejected, ", ") + ". Use only the card IDs listed in the system prompt.", nil
		}
		summary := fmt.Sprintf("Staged %d edits across %d cards", len(allEdits), len(updatesRaw)-len(rejected))
		if len(rejected) > 0 {
			summary += fmt.Sprintf(" (skipped %d out-of-project: %s)", len(rejected), strings.Join(rejected, ", "))
		}
		return summary, allEdits

	case "add_tags_to_cards":
		cardIDsRaw, _ := tc.Arguments["card_ids"].([]any)
		tagsRaw, _ := tc.Arguments["tags"].([]any)
		var tags []string
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
		var edits []model.PendingEdit
		var rejected []string
		for _, raw := range cardIDsRaw {
			cid, ok := raw.(string)
			if !ok || cid == "" {
				continue
			}
			if !inScope(cid) {
				rejected = append(rejected, cid)
				continue
			}
			cardLabel := a.cardDisplayLabel(cid)
			// Stage one edit per card so each can be approved individually,
			// but keep the `card_ids` (plural) shape so the executor matches
			// the original tool contract — it loops the array even when len=1.
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(),
				Tool:  tc.Name,
				Input: map[string]any{"card_ids": []any{cid}, "tags": tagsRaw},
				Label: cardLabel + " — add tags",
				Detail: "+" + strings.Join(tags, ", +"),
				Status: "pending",
			})
		}
		if len(rejected) > 0 && len(edits) == 0 {
			return "error: none of the supplied card_ids belong to the current project: " + strings.Join(rejected, ", "), nil
		}
		summary := fmt.Sprintf("Tag additions staged for %d cards", len(edits))
		if len(rejected) > 0 {
			summary += fmt.Sprintf(" (skipped %d out-of-project: %s)", len(rejected), strings.Join(rejected, ", "))
		}
		return summary, edits

	case "move_card":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project.", nil
		}
		// Display the destination by name when one is provided. The actual
		// resolution (id-or-name → id) happens at apply time, by which point
		// any just-staged create_category will have been applied first.
		toCatName, _ := tc.Arguments["to_category_name"].(string)
		toCatID, _ := tc.Arguments["to_category_id"].(string)
		toDisplay := toCatName
		if toDisplay == "" && toCatID != "" {
			toDisplay = a.categoryDisplayName(scope, toCatID)
		}
		if toDisplay == "" {
			toDisplay = "(unspecified)"
		}
		return "Move staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: tc.Name, Input: tc.Arguments,
			Label: a.cardDisplayLabel(cardID) + " — move",
			Detail: "To category: " + toDisplay,
			Status: "pending",
		}}

	case "configure_agent":
		cardID, _ := tc.Arguments["card_id"].(string)
		if !inScope(cardID) {
			return "error: card " + cardID + " is not in the current project.", nil
		}
		cardLabel := a.cardDisplayLabel(cardID)
		var edits []model.PendingEdit
		if v, ok := tc.Arguments["enabled"].(bool); ok {
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(), Tool: tc.Name,
				Input: map[string]any{"card_id": cardID, "enabled": v},
				Label: cardLabel + " — agent enabled",
				Detail: fmt.Sprintf("Set agent enabled to %t", v),
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["schedule"].(string); ok {
			detail := "Schedule: " + v
			if v == "" {
				detail = "Clear schedule"
			}
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(), Tool: tc.Name,
				Input: map[string]any{"card_id": cardID, "schedule": v},
				Label: cardLabel + " — agent schedule",
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["goal"].(string); ok {
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(), Tool: tc.Name,
				Input: map[string]any{"card_id": cardID, "goal": v},
				Label: cardLabel + " — agent goal",
				Detail: v,
				Status: "pending",
			})
		}
		if raw, ok := tc.Arguments["allowed_tools"].([]any); ok {
			var tools []string
			for _, t := range raw {
				if s, ok := t.(string); ok {
					tools = append(tools, s)
				}
			}
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(), Tool: tc.Name,
				Input: map[string]any{"card_id": cardID, "allowed_tools": raw},
				Label: cardLabel + " — agent tools",
				Detail: strings.Join(tools, ", "),
				Status: "pending",
			})
		}
		return "Agent configuration staged", edits

	// --- Project metadata ---
	case "update_project":
		var edits []model.PendingEdit
		mk := func(field string, fieldArg any, detail string) {
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(),
				Tool:  "update_project",
				Input: map[string]any{field: fieldArg},
				Label: "Project — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok && v != "" {
			mk("name", v, v)
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear description"
			}
			mk("description", v, detail)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", v, detail)
		}
		return "Project update staged", edits

	// --- Project tags ---
	case "create_project_tag":
		name, _ := tc.Arguments["name"].(string)
		var detailParts []string
		if c, _ := tc.Arguments["color"].(string); c != "" {
			detailParts = append(detailParts, "color "+c)
		}
		if i, _ := tc.Arguments["icon"].(string); i != "" {
			detailParts = append(detailParts, "icon "+i)
		}
		detail := name
		if len(detailParts) > 0 {
			detail += " (" + strings.Join(detailParts, ", ") + ")"
		}
		return "Tag creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "create_project_tag", Input: tc.Arguments,
			Label: "Create tag — " + name, Detail: detail, Status: "pending",
		}}

	case "update_project_tag":
		// Resolve tag name for the row label so the user knows what's changing.
		tagLabel := "tag"
		if id, _ := tc.Arguments["tag_id"].(string); id != "" {
			tagLabel = a.tagDisplayName(scope, id, "")
		} else if name, _ := tc.Arguments["tag_name"].(string); name != "" {
			tagLabel = name
		}
		var edits []model.PendingEdit
		mk := func(field string, detail string) {
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(),
				Tool:  "update_project_tag",
				Input: shallowCopyArgs(tc.Arguments, []string{"tag_id", "tag_name"}, field),
				Label: tagLabel + " — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok {
			mk("name", v)
		}
		if v, ok := tc.Arguments["color"].(string); ok {
			mk("color", v)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", detail)
		}
		return "Tag update staged", edits

	case "delete_project_tag":
		tagLabel := "tag"
		if id, _ := tc.Arguments["tag_id"].(string); id != "" {
			tagLabel = a.tagDisplayName(scope, id, "")
		} else if name, _ := tc.Arguments["tag_name"].(string); name != "" {
			tagLabel = name
		}
		return "Tag deletion staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "delete_project_tag", Input: tc.Arguments,
			Label: "Delete tag — " + tagLabel, Detail: "Delete from project", Status: "pending",
		}}

	// --- Categories ---
	case "create_category":
		name, _ := tc.Arguments["name"].(string)
		return "Category creation staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "create_category", Input: tc.Arguments,
			Label: "Create category — " + name, Detail: name, Status: "pending",
		}}

	case "update_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		// Use the supplied name if present so the row label is meaningful even
		// when the LLM only provided category_name (a category that doesn't
		// exist yet at staging time). Apply will resolve the actual ID.
		catLabel := catName
		if catLabel == "" && catID != "" {
			catLabel = a.categoryDisplayName(scope, catID)
		}
		if catLabel == "" {
			catLabel = "category"
		}
		// Lookup keys preserved on each per-field PendingEdit so apply can
		// resolve the right category whichever form the LLM used.
		lookup := map[string]any{}
		if catID != "" {
			lookup["category_id"] = catID
		}
		if catName != "" {
			lookup["category_name"] = catName
		}
		var edits []model.PendingEdit
		mk := func(field string, fieldArg any, detail string) {
			input := map[string]any{}
			for k, v := range lookup {
				input[k] = v
			}
			input[field] = fieldArg
			edits = append(edits, model.PendingEdit{
				ID:    uuid.New().String(),
				Tool:  "update_category",
				Input: input,
				Label: catLabel + " — " + field,
				Detail: detail,
				Status: "pending",
			})
		}
		if v, ok := tc.Arguments["name"].(string); ok && v != "" {
			mk("name", v, v)
		}
		if v, ok := tc.Arguments["description"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear description"
			}
			mk("description", v, detail)
		}
		if v, ok := tc.Arguments["icon"].(string); ok {
			detail := v
			if v == "" {
				detail = "Clear icon"
			}
			mk("icon", v, detail)
		}
		if raw, ok := tc.Arguments["accepted_types"].([]any); ok {
			var types []string
			for _, t := range raw {
				if s, ok := t.(string); ok {
					types = append(types, s)
				}
			}
			detail := "Accept: " + strings.Join(types, ", ")
			if len(types) == 0 {
				detail = "Accept all card types"
			}
			mk("accepted_types", raw, detail)
		}
		return "Category update staged", edits

	case "delete_category":
		catID, _ := tc.Arguments["category_id"].(string)
		catName, _ := tc.Arguments["category_name"].(string)
		catLabel := catName
		if catLabel == "" && catID != "" {
			catLabel = a.categoryDisplayName(scope, catID)
		}
		if catLabel == "" {
			catLabel = "category"
		}
		return "Category deletion staged", []model.PendingEdit{{
			ID: uuid.New().String(), Tool: "delete_category", Input: tc.Arguments,
			Label: "Delete category — " + catLabel, Detail: "Cards will be unpinned to inbox", Status: "pending",
		}}

	// Read-only tools execute even in suggest mode — nothing to stage.
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		return result, nil

	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// tagDisplayName returns a human-friendly name for a project tag, given
// either an ID or a name. Used in pending-edit row labels for the tag tools.
// Falls back to whichever value was provided if lookup fails.
func (a *App) tagDisplayName(scope projectChatScope, tagID, tagName string) string {
	if tagName != "" {
		return tagName
	}
	if tagID == "" {
		return "tag"
	}
	labels, err := a.GetProjectLabels(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return tagID
	}
	for _, l := range labels {
		if l.ID == tagID {
			return l.Name
		}
	}
	return tagID
}

// categoryDisplayName returns the human name of a category by ID, falling back
// to the ID itself if the lookup fails.
func (a *App) categoryDisplayName(scope projectChatScope, catID string) string {
	if catID == "" {
		return "category"
	}
	cats, err := a.repo.ListCategories(scope.brandSlug, scope.streamSlug, scope.projectSlug)
	if err != nil {
		return catID
	}
	for _, c := range cats {
		if c.ID == catID {
			return c.Name
		}
	}
	return catID
}

// shallowCopyArgs builds a new map containing the listed lookup keys from src
// (e.g. tag_id, tag_name for the tag tools) plus a single named field. Used
// when staging multi-field updates so each PendingEdit's Input contains only
// the lookup info plus the one field that edit applies.
func shallowCopyArgs(src map[string]any, lookupKeys []string, field string) map[string]any {
	out := make(map[string]any, len(lookupKeys)+1)
	for _, k := range lookupKeys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	if v, ok := src[field]; ok {
		out[field] = v
	}
	return out
}

// stageProjectCardUpdates expands a single-card update into one PendingEdit
// per field. Each edit is staged with `Tool: "update_card"` (the singular
// executor) regardless of whether the original LLM call was update_card or
// update_cards — see the call site comment for why.
func (a *App) stageProjectCardUpdates(cardID string, args map[string]any) []model.PendingEdit {
	cardLabel := a.cardDisplayLabel(cardID)
	var edits []model.PendingEdit
	mkEdit := func(field string, fieldArg any, detail string) {
		edits = append(edits, model.PendingEdit{
			ID:    uuid.New().String(),
			Tool:  "update_card",
			Input: map[string]any{"card_id": cardID, field: fieldArg},
			Label: cardLabel + " — " + field,
			Detail: detail,
			Status: "pending",
		})
	}
	if v, ok := args["title"].(string); ok && v != "" {
		mkEdit("title", v, v)
	}
	if v, ok := args["card_type"].(string); ok && v != "" {
		mkEdit("card_type", v, v)
	}
	if raw, ok := args["tags"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		detail := "Replace with: " + strings.Join(tags, ", ")
		if len(tags) == 0 {
			detail = "Remove all tags"
		}
		mkEdit("tags", raw, detail)
	}
	if raw, ok := args["tags_to_add"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, "+"+s)
			}
		}
		mkEdit("tags_to_add", raw, strings.Join(tags, ", "))
	}
	if raw, ok := args["tags_to_remove"].([]any); ok {
		var tags []string
		for _, t := range raw {
			if s, ok := t.(string); ok {
				tags = append(tags, "−"+s)
			}
		}
		mkEdit("tags_to_remove", raw, strings.Join(tags, ", "))
	}
	if v, ok := args["due_date"].(string); ok {
		detail := v
		if v == "" {
			detail = "Clear due date"
		}
		mkEdit("due_date", v, detail)
	}
	if v, ok := args["description"].(string); ok {
		mkEdit("description", v, v)
	}
	if raw, ok := args["blocks"].([]any); ok {
		mkEdit("blocks", raw, fmt.Sprintf("Replace with %d blocks", len(raw)))
	}
	return edits
}

// cardDisplayLabel returns a short label for a card (used in pending edit
// labels). Falls back to the card ID if the title can't be loaded.
func (a *App) cardDisplayLabel(cardID string) string {
	if a.repo == nil {
		return cardID
	}
	c, err := a.repo.GetCard(cardID)
	if err != nil || c == nil || c.Title == "" {
		return cardID
	}
	return c.Title
}

// formatStageResult builds a human-readable acknowledgement string for the LLM.
func formatStageResult(toolName string) string {
	return "Edits staged for " + toolName + " — awaiting user approval"
}

// ApplyProjectPendingEdits accepts the specified edits (in order) and rejects
// the rest, for a project chat message. Mirrors ApplyPendingEdits but uses the
// project chat ID and the project tool executor.
//
// `brandSlug`/`streamSlug`/`projectSlug` identify the project whose chat to
// load. `msgID` is the assistant message containing the staged edits. `acceptIDs`
// is the subset of edit IDs to apply; everything else is rejected.
func (a *App) ApplyProjectPendingEdits(brandSlug, streamSlug, projectSlug, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := projectChatID(project.ID)

	acceptSet := make(map[string]bool, len(acceptIDs))
	for _, id := range acceptIDs {
		acceptSet[id] = true
	}

	cf, err := config.LoadChatFor(a.repo.Manifest.ID,chatID)
	if err != nil {
		return nil, err
	}

	// Recompute project scope at apply time. This catches cases where the
	// chat session staged edits referencing cards that no longer belong to the
	// project (moved or deleted between staging and apply), in addition to
	// the original LLM-hallucination defence.
	categories, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	applyScope := projectChatScope{
		brandSlug:   brandSlug,
		streamSlug:  streamSlug,
		projectSlug: projectSlug,
		cardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			applyScope.cardIDs[p.CardID] = true
		}
	}

	// Walk the target message, applying accepted edits in order and marking
	// the rest rejected. Edits run synchronously through the project executor.
	// Failures are stamped into the edit's Detail so the user can hover to see
	// them, and we collect a count to surface as a returned error after save —
	// the frontend uses that to fire a toast.
	var failures int
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.Status != "pending" {
				continue
			}
			if acceptSet[edit.ID] {
				tc := llm.ToolCall{ID: edit.ID, Name: edit.Tool, Arguments: edit.Input}
				result, _ := a.executeProjectToolCall(tc, applyScope)
				if strings.HasPrefix(result, "error:") {
					// Leave it pending so the user can retry; record the error in detail.
					cf.Messages[i].PendingEdits[j].Detail = result
					failures++
					continue
				}
				cf.Messages[i].PendingEdits[j].Status = "accepted"
			} else {
				cf.Messages[i].PendingEdits[j].Status = "rejected"
			}
		}
		break
	}

	if err := config.SaveChatFor(a.repo.Manifest.ID,cf); err != nil {
		return nil, err
	}
	// Failures are surfaced via the per-edit Detail field (which starts with
	// "error:" for failed rows). The frontend scans for those after a refresh
	// and toasts the user. We don't return a Go error here because Wails would
	// drop the cf value, and the user needs to see the updated rows so they
	// can retry the failed ones.
	_ = failures
	return cf, nil
}

// AcceptPendingEdit applies a single pending edit from Suggest mode and marks it accepted.
func (a *App) AcceptPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return nil, err
	}
	card, _ := a.repo.GetCard(cardID)
	allCats, _ := a.ListAllCategories()
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			tc := llm.ToolCall{ID: editID, Name: edit.Tool, Arguments: edit.Input}
			// For suggest_pin, force "auto" so the card is actually pinned
			autoPinMode := ""
			if edit.Tool == "suggest_pin" {
				autoPinMode = "auto"
			}
			result, _, _ := a.executeToolCall(cardID, card, tc, allCats, autoPinMode)
			if strings.HasPrefix(result, "error:") {
				return nil, fmt.Errorf("could not apply edit: %s", result)
			}
			card, _ = a.repo.GetCard(cardID) // refresh for subsequent edits in same batch
			cf.Messages[i].PendingEdits[j].Status = "accepted"
			if err := config.SaveChatFor(a.repo.Manifest.ID,cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// RejectPendingEdit dismisses a single pending edit without applying it.
func (a *App) RejectPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			cf.Messages[i].PendingEdits[j].Status = "rejected"
			if err := config.SaveChatFor(a.repo.Manifest.ID,cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// ApplyPendingEdits accepts the specified edits (in order) and rejects the rest.
// This is the primary batch action for the Suggest mode UI.
func (a *App) ApplyPendingEdits(cardID, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	acceptSet := make(map[string]bool, len(acceptIDs))
	for _, id := range acceptIDs {
		acceptSet[id] = true
	}

	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return nil, err
	}

	// Collect pending IDs in message order, split into accept/reject
	var toAccept, toReject []string
	for _, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for _, e := range m.PendingEdits {
			if e.Status != "pending" {
				continue
			}
			if acceptSet[e.ID] {
				toAccept = append(toAccept, e.ID)
			} else {
				toReject = append(toReject, e.ID)
			}
		}
		break
	}

	var firstErr error
	for _, eid := range toAccept {
		if updated, err2 := a.AcceptPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		} else if firstErr == nil {
			firstErr = err2
		}
	}
	for _, eid := range toReject {
		if updated, err2 := a.RejectPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		}
	}
	return cf, firstErr
}

// AcceptAllPendingEdits applies all pending edits on a message in order.
func (a *App) AcceptAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	// Collect IDs first so we iterate a stable snapshot
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return nil, err
	}
	var pendingIDs []string
	for _, m := range cf.Messages {
		if m.ID == msgID {
			for _, e := range m.PendingEdits {
				if e.Status == "pending" {
					pendingIDs = append(pendingIDs, e.ID)
				}
			}
			break
		}
	}
	for _, eid := range pendingIDs {
		if updated, err := a.AcceptPendingEdit(cardID, msgID, eid); err == nil {
			cf = updated
		}
	}
	return cf, nil
}

// RejectAllPendingEdits dismisses all pending edits on a message.
func (a *App) RejectAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID == msgID {
			for j, e := range m.PendingEdits {
				if e.Status == "pending" {
					cf.Messages[i].PendingEdits[j].Status = "rejected"
				}
			}
			return cf, config.SaveChatFor(a.repo.Manifest.ID,cf)
		}
	}
	return cf, nil
}

// AcceptPinSuggestion accepts a pending pin suggestion on a chat message and performs the pin.
func (a *App) AcceptPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			// Pin convention: projectID == categoryID
			if err := a.PinCard(cardID, m.PinSuggestion.CategoryID, m.PinSuggestion.CategoryID); err != nil {
				return err
			}
			cf.Messages[i].PinSuggestion.Status = "accepted"
			return config.SaveChatFor(a.repo.Manifest.ID,cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}

// RejectPinSuggestion dismisses a pending pin suggestion on a chat message.
func (a *App) RejectPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID,cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			cf.Messages[i].PinSuggestion.Status = "rejected"
			return config.SaveChatFor(a.repo.Manifest.ID,cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}

func (a *App) IsLLMConfigured() bool {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return false
	}
	if cfg.Provider != "" {
		return true
	}
	// Check multi-account setup
	accounts, err := config.LoadLLMAccounts()
	if err != nil {
		return false
	}
	return len(accounts) > 0
}

func (a *App) TestLLMConnection() (string, error) {
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return "", err
	}
	if cfg.Provider == "" {
		return "", fmt.Errorf("no provider configured")
	}
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return "", err
	}
	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
		SystemPrompt: "You are a test. Reply with exactly: OK",
		Messages:     []llm.Message{{Role: "user", Content: "Hello"}},
		Model:        modelName,
	})
	if err != nil {
		return "", err
	}
	return resp.Model, nil
}

// TestSystemNotification sends a test OS notification to verify desktop notifications work.
func (a *App) TestSystemNotification() error {
	return notify.TestSystemNotification()
}

func defaultModelForProvider(provider string) string {
	switch provider {
	case "openai":
		return "gpt-4o"
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "ollama":
		return "llama3"
	default:
		return ""
	}
}

func (a *App) buildSystemPrompt(card *model.Card, cfg config.LLMConfig) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(`You are BRUV AI. You help the user both ORGANISE this card (edit title, tags, fields, pin location, agent config) AND RESEARCH anything relevant to it (look up live information on the web). Today is %s.

FIRST, decide which mode the user's message is in:
  - ORGANISE: they're describing the card's content or asking you to edit it ("this is about X", "add Y", "set due date…", "tag it Z"). In ORGANISE mode, call all applicable card tools at once: type, title, fields, tags, AND pin.
  - RESEARCH: they're asking about something external — current events, prices, news, looking up a URL, explaining why something happened. In RESEARCH mode, call web_search and/or web_fetch, then reply with the answer. Do NOT also call card-organising tools unless the user explicitly asks you to save the findings to the card.
  - CHAT: they're asking a general question that needs no tool. Just reply.
When in doubt between ORGANISE and RESEARCH, prefer RESEARCH — it's less disruptive to call the wrong web tool than to rewrite the card's title/tags.

RULES:
- NEVER call a tool if the value is already correct (check current card state below).
- NEVER call the same tool twice in one response.
- After using tools, briefly describe what you changed or what you found.
- When researching, ALWAYS cite the source URLs returned by web_search in your final reply.
- YOU HAVE WEB ACCESS via web_search and web_fetch. If the user asks about anything you don't already know, CALL those tools. Do NOT say "I can't search the web" or "use a search engine yourself" — those responses are wrong.

TOOLS:
- set_card_type — Pick the best type. Only if type is not set or wrong.
- set_fields — Fill field values with real content from the user's message. ALWAYS call this when fields are empty.
- set_title — Write a clear, specific title. Only if title is "New Card" or generic.
- set_due_date — YYYY-MM-DD format. Resolve relative dates from today (%s).
- suggest_pin — ALWAYS pin the card. STRONGLY prefer an existing category_id from the list below. The hierarchy is: Brand > Stream > Project > Category (e.g. "Big Ideas / YouTube Channels / Channel Brainstorm / Ideas"). Do NOT use the card title as a brand name. Only create new names if NOTHING existing fits.
- add_tags — Add relevant tags. Prefer existing project tags listed below, but you may create new short, descriptive tags if none fit.
- add_field — Add a NEW field to the card (e.g. a checklist, extra notes, a checkbox). Use when the user asks for a field that does not already exist. You can provide an initial value, or use set_fields afterward to populate it.
- configure_agent — Set up or modify the card's autonomous agent. Provide enabled, goal, schedule, and allowed_tools. The agent runs in the background and can fetch web pages, search, notify the user, and update this card. Use this when the user asks to "set up an agent", "run this on a schedule", "check daily", etc.
- web_fetch — Fetch a specific URL and read its text content. Use when the user gives you a link or when you need up-to-date info from a known page.
- web_search — Search the web via DuckDuckGo. Use for "look up…", "find the latest…", "what's happening with…" style asks. Returns titles, URLs, and snippets; follow up with web_fetch on the most relevant result if you need the full content.`, time.Now().Format("2006-01-02 (Monday)"), time.Now().Format("2006-01-02")))

	if cfg.Context != "" {
		parts = append(parts, "User context:\n"+cfg.Context)
	}

	// Available card types with their schemas
	if a.registry != nil {
		typeNames := a.registry.List()
		if len(typeNames) > 0 {
			var typeDescs []string
			for _, tn := range typeNames {
				s := a.registry.Get(tn)
				if s == nil {
					continue
				}
				desc := fmt.Sprintf("- %s: %s", tn, s.Description)
				var fields []string
				for key, prop := range s.Properties {
					f := key
					if prop.Description != "" {
						f += " (" + prop.Description + ")"
					}
					if len(prop.Enum) > 0 {
						f += " [" + strings.Join(prop.Enum, "/") + "]"
					}
					fields = append(fields, f)
				}
				if len(fields) > 0 {
					desc += "\n  Fields: " + strings.Join(fields, ", ")
				}
				typeDescs = append(typeDescs, desc)
			}
			parts = append(parts, "Available card types:\n"+strings.Join(typeDescs, "\n"))
		}
	}

	// Card context (always included)
	var cardParts []string
	cardParts = append(cardParts, fmt.Sprintf("Current card: %q", card.Title))
	if card.Type != "" {
		cardParts = append(cardParts, fmt.Sprintf("Type: %s", card.Type))
	} else {
		cardParts = append(cardParts, "Type: (not set)")
	}
	if len(card.Tags) > 0 {
		cardParts = append(cardParts, fmt.Sprintf("Tags: %s", strings.Join(card.Tags, ", ")))
	}
	if card.DueDate != nil {
		cardParts = append(cardParts, fmt.Sprintf("Due: %s", card.DueDate.Format("2006-01-02")))
	}
	// Include ALL fields — show empty ones so the LLM knows to fill them
	var emptyFields []string
	for _, b := range card.Blocks {
		label := b.Label
		if label == "" {
			label = b.Key
		}
		if label == "" {
			continue
		}
		if v, ok := b.Value.(string); ok && v != "" {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: %s", b.Key, v))
		} else if v, ok := b.Value.(string); ok && v == "" {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: (EMPTY — needs content)", b.Key))
			emptyFields = append(emptyFields, b.Key)
		} else if b.Value != nil {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: %v", b.Key, b.Value))
		}
	}
	if len(emptyFields) > 0 {
		cardParts = append(cardParts, fmt.Sprintf(">>> EMPTY FIELDS that need content: %s — use set_fields to fill them!", strings.Join(emptyFields, ", ")))
	}
	parts = append(parts, strings.Join(cardParts, "\n"))

	// Agent context — show current agent config if it exists
	if a.repo != nil {
		af, err := a.repo.GetAgentConfig(card.ID)
		if err == nil && af.Config.Enabled {
			agentParts := []string{"Agent: ENABLED"}
			agentParts = append(agentParts, fmt.Sprintf("  Goal: %s", af.Config.Goal))
			agentParts = append(agentParts, fmt.Sprintf("  Schedule: %s", af.Config.Schedule))
			agentParts = append(agentParts, fmt.Sprintf("  Status: %s", af.Config.Status))
			agentParts = append(agentParts, fmt.Sprintf("  Tools: %s", strings.Join(af.Config.AllowedTools, ", ")))
			if af.Config.LastRunAt != nil {
				agentParts = append(agentParts, fmt.Sprintf("  Last run: %s", af.Config.LastRunAt.Format("2006-01-02 15:04")))
			}
			parts = append(parts, strings.Join(agentParts, "\n"))
		} else {
			parts = append(parts, "Agent: not configured. Use configure_agent to set up an autonomous agent on this card.")
		}
	}

	// Hierarchy context based on ContextLevel
	level := card.ContextLevel
	if level == "" {
		level = model.ContextProject
	}
	if level == model.ContextIsolated || a.repo == nil {
		return strings.Join(parts, "\n\n")
	}

	// Repository description
	if a.repo.Manifest.Description != "" {
		parts = append(parts, "Repository: "+a.repo.Manifest.Name+"\n"+a.repo.Manifest.Description)
	}

	// Build hierarchy descriptions (deduplicated — each entity listed once)
	allCats, _ := a.ListAllCategories()
	if len(allCats) > 0 {
		seenBrands := map[string]bool{}
		seenStreams := map[string]bool{}
		seenProjects := map[string]bool{}
		var hierarchy []string
		for _, c := range allCats {
			if !seenBrands[c.BrandName] {
				seenBrands[c.BrandName] = true
				if c.BrandDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("Brand %q — %s", c.BrandName, c.BrandDescription))
				}
			}
			streamKey := c.BrandName + "/" + c.StreamName
			if !seenStreams[streamKey] {
				seenStreams[streamKey] = true
				if c.StreamDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("  Stream %q — %s", c.StreamName, c.StreamDescription))
				}
			}
			projectKey := streamKey + "/" + c.ProjectName
			if !seenProjects[projectKey] {
				seenProjects[projectKey] = true
				if c.ProjectDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("    Project %q — %s", c.ProjectName, c.ProjectDescription))
				}
			}
			if c.CategoryDescription != "" {
				hierarchy = append(hierarchy, fmt.Sprintf("      Category %q — %s", c.CategoryName, c.CategoryDescription))
			}
		}
		if len(hierarchy) > 0 {
			parts = append(parts, "Hierarchy descriptions:\n"+strings.Join(hierarchy, "\n"))
		}

		// Category listing for pin suggestions (compact — no repeated descriptions)
		var catDescs []string
		for _, c := range allCats {
			desc := fmt.Sprintf("- %s (id: %s)", c.Breadcrumb, c.CategoryID)
			if len(c.AcceptedTypes) > 0 {
				desc += " [accepts: " + strings.Join(c.AcceptedTypes, ", ") + "]"
			}
			catDescs = append(catDescs, desc)
		}
		parts = append(parts, "Available categories for pinning (PREFER these — only create new if none fit):\n"+strings.Join(catDescs, "\n"))
	} else {
		parts = append(parts, "No categories exist yet. Use suggest_pin with brand/stream/project/category names to create a new location.")
	}

	// Collect existing project tags so the AI prefers them over inventing new ones
	projectTags := a.collectProjectTags(allCats)
	if len(projectTags) > 0 {
		parts = append(parts, "Existing tags (prefer these, or create short new ones if none fit):\n"+strings.Join(projectTags, "\n"))
	}

	// Brand instructions for pinned cards
	pins, _ := a.repo.GetCardPins(card.ID)
	if len(pins) > 0 && (level == model.ContextBrand || level == model.ContextGlobal) {
		loc, err := a.GetCardLocation(card.ID)
		if err == nil {
			brand, err := a.repo.GetBrand(loc.BrandSlug)
			if err == nil && brand.SystemPrompt != "" {
				parts = append(parts, "Brand instructions:\n"+brand.SystemPrompt)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// collectProjectTags gathers all unique tags from all projects, grouped by project.
func (a *App) collectProjectTags(allCats []CategoryPath) []string {
	if a.repo == nil {
		return nil
	}
	// Deduplicate projects (multiple categories share the same project)
	type projectKey struct{ brand, stream, project string }
	seen := make(map[projectKey]bool)
	var results []string

	for _, c := range allCats {
		pk := projectKey{c.BrandSlug, c.StreamSlug, c.ProjectSlug}
		if seen[pk] {
			continue
		}
		seen[pk] = true
		labels, err := a.repo.GetProjectLabels(c.BrandSlug, c.StreamSlug, c.ProjectSlug)
		if err != nil || len(labels) == 0 {
			continue
		}
		var tagNames []string
		for _, l := range labels {
			tagNames = append(tagNames, l.Name)
		}
		results = append(results, fmt.Sprintf("- %s / %s / %s: %s", c.BrandName, c.StreamName, c.ProjectName, strings.Join(tagNames, ", ")))
	}
	return results
}

// GetTokenPricing returns the current token pricing configuration.
func (a *App) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return config.LoadCustomPricing()
}

// SaveTokenPricing saves custom token pricing overrides.
func (a *App) SaveTokenPricing(pricing map[string]config.ModelPricing) error {
	return config.SaveCustomPricing(pricing)
}

// --- Card Comments ---

// ListCardComments returns all comments attached to a card in chronological order.
func (a *App) ListCardComments(cardID string) ([]model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := a.repo.LoadComments(cardID)
	if err != nil {
		return nil, err
	}
	return cf.Comments, nil
}

// AddCardComment appends a new comment to a card. The author defaults to the
// current profile display name when empty.
func (a *App) AddCardComment(cardID, author, text string) (*model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	text = strings.TrimSpace(repo.SanitizeText(text))
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	if author == "" {
		if profile, err := config.LoadProfile(); err == nil && profile.DisplayName != "" {
			author = profile.DisplayName
		} else {
			author = "You"
		}
	}
	comment, err := a.repo.AddCardComment(cardID, author, text, time.Time{})
	if err != nil {
		return nil, err
	}
	a.logActivity(cardID, "commented", "")
	return comment, nil
}

// UpdateCardComment edits an existing comment's text.
func (a *App) UpdateCardComment(cardID, commentID, text string) (*model.Comment, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	text = strings.TrimSpace(repo.SanitizeText(text))
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	return a.repo.UpdateCardComment(cardID, commentID, text)
}

// DeleteCardComment removes a comment by ID.
func (a *App) DeleteCardComment(cardID, commentID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteCardComment(cardID, commentID)
}

// --- Trello Import ---

// ImportTrelloBoard reads a Trello JSON export from disk and creates a new
// project under the given brand/stream. archiveMode is one of
// "skip" | "archive" | "inline" — see importer.ArchiveMode.
func (a *App) ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode string) (*importer.Result, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return a.importTrelloBytes(brandSlug, streamSlug, data, archiveMode)
}

// ImportTrelloBoardFromJSON accepts a Trello JSON export as a string payload
// (useful when the frontend drops a file via FileReader and never has access
// to the original path).
func (a *App) ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode string) (*importer.Result, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.importTrelloBytes(brandSlug, streamSlug, []byte(jsonContent), archiveMode)
}

func (a *App) importTrelloBytes(brandSlug, streamSlug string, data []byte, archiveMode string) (*importer.Result, error) {
	parsed, err := importer.ParseTrelloJSON(data)
	if err != nil {
		return nil, err
	}

	mode := importer.ArchiveSeparate
	switch strings.ToLower(archiveMode) {
	case "skip":
		mode = importer.ArchiveSkip
	case "inline":
		mode = importer.ArchiveInline
	case "", "archive", "separate":
		mode = importer.ArchiveSeparate
	}

	result, err := importer.ImportTrello(a.repo, brandSlug, streamSlug, parsed, importer.Options{Archive: mode})
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		a.idxIncrementalRefresh()
	}
	return result, nil
}

// --- Project Export ---

// ExportProjectToFile writes a project export to the given absolute path.
// Returns the byte count of the written file on success.
func (a *App) ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath string) (int, error) {
	if a.repo == nil {
		return 0, fmt.Errorf("no repository open")
	}
	data, err := importer.ExportProject(a.repo, brandSlug, streamSlug, projectSlug)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return 0, fmt.Errorf("write export: %w", err)
	}
	return len(data), nil
}
