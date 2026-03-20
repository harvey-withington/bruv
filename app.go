package main

import (
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
	"context"
	"fmt"
	"path/filepath"
	"time"

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

func (a *App) DeleteBrand(slug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteBrand(slug)
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

func (a *App) DeleteStream(brandSlug, streamSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteStream(brandSlug, streamSlug)
}

// --- Project ---

func (a *App) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.CreateProject(brandSlug, streamSlug, name)
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

func (a *App) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteProject(brandSlug, streamSlug, projectSlug)
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

func (a *App) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return a.repo.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
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
	return card, err
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

func (a *App) PinCard(cardID, projectID, categoryID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
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
	return nil
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
		for _, cardID := range cardIDs {
			newCard, err := a.repo.DuplicateCard(cardID)
			if err != nil {
				continue
			}
			// Pin with categoryID for both projectID and categoryID
			_ = a.repo.PinCard(newCard.ID, newCatID, newCatID)
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
func (a *App) CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream string) (*model.Project, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Snapshot source category IDs
	oldCatIDs := a.snapshotCatIDs(fromBrand, fromStream, projectSlug)

	result, err := a.repo.CopyProject(fromBrand, fromStream, projectSlug, toBrand, toStream)
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

// SearchCards performs a full-text search across all indexed cards.
func (a *App) SearchCards(query string, limit int) ([]index.SearchResult, error) {
	if a.idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return a.idx.Search(query, limit)
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
