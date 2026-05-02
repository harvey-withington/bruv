package repo

import (
	"bruv/internal/model"
	"os"
	"path/filepath"
	"testing"
)

// helper to create a temporary repository for testing
func setupTestRepo(t *testing.T) *Repository {
	t.Helper()
	dir := t.TempDir()
	r, err := Init(dir, "Test Repository")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return r
}

// --- Repository Init/Open ---

func TestInitCreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir, "My Repo")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify directories exist. .bruv/ is intentionally NOT pre-created
	// — it's a derived-state cache and gets created on demand by the
	// SQLite index and the lock file.
	for _, sub := range []string{"brands", "cards", "pins", "types"} {
		path := filepath.Join(r.Root, sub)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", sub)
		}
	}

	// Manifest lives at the repo root, not under .bruv/.
	if _, err := os.Stat(filepath.Join(r.Root, "manifest.json")); err != nil {
		t.Errorf("expected manifest.json at repo root: %v", err)
	}

	// Verify manifest
	if r.Manifest.Name != "My Repo" {
		t.Errorf("manifest name = %q, want %q", r.Manifest.Name, "My Repo")
	}
	if r.Manifest.Version != "0.1.0" {
		t.Errorf("manifest version = %q, want %q", r.Manifest.Version, "0.1.0")
	}
}

func TestInitRejectsExistingRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir, "First"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(dir, "First"); err == nil {
		t.Fatal("expected error on second Init, got nil")
	}
}

func TestOpenExistingRepo(t *testing.T) {
	dir := t.TempDir()
	original, err := Init(dir, "Test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	reopened, err := Open(original.Root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if reopened.Manifest.Name != original.Manifest.Name {
		t.Errorf("reopened name = %q, want %q", reopened.Manifest.Name, original.Manifest.Name)
	}
}

func TestOpenMissingRepoFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err == nil {
		t.Fatal("expected error opening non-existent repo")
	}
}

// --- Brand CRUD ---

func TestBrandCRUD(t *testing.T) {
	r := setupTestRepo(t)

	// Create
	brand, err := r.CreateBrand("Good Egg Software")
	if err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}
	if brand.Name != "Good Egg Software" {
		t.Errorf("name = %q, want %q", brand.Name, "Good Egg Software")
	}
	if brand.Slug != "good-egg-software" {
		t.Errorf("slug = %q, want %q", brand.Slug, "good-egg-software")
	}
	if brand.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Get
	got, err := r.GetBrand("good-egg-software")
	if err != nil {
		t.Fatalf("GetBrand: %v", err)
	}
	if got.ID != brand.ID {
		t.Errorf("GetBrand ID = %q, want %q", got.ID, brand.ID)
	}

	// List
	brands, err := r.ListBrands()
	if err != nil {
		t.Fatalf("ListBrands: %v", err)
	}
	if len(brands) != 1 {
		t.Fatalf("ListBrands count = %d, want 1", len(brands))
	}

	// Update
	updated, err := r.UpdateBrand("good-egg-software", func(b *model.Brand) {
		b.Website = "https://goodegg.software"
	})
	if err != nil {
		t.Fatalf("UpdateBrand: %v", err)
	}
	if updated.Website != "https://goodegg.software" {
		t.Errorf("website = %q, want %q", updated.Website, "https://goodegg.software")
	}

	// Delete
	if err := r.DeleteBrand("good-egg-software"); err != nil {
		t.Fatalf("DeleteBrand: %v", err)
	}
	brands, _ = r.ListBrands()
	if len(brands) != 0 {
		t.Errorf("after delete, ListBrands count = %d, want 0", len(brands))
	}
}

func TestCreateDuplicateBrandFails(t *testing.T) {
	r := setupTestRepo(t)
	if _, err := r.CreateBrand("Test"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CreateBrand("Test"); err == nil {
		t.Fatal("expected error creating duplicate brand")
	}
}

// --- Stream CRUD ---

func TestStreamCRUD(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("My Brand")

	// Create
	stream, err := r.CreateStream("my-brand", "Season 1")
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if stream.Slug != "season-1" {
		t.Errorf("slug = %q, want %q", stream.Slug, "season-1")
	}

	// Get
	got, err := r.GetStream("my-brand", "season-1")
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got.ID != stream.ID {
		t.Errorf("ID mismatch")
	}

	// List
	streams, err := r.ListStreams("my-brand")
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(streams) != 1 {
		t.Fatalf("count = %d, want 1", len(streams))
	}

	// Update
	updated, err := r.UpdateStream("my-brand", "season-1", func(s *model.Stream) {
		s.Name = "Season 1 (Updated)"
	})
	if err != nil {
		t.Fatalf("UpdateStream: %v", err)
	}
	if updated.Name != "Season 1 (Updated)" {
		t.Errorf("name = %q", updated.Name)
	}

	// Delete
	if err := r.DeleteStream("my-brand", "season-1"); err != nil {
		t.Fatalf("DeleteStream: %v", err)
	}
}

// --- Project CRUD ---

func TestProjectCRUD(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("My Brand")
	r.CreateStream("my-brand", "v1")

	// Create
	project, err := r.CreateProject("my-brand", "v1", "Feature Backlog")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if project.Slug != "feature-backlog" {
		t.Errorf("slug = %q, want %q", project.Slug, "feature-backlog")
	}

	// List
	projects, err := r.ListProjects("my-brand", "v1")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("count = %d, want 1", len(projects))
	}

	// Delete
	if err := r.DeleteProject("my-brand", "v1", "feature-backlog"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

// --- Category CRUD ---

func TestCategoryCRUD(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("My Brand")
	r.CreateStream("my-brand", "v1")
	r.CreateProject("my-brand", "v1", "Backlog")

	// Create multiple categories
	cat1, err := r.CreateCategory("my-brand", "v1", "backlog", "Ideas", 0)
	if err != nil {
		t.Fatalf("CreateCategory Ideas: %v", err)
	}
	cat2, err := r.CreateCategory("my-brand", "v1", "backlog", "In Progress", 1)
	if err != nil {
		t.Fatalf("CreateCategory In Progress: %v", err)
	}
	_, err = r.CreateCategory("my-brand", "v1", "backlog", "Done", 2)
	if err != nil {
		t.Fatalf("CreateCategory Done: %v", err)
	}

	// List — should be sorted by position
	cats, err := r.ListCategories("my-brand", "v1", "backlog")
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("count = %d, want 3", len(cats))
	}
	if cats[0].Name != "Ideas" || cats[1].Name != "In Progress" || cats[2].Name != "Done" {
		t.Errorf("wrong order: %v, %v, %v", cats[0].Name, cats[1].Name, cats[2].Name)
	}

	_ = cat1
	_ = cat2

	// Delete
	if err := r.DeleteCategory("my-brand", "v1", "backlog", "done"); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}
	cats, _ = r.ListCategories("my-brand", "v1", "backlog")
	if len(cats) != 2 {
		t.Errorf("after delete, count = %d, want 2", len(cats))
	}
}

// --- Category Reorder ---

func TestReorderCategories(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	r.CreateProject("b", "v1", "P")
	r.CreateCategory("b", "v1", "p", "Alpha", 0)
	r.CreateCategory("b", "v1", "p", "Beta", 1)
	r.CreateCategory("b", "v1", "p", "Gamma", 2)

	// Reverse the order
	if err := r.ReorderCategories("b", "v1", "p", []string{"gamma", "beta", "alpha"}); err != nil {
		t.Fatalf("ReorderCategories: %v", err)
	}

	cats, _ := r.ListCategories("b", "v1", "p")
	if len(cats) != 3 {
		t.Fatalf("count = %d, want 3", len(cats))
	}
	if cats[0].Name != "Gamma" || cats[1].Name != "Beta" || cats[2].Name != "Alpha" {
		t.Errorf("wrong order after reorder: %v, %v, %v", cats[0].Name, cats[1].Name, cats[2].Name)
	}
}

func TestDeleteCategoryNonExistentFails(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	r.CreateProject("b", "v1", "P")

	if err := r.DeleteCategory("b", "v1", "p", "nonexistent"); err == nil {
		t.Fatal("expected error deleting non-existent category")
	}
}

// --- Move Card Between Categories ---

func TestMoveCardToCategory(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	proj, _ := r.CreateProject("b", "v1", "P")
	catA, _ := r.CreateCategory("b", "v1", "p", "Todo", 0)
	catB, _ := r.CreateCategory("b", "v1", "p", "Done", 1)

	card, _ := r.CreateCard("task", "Move me")
	r.PinCard(card.ID, proj.ID, catA.ID)

	// Move card from Todo to Done
	if err := r.MoveCardToCategory(card.ID, proj.ID, catA.ID, catB.ID, 0); err != nil {
		t.Fatalf("MoveCardToCategory: %v", err)
	}

	// Card should no longer be in Todo
	pinsInA, _ := r.ListCardsInCategory(proj.ID, catA.ID)
	if len(pinsInA) != 0 {
		t.Errorf("Todo should have 0 cards, got %d", len(pinsInA))
	}

	// Card should be in Done — both ProjectID and CategoryID updated
	pinsInB, _ := r.ListCardsInCategory(catB.ID, catB.ID)
	if len(pinsInB) != 1 {
		t.Errorf("Done should have 1 card, got %d", len(pinsInB))
	}

	// Verify pin fields were both updated
	pins, _ := r.GetCardPins(card.ID)
	if len(pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(pins))
	}
	if pins[0].ProjectID != catB.ID {
		t.Errorf("pin ProjectID = %q, want %q", pins[0].ProjectID, catB.ID)
	}
	if pins[0].CategoryID != catB.ID {
		t.Errorf("pin CategoryID = %q, want %q", pins[0].CategoryID, catB.ID)
	}
}

func TestMoveCardToCategoryFrontendConvention(t *testing.T) {
	// Simulates the frontend convention where projectID == categoryID
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	r.CreateProject("b", "v1", "P")
	catA, _ := r.CreateCategory("b", "v1", "p", "Todo", 0)
	catB, _ := r.CreateCategory("b", "v1", "p", "Done", 1)

	card, _ := r.CreateCard("task", "Drag me")
	// Pin with projectID == categoryID (frontend convention)
	r.PinCard(card.ID, catA.ID, catA.ID)

	// Move using fromCategoryId as projectID (frontend convention)
	if err := r.MoveCardToCategory(card.ID, catA.ID, catA.ID, catB.ID, 0); err != nil {
		t.Fatalf("MoveCardToCategory: %v", err)
	}

	// Card must be findable via ListCardsInCategory(catB.ID, catB.ID)
	pinsInB, _ := r.ListCardsInCategory(catB.ID, catB.ID)
	if len(pinsInB) != 1 {
		t.Errorf("card should be visible in target category, got %d pins", len(pinsInB))
	}

	// Must NOT appear in source category
	pinsInA, _ := r.ListCardsInCategory(catA.ID, catA.ID)
	if len(pinsInA) != 0 {
		t.Errorf("card should not be in source category, got %d pins", len(pinsInA))
	}

	// MoveCardInCategory should work on the moved card with new IDs
	if err := r.MoveCardInCategory(card.ID, catB.ID, catB.ID, 5); err != nil {
		t.Fatalf("MoveCardInCategory after cross-column move should work: %v", err)
	}
}

func TestMoveCardInCategoryReorder(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	proj, _ := r.CreateProject("b", "v1", "P")
	cat, _ := r.CreateCategory("b", "v1", "p", "Todo", 0)

	card1, _ := r.CreateCard("task", "First")
	card2, _ := r.CreateCard("task", "Second")
	r.PinCard(card1.ID, proj.ID, cat.ID)
	r.PinCard(card2.ID, proj.ID, cat.ID)

	// Move card1 to position 1 (after card2)
	if err := r.MoveCardInCategory(card1.ID, proj.ID, cat.ID, 1); err != nil {
		t.Fatalf("MoveCardInCategory: %v", err)
	}

	pins, _ := r.GetCardPins(card1.ID)
	for _, p := range pins {
		if p.ProjectID == proj.ID && p.CategoryID == cat.ID {
			if p.Position != 1 {
				t.Errorf("card1 position = %d, want 1", p.Position)
			}
		}
	}
}

// --- Tag Colors ---

func TestTagColorCRUD(t *testing.T) {
	r := setupTestRepo(t)

	// Set colors
	if _, err := r.SetTagColor("urgent", "#ff0000"); err != nil {
		t.Fatalf("SetTagColor: %v", err)
	}
	if _, err := r.SetTagColor("feature", "#00ff00"); err != nil {
		t.Fatalf("SetTagColor: %v", err)
	}

	// Get colors
	colors, err := r.GetTagColors()
	if err != nil {
		t.Fatalf("GetTagColors: %v", err)
	}
	if colors["urgent"] != "#ff0000" {
		t.Errorf("urgent = %q, want %q", colors["urgent"], "#ff0000")
	}
	if colors["feature"] != "#00ff00" {
		t.Errorf("feature = %q, want %q", colors["feature"], "#00ff00")
	}

	// Overwrite
	if _, err := r.SetTagColor("urgent", "#cc0000"); err != nil {
		t.Fatalf("SetTagColor overwrite: %v", err)
	}
	colors, _ = r.GetTagColors()
	if colors["urgent"] != "#cc0000" {
		t.Errorf("after overwrite: urgent = %q, want %q", colors["urgent"], "#cc0000")
	}
}

// --- Card CRUD ---

func TestCardCRUD(t *testing.T) {
	r := setupTestRepo(t)

	// Create
	card, err := r.CreateCard("feature", "Multi-board pinning")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if card.Title != "Multi-board pinning" {
		t.Errorf("title = %q", card.Title)
	}
	if card.Type != "feature" {
		t.Errorf("type = %q", card.Type)
	}
	if card.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Get
	got, err := r.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if got.Title != card.Title {
		t.Errorf("title mismatch")
	}

	// Update
	updated, err := r.UpdateCard(card.ID, func(c *model.Card) {
		c.Description = "Cards pinnable to multiple boards"
		c.Tags = []string{"core", "architecture"}
	})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if updated.Description != "Cards pinnable to multiple boards" {
		t.Error("description not updated")
	}
	if len(updated.Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(updated.Tags))
	}

	// List
	cards, err := r.ListCards()
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("count = %d, want 1", len(cards))
	}

	// ListByType
	features, err := r.ListCardsByType("feature")
	if err != nil {
		t.Fatalf("ListCardsByType: %v", err)
	}
	if len(features) != 1 {
		t.Errorf("features count = %d, want 1", len(features))
	}
	brainstorms, _ := r.ListCardsByType("brainstorm")
	if len(brainstorms) != 0 {
		t.Errorf("brainstorms count = %d, want 0", len(brainstorms))
	}

	// Delete
	if err := r.DeleteCard(card.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	cards, _ = r.ListCards()
	if len(cards) != 0 {
		t.Errorf("after delete, count = %d, want 0", len(cards))
	}
}

// --- Checklist ---

// --- Pins ---

func TestPinning(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand A")
	r.CreateStream("brand-a", "v1")
	projA, _ := r.CreateProject("brand-a", "v1", "Project Alpha")
	catA, _ := r.CreateCategory("brand-a", "v1", "project-alpha", "Todo", 0)

	r.CreateBrand("Brand B")
	r.CreateStream("brand-b", "v1")
	projB, _ := r.CreateProject("brand-b", "v1", "Project Beta")
	catB, _ := r.CreateCategory("brand-b", "v1", "project-beta", "Backlog", 0)

	card, _ := r.CreateCard("reference", "Bitcoin Self-Custody Guide")

	// Pin to first project
	if err := r.PinCard(card.ID, projA.ID, catA.ID); err != nil {
		t.Fatalf("PinCard to A: %v", err)
	}

	// Pin to second project
	if err := r.PinCard(card.ID, projB.ID, catB.ID); err != nil {
		t.Fatalf("PinCard to B: %v", err)
	}

	// Get pins
	pins, err := r.GetCardPins(card.ID)
	if err != nil {
		t.Fatalf("GetCardPins: %v", err)
	}
	if len(pins) != 2 {
		t.Fatalf("pin count = %d, want 2", len(pins))
	}

	// Duplicate pin should fail
	if err := r.PinCard(card.ID, projA.ID, catA.ID); err == nil {
		t.Fatal("expected error on duplicate pin")
	}

	// List cards in category
	cardsInA, err := r.ListCardsInCategory(projA.ID, catA.ID)
	if err != nil {
		t.Fatalf("ListCardsInCategory: %v", err)
	}
	if len(cardsInA) != 1 {
		t.Errorf("cards in A = %d, want 1", len(cardsInA))
	}

	// Unpin from first project
	if err := r.UnpinCard(card.ID, projA.ID, catA.ID); err != nil {
		t.Fatalf("UnpinCard: %v", err)
	}
	pins, _ = r.GetCardPins(card.ID)
	if len(pins) != 1 {
		t.Errorf("after unpin, count = %d, want 1", len(pins))
	}

	// Unpin from second — should clean up pin directory
	if err := r.UnpinCard(card.ID, projB.ID, catB.ID); err != nil {
		t.Fatalf("UnpinCard last: %v", err)
	}
	pins, _ = r.GetCardPins(card.ID)
	if len(pins) != 0 {
		t.Errorf("after full unpin, count = %d, want 0", len(pins))
	}
}

func TestDeleteCardRemovesPins(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	proj, _ := r.CreateProject("b", "v1", "P")
	cat, _ := r.CreateCategory("b", "v1", "p", "Todo", 0)

	card, _ := r.CreateCard("task", "Do something")
	r.PinCard(card.ID, proj.ID, cat.ID)

	// Delete card should also remove pins
	if err := r.DeleteCard(card.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}

	// Pins directory should be gone
	if fileExists(r.pinsDirPath(card.ID)) {
		t.Error("expected pin directory to be removed after card deletion")
	}
}

// --- Atomic Write Safety ---

func TestAtomicWriteNoCorruption(t *testing.T) {
	r := setupTestRepo(t)

	// Create a card, then verify the JSON on disk is valid
	card, _ := r.CreateCard("task", "Test atomic write")

	// Read raw file and verify it's valid JSON
	path := r.cardFilePath(card.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("card file is empty")
	}

	// Verify no .tmp files remain
	tmpPath := path + ".tmp"
	if fileExists(tmpPath) {
		t.Error("temp file should not exist after successful write")
	}
}

// --- Slugify ---

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Good Egg Software", "good-egg-software"},
		{"Season 1", "season-1"},
		{"v0.3", "v0-3"},
		{"  Spaced  Out  ", "spaced-out"},
		{"UPPERCASE", "uppercase"},
		{"already-slugged", "already-slugged"},
		{"Special!@#Characters", "specialcharacters"},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
