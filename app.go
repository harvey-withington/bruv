package main

import (
	"bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/notify"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "0.1.0-dev"

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
	trayPauseItem interface{ Check(); Uncheck(); Checked() bool } // system tray "Pause Agents" menu item
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load the card type schema registry
	reg, err := schema.NewRegistry()
	if err != nil {
		log.Printf("warning: failed to load card type schemas: %v", err)
	}
	a.registry = reg

	// Migrate legacy single-provider config to multi-account
	_ = config.MigrateLegacyLLMConfig()
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
		return false // allow quit
	}

	// Hide window instead of closing — agents keep running in background
	wailsRuntime.WindowHide(ctx)
	return true // prevent close
}

// Version returns the current application version
func (a *App) Version() string {
	return AppVersion
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
		log.Printf("warning: failed to open index: %v\n", err)
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

	// Revalidate repo data (remove stale pins, orphaned files, etc.)
	if repairStats, err := r.Revalidate(); err != nil {
		log.Printf("warning: revalidation failed: %v\n", err)
	} else {
		log.Printf("revalidate: %s\n", repairStats)
	}

	// Open the SQLite index and do an incremental refresh
	if err := a.openIndex(path); err != nil {
		log.Printf("warning: failed to open index: %v\n", err)
	} else if a.idx != nil {
		if _, err := a.idx.IncrementalRefresh(path); err != nil {
			log.Printf("warning: index refresh failed: %v\n", err)
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

	return nil
}

// HasRepository returns true if a repository is currently open.
func (a *App) HasRepository() bool {
	return a.repo != nil
}

// CloseRepository closes the current repository and its index.
func (a *App) CloseRepository() {
	a.stopScheduler()
	a.stopDueDateScanner()
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

// --- Brand ---

func (a *App) CreateBrand(name string) (*model.Brand, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateBrand(name)
}

func (a *App) GetBrand(slug string) (*model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetBrand(slug)
}

func (a *App) ListBrands() ([]model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListBrands()
}

func (a *App) RenameBrand(slug, newName string) (*model.Brand, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	brand, err := a.repo.RenameBrand(slug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return brand, nil
}

func (a *App) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateBrandDescription(slug, description)
}

func (a *App) UpdateBrandIcon(slug, icon string) (*model.Brand, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateBrandIcon(slug, icon)
}

func (a *App) DeleteBrand(slug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Unpin cards from all streams/projects/categories in this brand
	streams, _ := a.repo.ListStreams(slug)
	for _, stream := range streams {
		projects, _ := a.repo.ListProjects(slug, stream.Slug)
		for _, proj := range projects {
			cats, _ := a.repo.ListCategories(slug, stream.Slug, proj.Slug)
			for _, cat := range cats {
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				for _, p := range pins {
					_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
				}
			}
		}
	}
	err := a.repo.DeleteBrand(slug)
	if err == nil && a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return err
}

// --- Stream ---

func (a *App) CreateStream(brandSlug, name string) (*model.Stream, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateStream(brandSlug, name)
}

func (a *App) ListStreams(brandSlug string) ([]model.Stream, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListStreams(brandSlug)
}

func (a *App) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	stream, err := a.repo.RenameStream(brandSlug, streamSlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return stream, nil
}

func (a *App) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateStreamDescription(brandSlug, streamSlug, description)
}

func (a *App) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateStreamIcon(brandSlug, streamSlug, icon)
}

func (a *App) DeleteStream(brandSlug, streamSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Unpin cards from all projects/categories in this stream
	projects, _ := a.repo.ListProjects(brandSlug, streamSlug)
	for _, proj := range projects {
		cats, _ := a.repo.ListCategories(brandSlug, streamSlug, proj.Slug)
		for _, cat := range cats {
			pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
			for _, p := range pins {
				_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
			}
		}
	}
	err := a.repo.DeleteStream(brandSlug, streamSlug)
	if err == nil && a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return err
}

// --- Project ---

func (a *App) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.CreateProject(brandSlug, streamSlug, name)
	if err != nil {
		return nil, err
	}
	// Auto-create a default category so the project is immediately usable for pinning
	prefs, _ := config.LoadPreferences()
	catName := prefs.DefaultCategoryName
	if catName == "" {
		catName = "Ideas"
	}
	a.repo.CreateCategory(brandSlug, streamSlug, project.Slug, catName, 0)
	return project, nil
}

func (a *App) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListProjects(brandSlug, streamSlug)
}

func (a *App) RenameProject(brandSlug, streamSlug, projectSlug, newName string) (*model.Project, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.RenameProject(brandSlug, streamSlug, projectSlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return project, nil
}

func (a *App) UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description)
}

func (a *App) UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon string) (*model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
}

func (a *App) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}

	// Before deleting, unpin all cards from this project's categories
	// so they become orphaned (appear in inbox) instead of silently deleted.
	cats, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	for _, cat := range cats {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
		}
	}

	err := a.repo.DeleteProject(brandSlug, streamSlug, projectSlug)
	if err == nil && a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return err
}

// --- Category ---

func (a *App) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
}

func (a *App) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
}

func (a *App) RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName string) (*model.Category, error) {
	newName = repo.SanitizeText(newName)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cat, err := a.repo.RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName)
	if err != nil {
		return nil, err
	}
	if a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return cat, nil
}

func (a *App) UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description string) (*model.Category, error) {
	description = repo.SanitizeText(description)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description)
}

func (a *App) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	// Prevent deleting the last category in a project
	cats, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	if len(cats) <= 1 {
		return fmt.Errorf("cannot delete the last category in a project")
	}

	// Unpin cards from this category so they become orphaned (inbox) instead of invisible
	cat, err := a.repo.GetCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			_ = a.repo.UnpinCard(p.CardID, p.ProjectID, p.CategoryID)
		}
	}

	err = a.repo.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
	if err == nil && a.idx != nil {
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
	}
	return err
}

// UpdateCategoryAcceptedTypes sets which card types a category will accept.
// An empty or nil slice clears the restriction (all types accepted).
func (a *App) UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug string, acceptedTypes []string) (*model.Category, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateCategory(brandSlug, streamSlug, projectSlug, categorySlug, func(c *model.Category) {
		c.AcceptedTypes = acceptedTypes
	})
}

// MoveCategoryCards moves all card pins from one category to another, then deletes the source category.
func (a *App) MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	if a.idx == nil {
		return fmt.Errorf("no index available")
	}

	// Get all card IDs in the source category
	cardIDs, err := a.idx.ListCardIDsInCategory(fromCategoryID, fromCategoryID)
	if err != nil {
		return fmt.Errorf("list cards in category: %w", err)
	}

	// Move each card's pin to the target category
	for i, cardID := range cardIDs {
		if err := a.repo.MoveCardToCategory(cardID, fromCategoryID, fromCategoryID, toCategoryID, i); err != nil {
			return fmt.Errorf("move card %s: %w", cardID, err)
		}
		// Re-index pins
		if pins, err := a.repo.GetCardPins(cardID); err == nil {
			_ = a.idx.IndexPins(cardID, pins)
		}
	}

	return nil
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
		_ = a.idx.IndexCard(card, time.Now(), "")
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
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
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
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
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
	if a.idx != nil {
		_ = a.idx.RemoveCard(id)
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
			_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
			_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
			_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
			_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
			_ = a.idx.IndexPins(cardID, pins)
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
	type hierKey struct{ brand, stream, project string }
	catToHier := make(map[string]hierKey)
	brands, _ := a.repo.ListBrands()
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					catToHier[c.ID] = hierKey{b.Slug, s.Slug, p.Slug}
				}
			}
		}
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
			_ = a.idx.IndexPins(cardID, pins)
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
func (a *App) ListAllCategories() ([]CategoryPath, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	var results []CategoryPath
	brands, err := a.repo.ListBrands()
	if err != nil {
		return nil, err
	}
	for _, b := range brands {
		streams, _ := a.repo.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := a.repo.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := a.repo.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					results = append(results, CategoryPath{
						BrandSlug:           b.Slug,
						StreamSlug:          s.Slug,
						ProjectSlug:         p.Slug,
						CategorySlug:        c.Slug,
						BrandName:           b.Name,
						StreamName:          s.Name,
						ProjectName:         p.Name,
						CategoryName:        c.Name,
						BrandDescription:    b.Description,
						StreamDescription:   s.Description,
						ProjectDescription:  p.Description,
						CategoryDescription: c.Description,
						ProjectID:           p.ID,
						CategoryID:          c.ID,
						Breadcrumb:          b.Name + " / " + s.Name + " / " + p.Name + " / " + c.Name,
						AcceptedTypes:       c.AcceptedTypes,
					})
				}
			}
		}
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
			_ = a.idx.IndexPins(cardID, pins)
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
			_ = a.idx.IndexPins(cardID, pins)
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
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
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
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
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
		_, _ = a.idx.IncrementalRefresh(a.repo.Root)
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

// UpdateCardLabels replaces a card's label IDs.
func (a *App) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Labels = labelIDs
	})
	if err == nil && a.idx != nil {
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
func (a *App) ListCardTypes() []CardTypeInfo {
	store, _ := config.LoadUserTypeStore()
	dirty := a.ensureSeeded(&store)
	dirty = a.ensureStarterTemplates(&store) || dirty
	dirty = a.ensureMissingBuiltinTemplates(&store) || dirty
	if dirty {
		_ = config.SaveUserTypeStore(store)
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
func (a *App) resolveTemplateBlocks(cardType string) []model.Block {
	store, _ := config.LoadUserTypeStore()

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
	store, err := config.LoadUserTypeStore()
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
	return t, config.SaveUserTypeStore(store)
}

// UpdateUserCardType updates an existing user-defined card type by ID.
func (a *App) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	store, err := config.LoadUserTypeStore()
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
			return store.Types[i], config.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// UpdateUserCardTypeIcon sets or clears the icon on a user-defined card type.
func (a *App) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	store, err := config.LoadUserTypeStore()
	if err != nil {
		return config.UserCardType{}, err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types[i].Icon = icon
			return store.Types[i], config.SaveUserTypeStore(store)
		}
	}
	return config.UserCardType{}, fmt.Errorf("card type %q not found", id)
}

// DeleteUserCardType removes a user-defined card type by ID.
func (a *App) DeleteUserCardType(id string) error {
	store, err := config.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, t := range store.Types {
		if t.ID == id {
			store.Types = append(store.Types[:i], store.Types[i+1:]...)
			return config.SaveUserTypeStore(store)
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
	store, err := config.LoadUserTypeStore()
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
	return config.SaveUserTypeStore(store)
}

// ListCardTemplates returns all user-defined card templates.
func (a *App) ListCardTemplates() ([]config.CardTemplate, error) {
	store, err := config.LoadUserTypeStore()
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
	store, err := config.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	tmpl := config.CardTemplate{
		ID:     uuid.New().String(),
		Name:   name,
		Blocks: blocks,
	}
	store.Templates = append(store.Templates, tmpl)
	return tmpl, config.SaveUserTypeStore(store)
}

// UpdateCardTemplate updates an existing card template by ID.
func (a *App) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	store, err := config.LoadUserTypeStore()
	if err != nil {
		return config.CardTemplate{}, err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates[i].Name = name
			store.Templates[i].Blocks = blocks
			return store.Templates[i], config.SaveUserTypeStore(store)
		}
	}
	return config.CardTemplate{}, fmt.Errorf("template %q not found", id)
}

// DeleteCardTemplate removes a card template by ID.
func (a *App) DeleteCardTemplate(id string) error {
	store, err := config.LoadUserTypeStore()
	if err != nil {
		return err
	}
	for i, tmpl := range store.Templates {
		if tmpl.ID == id {
			store.Templates = append(store.Templates[:i], store.Templates[i+1:]...)
			return config.SaveUserTypeStore(store)
		}
	}
	return fmt.Errorf("template %q not found", id)
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
		_ = a.idx.UpdateAgentIndex(cardID, cfg.Enabled, string(cfg.Status), nextRun)
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
	return a.repo.LoadChat(cardID)
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
	return a.repo.LoadChat(projectChatID(project.ID))
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
	return a.repo.SaveChat(&model.ChatFile{CardID: chatID, Messages: []model.ChatMessage{}})
}

// ClearCardChatHistory deletes all messages in a card's AI chat.
func (a *App) ClearCardChatHistory(cardID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.SaveChat(&model.ChatFile{CardID: cardID, Messages: []model.ChatMessage{}})
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
			cf, _ = a.repo.AppendMessage(lc.chatID, errMsg)
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
			cf, _ = a.repo.AppendMessage(lc.chatID, budgetMsg)
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
			cf, _ = a.repo.AppendMessage(lc.chatID, assistantMsg)
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
	cf, _ = a.repo.AppendMessage(lc.chatID, assistantMsg)
	if lc.totalTokensUsed != nil {
		*lc.totalTokensUsed = cumulativeTokens
	}
	return cf, nil
}

// --- Project chat ---

func (a *App) SendProjectChatMessage(brandSlug, streamSlug, projectSlug, userMessage string) (*model.ChatFile, error) {
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
	systemPrompt := a.buildProjectSystemPrompt(brand, stream, project, categories, cfg)

	// Build tool definitions
	var catMaps []map[string]string
	for _, cat := range categories {
		catMaps = append(catMaps, map[string]string{"id": cat.ID, "breadcrumb": cat.Name})
	}
	cardTypes := a.listCardTypeIDs()
	toolDefs := llm.ProjectTools(cardTypes, catMaps)

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	return a.runChatLoop(ctx, provider, modelName, cf, chatLoopConfig{
		chatID:             chatID,
		systemPrompt:       systemPrompt,
		tools:              toolDefs,
		maxIter:            5,
		allowDuplicateTool: map[string]bool{"create_card": true},
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := a.executeProjectToolCall(tc)
			return result, action, nil
		},
		fallbackContent: "I've made the requested changes to your project.",
	})
}

// buildProjectSystemPrompt builds the system prompt for project-level chat.
func (a *App) buildProjectSystemPrompt(brand *model.Brand, stream *model.Stream, project *model.Project, categories []model.Category, cfg config.LLMConfig) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are BRUV AI, a project assistant. Today is %s.\n\n", time.Now().Format("2006-01-02 (Monday)")))

	// Hierarchy context
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
	sb.WriteString("\n")

	if cfg.Context != "" {
		sb.WriteString("## User context\n" + cfg.Context + "\n\n")
	}

	// Enumerate all cards grouped by category
	totalCards := 0
	seenCards := make(map[string]bool)
	if len(categories) > 0 {
		sb.WriteString("## Categories and cards\n\n")
		for _, cat := range categories {
			sb.WriteString(fmt.Sprintf("### %s (category_id: %s)\n", cat.Name, cat.ID))
			if cat.Description != "" {
				sb.WriteString(cat.Description + "\n")
			}
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
				line := fmt.Sprintf("- **%s** (id: `%s`, type: %s", card.Title, card.ID, card.Type)
				if len(card.Tags) > 0 {
					line += ", tags: " + strings.Join(card.Tags, ", ")
				}
				if card.DueDate != nil {
					line += ", due: " + card.DueDate.Format("2006-01-02")
				}
				line += ")"
				// Include description from intrinsic fields
				if desc, ok := card.Fields["description"].(string); ok && desc != "" {
					if len(desc) > 200 {
						desc = desc[:200] + "…"
					}
					line += "\n  > " + strings.ReplaceAll(desc, "\n", "\n  > ")
				}
				// Include first text block content if no description field
				if _, hasDesc := card.Fields["description"]; !hasDesc {
					for _, b := range card.Blocks {
						if b.Type == "text" {
							if s, ok := b.Value.(string); ok && s != "" {
								if len(s) > 200 {
									s = s[:200] + "…"
								}
								line += "\n  > " + strings.ReplaceAll(s, "\n", "\n  > ")
							}
							break
						}
					}
				}
				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("This project has no categories yet.\n\n")
	}

	if totalCards == 0 && len(categories) > 0 {
		sb.WriteString("All categories are empty — no cards yet.\n\n")
	}

	sb.WriteString("## Your capabilities\n")
	sb.WriteString("You can answer questions about the project's cards and structure. ")
	sb.WriteString("You have tools to **create cards**, **add tags to cards**, **move cards between categories**, and **update cards**. ")
	sb.WriteString("When the user asks you to create a card or make changes, USE THE TOOLS — do not just describe what to do. ")
	sb.WriteString("When creating cards, always pin them to the most appropriate category.\n")
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
		tools := llm.CardTools(cardTypes, catMaps)
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

	// In chat-only mode skip all tools
	var toolDefs []llm.ToolDef
	if cfg.AIMode != "chat" {
		toolDefs = buildToolDefs(card)
	}

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
		maxIter:      3,
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
		fallbackContent: "I've made the changes to your card.",
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
	return a.repo.AppendMessage(chatID, userMsg)
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

// coerceBlockValue converts an LLM-provided value into the format expected by
// the given block type. LLMs may ignore the schema and send strings for
// booleans/numbers, or flat text for checklists, so we handle all of that here.
func coerceBlockValue(blockType string, val any) any {
	switch blockType {
	case model.BlockChecklist:
		return coerceChecklist(val)
	case model.BlockCheckbox:
		return coerceCheckbox(val)
	case model.BlockNumber:
		return coerceNumber(val)
	default:
		// text, select, radio, date, url, image, video — all string, pass through
		return val
	}
}

// coerceChecklist converts []any of strings or a single string into [{id, text, done}].
func coerceChecklist(val any) []map[string]any {
	var texts []string
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				texts = append(texts, s)
			}
		}
	case string:
		for _, line := range strings.Split(v, "\n") {
			line = strings.TrimSpace(line)
			// Strip leading numbering like "1. " or "- "
			line = strings.TrimLeft(line, "0123456789.")
			line = strings.TrimPrefix(line, "-")
			line = strings.TrimSpace(line)
			if line != "" {
				texts = append(texts, line)
			}
		}
	}
	items := make([]map[string]any, len(texts))
	for i, t := range texts {
		items[i] = map[string]any{
			"id":   fmt.Sprintf("cli-%s", uuid.New().String()[:8]),
			"text": t,
			"done": false,
		}
	}
	return items
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

		summary := fmt.Sprintf("Agent %s — schedule: %s, tools: %s", map[bool]string{true: "enabled", false: "disabled"}[enabled], schedule, strings.Join(allowedTools, ", "))
		action := &model.ToolAction{Tool: "configure_agent", Input: tc.Arguments, Result: summary}
		return summary, action, nil

	default:
		return "error: unknown tool " + tc.Name, nil, nil
	}
}

// executeProjectToolCall runs a single project-level tool and returns (result, action).
func (a *App) executeProjectToolCall(tc llm.ToolCall) (string, *model.ToolAction) {
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
		// Pin to category if specified
		categoryID, _ := tc.Arguments["category_id"].(string)
		if categoryID != "" {
			_ = a.PinCard(cardID, categoryID, categoryID)
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
		fromCat, _ := tc.Arguments["from_category_id"].(string)
		toCat, _ := tc.Arguments["to_category_id"].(string)
		if cardID == "" || fromCat == "" || toCat == "" {
			return "error: card_id, from_category_id, and to_category_id are required", nil
		}
		err := a.MoveCardToCategory(cardID, fromCat, fromCat, toCat, 0)
		if err != nil {
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
		var changes []string
		if title, ok := tc.Arguments["title"].(string); ok && title != "" {
			if _, err := a.UpdateCardTitle(cardID, title); err == nil {
				changes = append(changes, "title")
			}
		}
		if cardType, ok := tc.Arguments["card_type"].(string); ok && cardType != "" {
			if _, err := a.UpdateCardType(cardID, cardType); err == nil {
				changes = append(changes, "type")
			}
		}
		if tagsRaw, ok := tc.Arguments["tags"].([]any); ok && len(tagsRaw) > 0 {
			c, err := a.repo.GetCard(cardID)
			if err == nil {
				existing := make(map[string]bool)
				for _, t := range c.Tags {
					existing[strings.ToLower(t)] = true
				}
				merged := c.Tags
				for _, raw := range tagsRaw {
					if s, ok := raw.(string); ok && s != "" && !existing[strings.ToLower(s)] {
						merged = append(merged, s)
						existing[strings.ToLower(s)] = true
					}
				}
				a.UpdateCardTags(cardID, merged)
				changes = append(changes, "tags")
			}
		}
		if len(changes) == 0 {
			return "No changes applied", nil
		}
		result := "Updated: " + strings.Join(changes, ", ")
		action := &model.ToolAction{Tool: "update_card", Input: tc.Arguments, Result: result}
		return result, action

	default:
		return "error: unknown tool " + tc.Name, nil
	}
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
		store, _ := config.LoadUserTypeStore()
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

	default:
		return "Staged unknown tool " + tc.Name, nil
	}
}

// AcceptPendingEdit applies a single pending edit from Suggest mode and marks it accepted.
func (a *App) AcceptPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := a.repo.LoadChat(cardID)
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
			if err := a.repo.SaveChat(cf); err != nil {
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
	cf, err := a.repo.LoadChat(cardID)
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
			if err := a.repo.SaveChat(cf); err != nil {
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

	cf, err := a.repo.LoadChat(cardID)
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
	cf, err := a.repo.LoadChat(cardID)
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
	cf, err := a.repo.LoadChat(cardID)
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
			return cf, a.repo.SaveChat(cf)
		}
	}
	return cf, nil
}

// AcceptPinSuggestion accepts a pending pin suggestion on a chat message and performs the pin.
func (a *App) AcceptPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := a.repo.LoadChat(cardID)
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
			return a.repo.SaveChat(cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}

// RejectPinSuggestion dismisses a pending pin suggestion on a chat message.
func (a *App) RejectPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := a.repo.LoadChat(cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			cf.Messages[i].PinSuggestion.Status = "rejected"
			return a.repo.SaveChat(cf)
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

	parts = append(parts, fmt.Sprintf(`You are BRUV AI, a card organizer. Today is %s.

RULES:
- On the FIRST message, call ALL applicable tools at once: type, title, fields, tags, AND pin. Do not wait for follow-up messages.
- NEVER call a tool if the value is already correct (check current card state below).
- NEVER call the same tool twice in one response.
- After using tools, briefly describe what you changed.

TOOLS:
- set_card_type — Pick the best type. Only if type is not set or wrong.
- set_fields — Fill field values with real content from the user's message. ALWAYS call this when fields are empty.
- set_title — Write a clear, specific title. Only if title is "New Card" or generic.
- set_due_date — YYYY-MM-DD format. Resolve relative dates from today (%s).
- suggest_pin — ALWAYS pin the card. STRONGLY prefer an existing category_id from the list below. The hierarchy is: Brand > Stream > Project > Category (e.g. "Big Ideas / YouTube Channels / Channel Brainstorm / Ideas"). Do NOT use the card title as a brand name. Only create new names if NOTHING existing fits.
- add_tags — Add relevant tags. Prefer existing project tags listed below, but you may create new short, descriptive tags if none fit.
- add_field — Add a NEW field to the card (e.g. a checklist, extra notes, a checkbox). Use when the user asks for a field that does not already exist. You can provide an initial value, or use set_fields afterward to populate it.
- configure_agent — Set up or modify the card's autonomous agent. Provide enabled, goal, schedule, and allowed_tools. The agent runs in the background and can fetch web pages, search, notify the user, and update this card. Use this when the user asks to "set up an agent", "run this on a schedule", "check daily", etc.`, time.Now().Format("2006-01-02 (Monday)"), time.Now().Format("2006-01-02")))

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
