package main

import (
	"bruv/internal/agent"
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

	// Pre-v1.0a portability migration: seed the repo-scoped card types
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


// coerceBlockValue converts an LLM-provided value into the format expected
// by the given block type. LLMs may ignore the schema and send strings for
// booleans/numbers, or flat text for checklists/lists, so we handle all of
// that here. The goal is to keep the block renderable no matter what shape
// the model sent, because LLMs are inconsistent and a corrupted block is a
// silent failure the user only notices when they look at the card.
//
// This variant knows only the block type. For constraint checks that need
// block.Meta (select options, rating max, etc.), use coerceBlockValueForBlock.
