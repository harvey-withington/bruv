package index

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bruv/internal/model"
)

// The intrinsic description is searchable (it lives outside the blocks
// since the description refactor), and the title still is.
func TestSearchFindsDescriptionAndTitle(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now().UTC()
	card := &model.Card{
		ID: "c1", Type: "brainstorm", Title: "Quarterly roadmap",
		Description: "Covers the zeppelin launch",
		CreatedAt:   now, UpdatedAt: now, ContextLevel: model.ContextProject,
	}
	if err := idx.IndexCard(card, now, ""); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"zeppelin", "roadmap"} {
		res, err := idx.Search(q, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if len(res) != 1 || res[0].CardID != "c1" {
			t.Errorf("Search(%q) = %v, want [c1]", q, res)
		}
	}
}

// Inbox search got the same FTS quoting as Search: a hyphenated word is
// a term, not "no such column".
func TestSearchOrphanedQuotesOperatorCharacters(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now().UTC()
	for _, c := range []*model.Card{
		{ID: "loose", Type: "brainstorm", Title: "Non-Fiction shelf", CreatedAt: now, UpdatedAt: now},
		{ID: "pinned", Type: "brainstorm", Title: "Non-Fiction pinned", CreatedAt: now, UpdatedAt: now},
	} {
		if err := idx.IndexCard(c, now, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := idx.IndexPins("pinned", []model.Pin{{CardID: "pinned", ProjectID: "cat", CategoryID: "cat", PinnedAt: now}}); err != nil {
		t.Fatal(err)
	}
	res, err := idx.SearchOrphanedCards("Non-Fiction", 10)
	if err != nil {
		t.Fatalf("SearchOrphanedCards: %v", err)
	}
	if len(res) != 1 || res[0].CardID != "loose" {
		t.Errorf("orphaned search = %v, want [loose]", res)
	}
}

// Sidecars in the cards directory are not cards: no phantom id="" row
// (Inbox count off by one), and not reindexed/removed on every refresh.
func TestSidecarFilesAreNotIndexed(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)
	card, err := r.CreateCard("brainstorm", "Real card")
	if err != nil {
		t.Fatal(err)
	}
	cardsDir := filepath.Join(r.Root, "cards")
	for _, name := range []string{
		card.ID + ".comments.json",
		card.ID + ".agent.json",
		card.ID + ".messages.json",
		card.ID + ".json.tmp",
	} {
		if err := os.WriteFile(filepath.Join(cardsDir, name), []byte(`{"comments":[]}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := idx.FullRebuild(r.Root); err != nil {
		t.Fatal(err)
	}
	orphans, err := idx.ListOrphanedCardIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0] != card.ID {
		t.Errorf("orphans after rebuild = %q, want [%s]", orphans, card.ID)
	}

	stats, err := idx.IncrementalRefresh(r.Root)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CardsIndexed != 0 || stats.CardsRemoved != 0 || stats.CardsSkipped != 1 {
		t.Errorf("refresh stats = %+v, want 1 skipped and nothing indexed/removed", stats)
	}
}

// An unchanged repo refreshes without reindexing anything — including a
// card last indexed by a service write that passed time.Now() instead of
// the file's mtime.
func TestIncrementalRefreshSkipsUnchangedCards(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)
	c1, _ := r.CreateCard("brainstorm", "One")
	r.CreateCard("brainstorm", "Two")
	if _, err := idx.FullRebuild(r.Root); err != nil {
		t.Fatal(err)
	}

	updated, err := r.UpdateCard(c1.ID, func(c *model.Card) { c.Title = "One edited" })
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexCard(updated, time.Now().Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}

	stats, err := idx.IncrementalRefresh(r.Root)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CardsIndexed != 0 || stats.CardsSkipped != 2 {
		t.Errorf("refresh stats = %+v, want 0 indexed / 2 skipped", stats)
	}
}

// Reindexing a card replaces its FTS row (keyed by rowid) rather than
// adding another one, and removing it leaves no FTS row behind.
func TestReindexReplacesFTSRow(t *testing.T) {
	idx, _ := setupTestIndex(t)
	now := time.Now().UTC()
	card := &model.Card{ID: "c1", Type: "brainstorm", Title: "alpha", CreatedAt: now, UpdatedAt: now}
	for _, title := range []string{"alpha", "beta", "gamma"} {
		card.Title = title
		if err := idx.IndexCard(card, now, ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, idx.db, "cards_fts"); n != 1 {
		t.Errorf("fts rows = %d, want 1", n)
	}
	if res, _ := idx.Search("alpha", 10); len(res) != 0 {
		t.Errorf("stale title still matches: %v", res)
	}
	if err := idx.RemoveCard("c1"); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, idx.db, "cards_fts"); n != 0 {
		t.Errorf("fts rows after remove = %d, want 0", n)
	}
}

// busy_timeout applies to every pooled connection, not just the first.
func TestPragmasApplyToEveryConnection(t *testing.T) {
	idx, _ := setupTestIndex(t)
	ctx := context.Background()
	var conns []*sql.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for i := 0; i < 3; i++ {
		c, err := idx.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		var timeout int
		var mode string
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if timeout != 5000 || mode != "wal" {
			t.Errorf("conn %d: busy_timeout=%d journal_mode=%s, want 5000/wal", i, timeout, mode)
		}
	}
}

// An index written by an older layout is dropped and recreated on Open
// (the refresh on open then refills it), so v1 rows — second-precision
// mtimes, unindexed descriptions, phantom sidecar rows — don't linger.
func TestOpenRecreatesOutdatedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".bruv", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`
		CREATE TABLE cards (id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL,
			context_level TEXT NOT NULL DEFAULT 'project', due_date TEXT, created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL, file_mtime TEXT NOT NULL);
		INSERT INTO cards VALUES ('', '', '', 'project', NULL, '', '', '');
		CREATE VIRTUAL TABLE cards_fts USING fts5(id UNINDEXED, title, content, tags);`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close()
	if n, _ := idx.CardCount(); n != 0 {
		t.Errorf("outdated rows survived migration: %d cards", n)
	}
	var version int
	if err := idx.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("user_version = %d, want %d", version, schemaVersion)
	}
	now := time.Now().UTC()
	if err := idx.IndexCard(&model.Card{ID: "c1", Type: "t", Title: "x", CreatedAt: now, UpdatedAt: now}, now, ""); err != nil {
		t.Fatalf("IndexCard on migrated schema: %v", err)
	}

	// Reopening a current index keeps its data.
	idx.Close()
	idx2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx2.Close()
	if n, _ := idx2.CardCount(); n != 1 {
		t.Errorf("reopen of a current index lost data: %d cards", n)
	}
}

// A category's pins never read as empty while a refresh or rebuild runs
// (category move/copy lists cards from the index; an empty read orphaned
// every card in the category).
func TestPinsNeverEmptyDuringRefresh(t *testing.T) {
	r, idx := setupTestRepoWithIndex(t)
	r.CreateBrand("B")
	r.CreateStream("b", "S")
	r.CreateProject("b", "s", "P")
	cat, err := r.CreateCategory("b", "s", "p", "Todo", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"one", "two", "three"} {
		c, _ := r.CreateCard("brainstorm", title)
		if err := r.PinCard(c.ID, cat.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := idx.FullRebuild(r.Root); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var emptyReads atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			ids, err := idx.ListCardIDsInCategory(cat.ID)
			if err == nil && len(ids) == 0 {
				emptyReads.Add(1)
			}
		}
	}()
	for i := 0; i < 15; i++ {
		if _, err := idx.IncrementalRefresh(r.Root); err != nil {
			t.Error(err)
		}
		if _, err := idx.FullRebuild(r.Root); err != nil {
			t.Error(err)
		}
	}
	stop.Store(true)
	wg.Wait()
	if n := emptyReads.Load(); n != 0 {
		t.Errorf("category read as empty %d times during refresh", n)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
