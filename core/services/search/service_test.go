package search

import (
	"bruv/internal/index"
	"bruv/internal/repo"
	"path/filepath"
	"testing"
)

// stubDeps is an in-test host adapter. Establishes the pattern: every
// service ships a Deps interface and tests construct it with whatever
// stub fits the scenario — no App, no Wails, no harness.
type stubDeps struct {
	repo *repo.Repository
	idx  *index.Index
}

func (s stubDeps) Repo() *repo.Repository { return s.repo }
func (s stubDeps) Index() *index.Index    { return s.idx }

// TestServiceWithoutIndexReturnsFriendlyErrors covers the "no repo open"
// state. Methods must not panic and must return a clear error.
func TestServiceWithoutIndexReturnsFriendlyErrors(t *testing.T) {
	svc := New(stubDeps{})

	if got := svc.GetCardProjectContext("nope"); got != "" {
		t.Errorf("GetCardProjectContext with nil index = %q, want empty", got)
	}

	if _, err := svc.SearchCards("q", 10); err == nil {
		t.Error("SearchCards with nil index: expected error, got nil")
	}
	if _, err := svc.ListCardIDsByTag("t"); err == nil {
		t.Error("ListCardIDsByTag with nil index: expected error, got nil")
	}
	if _, err := svc.RebuildIndex(); err == nil {
		t.Error("RebuildIndex with nil deps: expected error, got nil")
	}
}

// TestServiceWithRepoAndIndex exercises the happy path — service built
// with real repo+index, operating on an empty repository. Proves the
// extraction keeps the original behaviour: no index errors, methods
// return the empty results index.Search would return directly.
func TestServiceWithRepoAndIndex(t *testing.T) {
	dir := t.TempDir()
	r, err := repo.Init(dir, "test")
	if err != nil {
		t.Fatalf("repo.Init: %v", err)
	}
	idx, err := index.Open(filepath.Join(r.Root, ".bruv", "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	svc := New(stubDeps{repo: r, idx: idx})

	results, err := svc.SearchCards("anything", 10)
	if err != nil {
		t.Fatalf("SearchCards: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty repo SearchCards = %d results, want 0", len(results))
	}

	ids, err := svc.ListOrphanedCardIDs()
	if err != nil {
		t.Fatalf("ListOrphanedCardIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("empty repo ListOrphanedCardIDs = %d, want 0", len(ids))
	}

	if _, err := svc.RebuildIndex(); err != nil {
		t.Errorf("RebuildIndex on empty repo: %v", err)
	}
}
