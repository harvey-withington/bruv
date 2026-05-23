package index

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestIndex(t *testing.T) (*Index, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".bruv", "index.db")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx, dir
}

func setupTestRepoWithIndex(t *testing.T) (*repo.Repository, *Index) {
	t.Helper()
	dir := t.TempDir()

	r, err := repo.Init(dir, "Test Repo")
	if err != nil {
		t.Fatalf("Init repo: %v", err)
	}

	dbPath := filepath.Join(dir, ".bruv", "index.db")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	return r, idx
}

// --- Basic Index Operations ---

func TestOpenAndClose(t *testing.T) {
	idx, _ := setupTestIndex(t)
	count, err := idx.CardCount()
	if err != nil {
		t.Fatalf("CardCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 cards, got %d", count)
	}
}

func TestIndexCard(t *testing.T) {
	idx, _ := setupTestIndex(t)

	card := &model.Card{
		ID:           "card-001",
		Type:         "feature",
		Title:        "Multi-board pinning",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		ContextLevel: model.ContextProject,
		Blocks: []model.Block{
			{ID: "b1", Type: model.BlockText, Key: "description", Value: "Cards should be pinnable to multiple boards"},
		},
		Tags: []string{"core", "architecture"},
	}

	if err := idx.IndexCard(card, time.Now().UTC(), ""); err != nil {
		t.Fatalf("IndexCard: %v", err)
	}

	count, _ := idx.CardCount()
	if count != 1 {
		t.Errorf("card count = %d, want 1", count)
	}
}

func TestRemoveCard(t *testing.T) {
	idx, _ := setupTestIndex(t)

	card := &model.Card{
		ID: "card-001", Type: "task", Title: "Test",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		ContextLevel: model.ContextIsolated,
		Tags:         []string{"test"},
	}
	idx.IndexCard(card, time.Now().UTC(), "")

	if err := idx.RemoveCard("card-001"); err != nil {
		t.Fatalf("RemoveCard: %v", err)
	}

	count, _ := idx.CardCount()
	if count != 0 {
		t.Errorf("after remove, count = %d, want 0", count)
	}
}

// --- Search ---

func TestFullTextSearch(t *testing.T) {
	idx, _ := setupTestIndex(t)

	cards := []*model.Card{
		{
			ID: "card-001", Type: "feature", Title: "Kanban board drag and drop",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			ContextLevel: model.ContextProject,
			Blocks: []model.Block{
				{ID: "b1", Type: model.BlockText, Key: "description", Value: "Implement drag and drop for cards between categories"},
			},
			Tags: []string{"ui", "core"},
		},
		{
			ID: "card-002", Type: "reference", Title: "Bitcoin self-custody guide",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			ContextLevel: model.ContextBrand,
			Blocks: []model.Block{
				{ID: "b1", Type: model.BlockText, Key: "content", Value: "A comprehensive guide to Bitcoin self-custody wallets"},
			},
			Tags: []string{"bitcoin", "reference"},
		},
		{
			ID: "card-003", Type: "task", Title: "Fix card rendering bug",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			ContextLevel: model.ContextIsolated,
			Blocks: []model.Block{
				{ID: "b1", Type: model.BlockText, Key: "description", Value: "Cards don't render correctly when dragged"},
			},
			Tags: []string{"bug", "ui"},
		},
	}

	now := time.Now().UTC()
	for _, c := range cards {
		if err := idx.IndexCard(c, now, ""); err != nil {
			t.Fatalf("IndexCard %s: %v", c.ID, err)
		}
	}

	// Search for "bitcoin" — should find card-002
	results, err := idx.Search("bitcoin", 10)
	if err != nil {
		t.Fatalf("Search bitcoin: %v", err)
	}
	if len(results) != 1 || results[0].CardID != "card-002" {
		t.Errorf("bitcoin search: got %d results, want 1 (card-002)", len(results))
	}

	// Search for "drag" — should find card-001 and card-003
	results, err = idx.Search("drag", 10)
	if err != nil {
		t.Fatalf("Search drag: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("drag search: got %d results, want 2", len(results))
	}

	// Search for "card" — should find multiple
	results, err = idx.Search("card", 10)
	if err != nil {
		t.Fatalf("Search card: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("card search: got %d results, want >= 2", len(results))
	}
}

// --- Tag queries ---

func TestListCardIDsByTag(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now().UTC()

	idx.IndexCard(&model.Card{
		ID: "c1", Type: "task", Title: "A", Tags: []string{"ui", "core"},
		CreatedAt: now, UpdatedAt: now, ContextLevel: model.ContextProject,
	}, now, "")
	idx.IndexCard(&model.Card{
		ID: "c2", Type: "task", Title: "B", Tags: []string{"backend"},
		CreatedAt: now, UpdatedAt: now, ContextLevel: model.ContextProject,
	}, now, "")

	ids, err := idx.ListCardIDsByTag("ui")
	if err != nil {
		t.Fatalf("ListCardIDsByTag: %v", err)
	}
	if len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("tag=ui: got %v, want [c1]", ids)
	}
}

// --- Type queries ---

func TestListCardIDsByType(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now().UTC()

	idx.IndexCard(&model.Card{
		ID: "c1", Type: "feature", Title: "F1",
		CreatedAt: now, UpdatedAt: now, ContextLevel: model.ContextProject, Tags: []string{},
	}, now, "")
	idx.IndexCard(&model.Card{
		ID: "c2", Type: "task", Title: "T1",
		CreatedAt: now, UpdatedAt: now, ContextLevel: model.ContextProject, Tags: []string{},
	}, now, "")

	ids, _ := idx.ListCardIDsByType("feature")
	if len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("type=feature: got %v, want [c1]", ids)
	}
}

// --- Pin queries ---

func TestPinIndexAndQuery(t *testing.T) {
	idx, _ := setupTestIndex(t)

	pins := []model.Pin{
		{CardID: "c1", ProjectID: "proj-a", CategoryID: "cat-1", Position: 0, PinnedAt: time.Now().UTC()},
		{CardID: "c2", ProjectID: "proj-a", CategoryID: "cat-1", Position: 1, PinnedAt: time.Now().UTC()},
		{CardID: "c1", ProjectID: "proj-b", CategoryID: "cat-2", Position: 0, PinnedAt: time.Now().UTC()},
	}

	if err := idx.IndexPins("c1", []model.Pin{pins[0], pins[2]}); err != nil {
		t.Fatalf("IndexPins c1: %v", err)
	}
	if err := idx.IndexPins("c2", []model.Pin{pins[1]}); err != nil {
		t.Fatalf("IndexPins c2: %v", err)
	}

	ids, err := idx.ListCardIDsInCategory("cat-1")
	if err != nil {
		t.Fatalf("ListCardIDsInCategory: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("cat-1: got %d cards, want 2", len(ids))
	}
	if ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("cat-1 order: got %v, want [c1, c2]", ids)
	}

	ids, _ = idx.ListCardIDsInCategory("cat-2")
	if len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("cat-2: got %v, want [c1]", ids)
	}
}

// --- Full Rebuild ---

func TestFullRebuild(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)

	// Create cards and pins via the repo layer
	r.CreateBrand("Test Brand")
	r.CreateStream("test-brand", "v1")
	proj, _ := r.CreateProject("test-brand", "v1", "Backlog")
	cat, _ := r.CreateCategory("test-brand", "v1", "backlog", "Todo", 0)

	card1, _ := r.CreateCard("feature", "Drag and drop")
	card2, _ := r.CreateCard("task", "Write tests")
	card3, _ := r.CreateCard("brainstorm", "Future ideas")

	r.PinCard(card1.ID, cat.ID)
	r.PinCard(card2.ID, cat.ID)
	_ = proj // proj fixture left for clarity; pin keys on category alone now

	// Full rebuild
	stats, err := idx.FullRebuild(r.Root)
	if err != nil {
		t.Fatalf("FullRebuild: %v", err)
	}

	if stats.CardsIndexed != 3 {
		t.Errorf("cards indexed = %d, want 3", stats.CardsIndexed)
	}
	if stats.PinsIndexed != 2 {
		t.Errorf("pins indexed = %d, want 2", stats.PinsIndexed)
	}

	count, _ := idx.CardCount()
	if count != 3 {
		t.Errorf("card count = %d, want 3", count)
	}

	// Search should work after rebuild
	results, _ := idx.Search("drag", 10)
	if len(results) != 1 {
		t.Errorf("search 'drag' after rebuild: got %d results, want 1", len(results))
	}

	// Pin query should work
	ids, _ := idx.ListCardIDsInCategory(cat.ID)
	if len(ids) != 2 {
		t.Errorf("category cards after rebuild: got %d, want 2", len(ids))
	}

	_ = card3
}

// --- Incremental Refresh ---

func TestIncrementalRefresh(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)

	// Create initial cards
	card1, _ := r.CreateCard("feature", "Initial feature")
	r.CreateCard("task", "Initial task")

	// Full rebuild first
	idx.FullRebuild(r.Root)

	count, _ := idx.CardCount()
	if count != 2 {
		t.Fatalf("initial count = %d, want 2", count)
	}

	// Add a new card (not in index yet)
	r.CreateCard("brainstorm", "New idea")

	// Modify an existing card
	r.UpdateCard(card1.ID, func(c *model.Card) {
		c.Title = "Updated feature title"
	})

	// Incremental refresh should pick up the new card and the modified one
	stats, err := idx.IncrementalRefresh(r.Root)
	if err != nil {
		t.Fatalf("IncrementalRefresh: %v", err)
	}

	// Should have indexed the new card + the modified one
	if stats.CardsIndexed < 2 {
		t.Errorf("cards indexed = %d, want >= 2", stats.CardsIndexed)
	}

	count, _ = idx.CardCount()
	if count != 3 {
		t.Errorf("after refresh, count = %d, want 3", count)
	}

	// Search for updated title
	results, _ := idx.Search("Updated", 10)
	if len(results) != 1 {
		t.Errorf("search 'Updated': got %d results, want 1", len(results))
	}
}

func TestIncrementalRefreshRemovesDeletedCards(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)

	card, _ := r.CreateCard("task", "Will be deleted")
	idx.FullRebuild(r.Root)

	count, _ := idx.CardCount()
	if count != 1 {
		t.Fatalf("initial count = %d, want 1", count)
	}

	// Delete the card file directly
	os.Remove(filepath.Join(r.Root, "cards", card.ID+".json"))

	stats, err := idx.IncrementalRefresh(r.Root)
	if err != nil {
		t.Fatalf("IncrementalRefresh: %v", err)
	}

	if stats.CardsRemoved != 1 {
		t.Errorf("cards removed = %d, want 1", stats.CardsRemoved)
	}

	count, _ = idx.CardCount()
	if count != 0 {
		t.Errorf("after refresh, count = %d, want 0", count)
	}
}

// --- Rebuild from clean (deleted index.db) ---

func TestRebuildAfterIndexDeletion(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)

	r.CreateCard("feature", "Persistent card")
	idx.FullRebuild(r.Root)

	// Close and delete the index
	idx.Close()
	dbPath := filepath.Join(r.Root, ".bruv", "index.db")
	os.Remove(dbPath)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

	// Re-open a fresh index and rebuild
	idx2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer idx2.Close()

	stats, err := idx2.FullRebuild(r.Root)
	if err != nil {
		t.Fatalf("FullRebuild after deletion: %v", err)
	}

	if stats.CardsIndexed != 1 {
		t.Errorf("cards indexed = %d, want 1", stats.CardsIndexed)
	}

	count, _ := idx2.CardCount()
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
