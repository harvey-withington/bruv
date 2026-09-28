package index

import (
	"testing"
	"time"

	"bruv/internal/model"
)

// A pin file naming one category twice must not fail the whole card's
// index write (field report 2026-09-17: every later pin write for the
// card hit the unique key and the boards drifted from disk).
func TestIndexPinsToleratesDuplicateCategory(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now()
	pins := []model.Pin{
		{CardID: "c1", ProjectID: "cat", CategoryID: "cat", Position: 0, PinnedAt: now},
		{CardID: "c1", ProjectID: "cat", CategoryID: "cat", Position: 1, PinnedAt: now},
		{CardID: "c1", ProjectID: "other", CategoryID: "other", Position: 0, PinnedAt: now},
	}
	if err := idx.IndexPins("c1", pins); err != nil {
		t.Fatalf("IndexPins must survive a duplicate pin: %v", err)
	}
	ids, err := idx.ListCardIDsInCategory("cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("category listing = %v, want [c1]", ids)
	}
	if ids, _ := idx.ListCardIDsInCategory("other"); len(ids) != 1 {
		t.Errorf("the non-duplicate pin must still be indexed, got %v", ids)
	}
}

// FTS5 treats `-`, `:` and friends as syntax; every search term is quoted
// so a hyphenated title is findable and a stray colon isn't a column.
func TestSearchQuotesOperatorCharacters(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)
	card, err := r.CreateCard("", "Books (Non-Fiction): a title with punctuation")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexCard(card, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"Non-Fiction", "non-fic", "Fiction:", `"quoted"`, "punct"} {
		if _, err := idx.Search(q, 10); err != nil {
			t.Errorf("Search(%q) errored: %v", q, err)
		}
	}
	hits, err := idx.Search("Non-Fic", 10)
	if err != nil || len(hits) != 1 || hits[0].CardID != card.ID {
		t.Fatalf("hyphenated prefix search = %v, %v", hits, err)
	}
}
