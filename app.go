package main

import (
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "0.1.0-dev"

// App struct
type App struct {
	ctx            context.Context
	repo           *repo.Repository
	registry       *schema.Registry
	idx            *index.Index
	savedBounds    *config.WindowBounds
	boundsRestored bool
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
		fmt.Printf("warning: failed to load card type schemas: %v\n", err)
	}
	a.registry = reg
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
}

// beforeClose is called when the window is about to close.
// We save the current window position and size.
func (a *App) beforeClose(ctx context.Context) bool {
	maximised := wailsRuntime.WindowIsMaximised(ctx)

	// If maximised, un-maximise briefly to get the restored position
	if maximised {
		wailsRuntime.WindowUnmaximise(ctx)
	}

	x, y := wailsRuntime.WindowGetPosition(ctx)
	w, h := wailsRuntime.WindowGetSize(ctx)

	wb := &config.WindowBounds{
		X:         x,
		Y:         y,
		Width:     w,
		Height:    h,
		Maximised: maximised,
	}
	_ = config.SaveWindowBounds(wb)

	return false // allow close
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
		fmt.Printf("warning: failed to open index: %v\n", err)
	}

	// Add to recent repos
	_ = config.AddRecent(r.Root, name)

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
		fmt.Printf("warning: revalidation failed: %v\n", err)
	} else {
		fmt.Printf("revalidate: %s\n", repairStats)
	}

	// Open the SQLite index and do an incremental refresh
	if err := a.openIndex(path); err != nil {
		fmt.Printf("warning: failed to open index: %v\n", err)
	} else if a.idx != nil {
		if _, err := a.idx.IncrementalRefresh(path); err != nil {
			fmt.Printf("warning: index refresh failed: %v\n", err)
		}
	}

	// Add to recent repos
	_ = config.AddRecent(path, r.Manifest.Name)

	return nil
}

// HasRepository returns true if a repository is currently open.
func (a *App) HasRepository() bool {
	return a.repo != nil
}

// CloseRepository closes the current repository and its index.
func (a *App) CloseRepository() {
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
	err := a.repo.DeleteCard(id)
	if err != nil {
		return err
	}
	if a.idx != nil {
		_ = a.idx.RemoveCard(id)
	}
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
	if err == nil && a.idx != nil {
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
	}
	return card, err
}

// UpdateCardType sets the type on a card (e.g. "task", "feature", or "" for none).
func (a *App) UpdateCardType(id, cardType string) (*model.Card, error) {
	cardType = repo.SanitizeText(cardType)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Type = cardType
	})
	if err == nil && a.idx != nil {
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
	}
	return card, err
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
	if err == nil && a.idx != nil {
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
	}
	// Sync new tags to all projects the card is pinned to
	if err == nil {
		a.syncTagsToAllPinnedProjects(id)
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
	if err == nil && a.idx != nil {
		_ = a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID))
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
			a.repo.AddProjectLabel(brandSlug, streamSlug, projectSlug, tag, "")
		}
	}
}

func (a *App) UnpinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
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

func (a *App) ListCardTypes() []string {
	if a.registry == nil {
		return nil
	}
	return a.registry.List()
}

func (a *App) ValidateCardFields(cardType string, fields map[string]any) []string {
	if a.registry == nil {
		return []string{"schema registry not loaded"}
	}
	return a.registry.Validate(cardType, fields)
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

// --- Chat ---

func (a *App) LoadChatHistory(cardID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.LoadChat(cardID)
}

func (a *App) SendChatMessage(cardID, userMessage string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// 1. Save user message immediately
	userMsg := model.ChatMessage{
		ID:        uuid.New().String(),
		Role:      model.RoleUser,
		Content:   repo.SanitizeText(userMessage),
		Timestamp: time.Now().UTC(),
	}
	cf, err := a.repo.AppendMessage(cardID, userMsg)
	if err != nil {
		return nil, err
	}

	// 2. Load LLM config — if not configured, return with just user message
	cfg, err := config.LoadLLMConfig()
	if err != nil || cfg.Provider == "" {
		return cf, nil
	}

	// 3. Create provider
	provider, err := llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return cf, nil
	}

	// 4. Load card for context
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return cf, nil
	}

	// 5. Build system prompt
	systemPrompt := a.buildSystemPrompt(card, cfg)

	// 6. Build tool definitions
	cardTypes := []string{}
	if a.registry != nil {
		cardTypes = a.registry.List()
	}
	allCats, _ := a.ListAllCategories()
	var catMaps []map[string]string
	for _, c := range allCats {
		catMaps = append(catMaps, map[string]string{
			"id":         c.CategoryID,
			"breadcrumb": c.Breadcrumb,
		})
	}
	// Build tools with a card-specific set_fields definition
	buildToolDefs := func(c *model.Card) []llm.ToolDef {
		tools := llm.CardTools(cardTypes, catMaps)
		// Replace the generic set_fields with a card-specific version
		// that has explicit property names matching the card's actual blocks
		if c != nil && len(c.Blocks) > 0 {
			fieldProps := make(map[string]any)
			for _, b := range c.Blocks {
				if b.Key == "" {
					continue
				}
				prop := map[string]any{"type": "string"}
				if b.Meta != nil {
					if desc, ok := b.Meta["description"].(string); ok && desc != "" {
						prop["description"] = desc
					}
					if opts, ok := b.Meta["options"].([]string); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
					// Handle options as []any (from JSON unmarshaling)
					if opts, ok := b.Meta["options"].([]any); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
				}
				fieldProps[b.Key] = prop
			}
			if len(fieldProps) > 0 {
				// Find and replace the generic set_fields tool
				for i, t := range tools {
					if t.Name == "set_fields" {
						tools[i] = llm.ToolDef{
							Name:        "set_fields",
							Description: "Fill in field values on the card. ALWAYS call this to write content into fields.",
							Parameters: map[string]any{
								"type":       "object",
								"properties": fieldProps,
							},
						}
						break
					}
				}
			}
		}
		return tools
	}
	toolDefs := buildToolDefs(card)

	// 7. Convert chat history to LLM messages
	var llmMessages []llm.Message
	for _, m := range cf.Messages {
		if m.Role == model.RoleUser || m.Role == model.RoleAssistant {
			llmMessages = append(llmMessages, llm.Message{Role: m.Role, Content: m.Content})
		}
	}

	// 8. Call LLM with tool loop (max 3 iterations)
	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	var allToolActions []model.ToolAction
	var pinSuggestion *model.PinSuggestion

	for iteration := 0; iteration < 3; iteration++ {
		resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
			SystemPrompt: systemPrompt,
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
			cf, _ = a.repo.AppendMessage(cardID, errMsg)
			return cf, nil
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
			}
			cf, _ = a.repo.AppendMessage(cardID, assistantMsg)
			return cf, nil
		}

		// Process tool calls
		// Add assistant message with tool calls to conversation
		assistantLLMMsg := llm.Message{
			Role:      model.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		llmMessages = append(llmMessages, assistantLLMMsg)

		// Deduplicate tool calls within same response — same tool name = skip
		seenCalls := make(map[string]bool)
		for _, tc := range resp.ToolCalls {
			if seenCalls[tc.Name] {
				// Still need to send a tool result back to the LLM
				llmMessages = append(llmMessages, llm.Message{
					Role:       "tool",
					Content:    "Skipped — duplicate call",
					ToolCallID: tc.ID,
				})
				continue
			}
			seenCalls[tc.Name] = true
			result, action, ps := a.executeToolCall(cardID, card, tc, allCats, cfg.AutoPin)
			if action != nil {
				allToolActions = append(allToolActions, *action)
			}
			if ps != nil {
				pinSuggestion = ps
			}
			// Add tool result to conversation
			llmMessages = append(llmMessages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// Reload card after tool execution (it may have changed)
		card, _ = a.repo.GetCard(cardID)
		// Rebuild tool definitions in case type changed and blocks are different now
		toolDefs = buildToolDefs(card)

		// Check if set_fields was missed — nudge the LLM to fill empty fields
		calledSetFields := false
		for _, tc := range resp.ToolCalls {
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
				llmMessages = append(llmMessages, llm.Message{
					Role:    model.RoleUser,
					Content: fmt.Sprintf("[System: The card has empty fields that need content: %s. Use set_fields to fill them based on the conversation.]", strings.Join(emptyKeys, ", ")),
				})
			}
		}

		// Nudge for pin if not yet pinned and suggest_pin wasn't called
		calledPin := false
		for _, tc := range resp.ToolCalls {
			if tc.Name == "suggest_pin" {
				calledPin = true
				break
			}
		}
		if !calledPin && pinSuggestion == nil {
			existingPins, _ := a.repo.GetCardPins(cardID)
			if len(existingPins) == 0 {
				llmMessages = append(llmMessages, llm.Message{
					Role:    model.RoleUser,
					Content: "[System: This card has no pin location yet. Use suggest_pin to pin it to the best-fit category.]",
				})
			}
		}
	}

	// Max iterations reached — save what we have
	assistantMsg := model.ChatMessage{
		ID:            uuid.New().String(),
		Role:          model.RoleAssistant,
		Content:       "I've made the changes to your card.",
		Timestamp:     time.Now().UTC(),
		ToolActions:   allToolActions,
		PinSuggestion: pinSuggestion,
	}
	cf, _ = a.repo.AppendMessage(cardID, assistantMsg)
	return cf, nil
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
		// Apply the schema blocks to the card, preserving existing content
		if a.registry != nil {
			schemaBlocks := a.registry.SchemaToBlocks(cardType)
			if len(schemaBlocks) > 0 {
				existingCard, _ := a.repo.GetCard(cardID)
				if existingCard != nil && len(existingCard.Blocks) > 0 {
					// Index existing blocks by key
					existingByKey := make(map[string]model.Block)
					for _, b := range existingCard.Blocks {
						if b.Key != "" {
							existingByKey[b.Key] = b
						}
					}
					// Merge existing values into schema blocks
					schemaKeys := make(map[string]bool)
					for i, b := range schemaBlocks {
						schemaKeys[b.Key] = true
						if existing, ok := existingByKey[b.Key]; ok {
							schemaBlocks[i].Value = existing.Value
						}
					}
					// Append truly user-added blocks (no Key = manually added by user)
					// Blocks with a Key that doesn't match the new schema are from the OLD
					// type's schema and should be dropped during type change.
					for _, b := range existingCard.Blocks {
						if b.Key == "" {
							schemaBlocks = append(schemaBlocks, b)
						}
					}
				}
				a.UpdateCardBlocks(cardID, schemaBlocks)
			}
		}
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
			// Try flat arguments: the dynamic schema puts block keys directly in tc.Arguments
			currentCard2, err2 := a.repo.GetCard(cardID)
			if err2 == nil {
				blockKeys := make(map[string]bool)
				for _, b := range currentCard2.Blocks {
					if b.Key != "" {
						blockKeys[b.Key] = true
					}
				}
				flat := make(map[string]any)
				for k, v := range tc.Arguments {
					if blockKeys[k] {
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
		updated := false
		var updatedKeys []string
		for i, b := range currentCard.Blocks {
			if val, ok := fieldsMap[b.Key]; ok {
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

	case "suggest_pin":
		catID, _ := tc.Arguments["category_id"].(string)
		reason, _ := tc.Arguments["reason"].(string)

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
			Status:       "pending",
		}
		action := &model.ToolAction{Tool: "suggest_pin", Input: tc.Arguments, Result: "Suggested pin to " + breadcrumb}
		return "Pin suggestion created for " + breadcrumb, action, ps

	default:
		return "error: unknown tool " + tc.Name, nil, nil
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
	return cfg.Provider != ""
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
- add_tags — Add relevant tags. Prefer existing project tags listed below, but you may create new short, descriptive tags if none fit.`, time.Now().Format("2006-01-02 (Monday)"), time.Now().Format("2006-01-02")))

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

	// Hierarchy context based on ContextLevel
	level := card.ContextLevel
	if level == "" {
		level = model.ContextProject
	}
	if level == model.ContextIsolated || a.repo == nil {
		return strings.Join(parts, "\n\n")
	}

	// Always show categories for pin suggestions — include descriptions for context
	allCats, _ := a.ListAllCategories()
	if len(allCats) > 0 {
		var catDescs []string
		for _, c := range allCats {
			desc := fmt.Sprintf("- %s (id: %s)", c.Breadcrumb, c.CategoryID)
			if len(c.AcceptedTypes) > 0 {
				desc += " [accepts: " + strings.Join(c.AcceptedTypes, ", ") + "]"
			}
			// Append any descriptions to help the LLM understand what this location is for
			var descs []string
			if c.BrandDescription != "" {
				descs = append(descs, c.BrandName+": "+c.BrandDescription)
			}
			if c.StreamDescription != "" {
				descs = append(descs, c.StreamName+": "+c.StreamDescription)
			}
			if c.ProjectDescription != "" {
				descs = append(descs, c.ProjectName+": "+c.ProjectDescription)
			}
			if c.CategoryDescription != "" {
				descs = append(descs, c.CategoryName+": "+c.CategoryDescription)
			}
			if len(descs) > 0 {
				desc += "\n  " + strings.Join(descs, " | ")
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

	// Brand context for pinned cards
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
