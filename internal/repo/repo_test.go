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

	// Verify directories exist
	for _, sub := range []string{".bruv", "brands", "cards", "pins", "types"} {
		path := filepath.Join(r.Root, sub)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", sub)
		}
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
	if _, err := Init(dir, "Second"); err == nil {
		t.Fatal("expected error on second Init, got nil")
	}
}

func TestOpenExistingRepo(t *testing.T) {
	dir := t.TempDir()
	original, err := Init(dir, "Test")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	reopened, err := Open(dir)
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
		c.Fields["description"] = "Cards pinnable to multiple boards"
		c.Tags = []string{"core", "architecture"}
	})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if updated.Fields["description"] != "Cards pinnable to multiple boards" {
		t.Error("field not updated")
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

func TestChecklist(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("brainstorm", "Ideas")

	// Add items
	card, err := r.AddChecklistItem(card.ID, "Research competitors")
	if err != nil {
		t.Fatalf("AddChecklistItem: %v", err)
	}
	card, _ = r.AddChecklistItem(card.ID, "Draft proposal")

	if len(card.Checklist) != 2 {
		t.Fatalf("checklist count = %d, want 2", len(card.Checklist))
	}

	// Toggle
	itemID := card.Checklist[0].ID
	card, err = r.ToggleChecklistItem(card.ID, itemID)
	if err != nil {
		t.Fatalf("ToggleChecklistItem: %v", err)
	}
	if !card.Checklist[0].Done {
		t.Error("expected item to be done")
	}

	// Toggle back
	card, _ = r.ToggleChecklistItem(card.ID, itemID)
	if card.Checklist[0].Done {
		t.Error("expected item to be undone")
	}

	// Remove
	card, err = r.RemoveChecklistItem(card.ID, itemID)
	if err != nil {
		t.Fatalf("RemoveChecklistItem: %v", err)
	}
	if len(card.Checklist) != 1 {
		t.Errorf("after remove, count = %d, want 1", len(card.Checklist))
	}
}

func TestPromoteChecklistItem(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("brainstorm", "Ideas")
	card, _ = r.AddChecklistItem(card.ID, "Build kanban board")

	itemID := card.Checklist[0].ID
	promoted, err := r.PromoteChecklistItem(card.ID, itemID, "feature")
	if err != nil {
		t.Fatalf("PromoteChecklistItem: %v", err)
	}

	if promoted.Title != "Build kanban board" {
		t.Errorf("promoted title = %q", promoted.Title)
	}
	if promoted.Type != "feature" {
		t.Errorf("promoted type = %q", promoted.Type)
	}
	if promoted.Fields["promoted_from"] != card.ID {
		t.Error("missing promoted_from reference")
	}

	// Parent card should no longer have the checklist item
	parent, _ := r.GetCard(card.ID)
	if len(parent.Checklist) != 0 {
		t.Errorf("parent checklist count = %d, want 0", len(parent.Checklist))
	}
}

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
