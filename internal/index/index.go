package index

import (
	"bruv/internal/model"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Index is the SQLite performance layer over the file-based repository.
// It is never the source of truth — it can be deleted and rebuilt at any time.
type Index struct {
	db   *sql.DB
	path string
	// cardsDir is the repo's cards directory when the index lives at the
	// standard <repo>/.bruv/index.db, else "". IndexCard stats the card
	// file there so the stored mtime is always the file's own — the value
	// IncrementalRefresh compares against — whatever the caller passed.
	cardsDir string
}

// schemaVersion is the index layout version, kept in PRAGMA user_version.
// An index with any other version is dropped and recreated on Open; the
// caller's IncrementalRefresh then repopulates it from disk (the index is
// a cache, never the source of truth).
//
//	2: cards gain an INTEGER PRIMARY KEY rid that keys cards_fts by rowid
//	   (FTS updates no longer scan the UNINDEXED id column); card
//	   descriptions are indexed; file_mtime is stored at full precision;
//	   sidecar files are no longer indexed as cards.
const schemaVersion = 2

// connParams are applied by the driver to EVERY pooled connection —
// database/sql opens more than one, so a one-off db.Exec("PRAGMA ...")
// would leave the others with busy_timeout=0 and "database is locked"
// failures that silently drift the index.
//
// busy_timeout waits out transient lock contention (concurrent runtime
// build, the installed BRUV-Server service holding the same repo, WAL
// recovery after an unclean shutdown) instead of failing instantly; it
// must be in effect BEFORE journal_mode, because the WAL switch is itself
// the statement that hits the lock — the driver always applies
// busy_timeout first. WAL gives crash safety and readers that see the
// last committed state while a write transaction is open.
//
// _txlock=immediate: every transaction in this package writes, so take
// the write lock at BEGIN, where busy_timeout covers it, instead of a
// mid-transaction read→write upgrade that fails with SQLITE_BUSY.
var connParams = strings.Join([]string{
	"_pragma=busy_timeout(5000)",
	"_pragma=journal_mode(WAL)",
	"_pragma=synchronous(NORMAL)",
	"_txlock=immediate",
}, "&")

// Open opens (or creates) the SQLite index at the given path with WAL mode.
func Open(dbPath string) (*Index, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	if strings.ContainsRune(dbPath, '?') {
		return nil, fmt.Errorf("open index db: path %q contains '?'", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath+"?"+connParams)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}

	idx := &Index{db: db, path: dbPath}
	if filepath.Base(dir) == ".bruv" {
		idx.cardsDir = filepath.Join(filepath.Dir(dir), "cards")
	}
	if err := idx.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}

	return idx, nil
}

// Close closes the index database.
func (idx *Index) Close() error {
	if idx.db != nil {
		return idx.db.Close()
	}
	return nil
}

const schemaSQL = `
	CREATE TABLE IF NOT EXISTS cards (
		rid             INTEGER PRIMARY KEY,
		id              TEXT NOT NULL UNIQUE,
		type            TEXT NOT NULL,
		title           TEXT NOT NULL,
		context_level   TEXT NOT NULL DEFAULT 'project',
		due_date        TEXT,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		file_mtime      TEXT NOT NULL,
		project_context TEXT NOT NULL DEFAULT '',
		agent_enabled   BOOLEAN DEFAULT 0,
		agent_status    TEXT DEFAULT '',
		next_run_at     TEXT DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS pins (
		card_id     TEXT NOT NULL,
		project_id  TEXT NOT NULL,
		category_id TEXT NOT NULL,
		position    INTEGER NOT NULL DEFAULT 0,
		pinned_at   TEXT NOT NULL,
		PRIMARY KEY (card_id, project_id, category_id)
	);

	CREATE TABLE IF NOT EXISTS tags (
		card_id TEXT NOT NULL,
		tag     TEXT NOT NULL,
		PRIMARY KEY (card_id, tag)
	);

	CREATE INDEX IF NOT EXISTS idx_pins_project ON pins(project_id, category_id);
	CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
	CREATE INDEX IF NOT EXISTS idx_cards_type ON cards(type);
	CREATE INDEX IF NOT EXISTS idx_cards_updated ON cards(updated_at);

	-- rowid = cards.rid; id is kept to guard the join against stray rows.
	CREATE VIRTUAL TABLE IF NOT EXISTS cards_fts USING fts5(
		id UNINDEXED,
		title,
		content,
		tags,
		tokenize='porter unicode61'
	);
	`

// migrate brings the schema to schemaVersion. A mismatched version drops
// every index table and recreates it empty, in one transaction so a
// concurrent opener sees the old layout or the new, never a half-built
// one; the caller's refresh then refills it from disk.
func (idx *Index) migrate() error {
	return idx.inTx(func(tx *sql.Tx) error {
		var version int
		if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		if version != schemaVersion {
			for _, table := range []string{"cards_fts", "cards", "tags", "pins"} {
				if _, err := tx.Exec("DROP TABLE IF EXISTS " + table); err != nil {
					return fmt.Errorf("drop %s: %w", table, err)
				}
			}
		}
		if _, err := tx.Exec(schemaSQL); err != nil {
			return err
		}
		if version != schemaVersion {
			if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
				return fmt.Errorf("set schema version: %w", err)
			}
		}
		return nil
	})
}

// inTx runs fn in one write transaction, committing only if it succeeds.
func (idx *Index) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Card Indexing ---

// IndexCard inserts or replaces a card in the index.
// projectContext is an optional string of brand/stream/project names that gets
// prepended to the FTS content so cards are searchable by project name.
// fileMtime is a fallback: when the card's file can be stat'd its own
// mtime is stored instead, so IncrementalRefresh recognises the card as
// up to date rather than reindexing it on every run.
func (idx *Index) IndexCard(card *model.Card, fileMtime time.Time, projectContext string) error {
	if m, ok := idx.cardFileMtime(card.ID); ok {
		fileMtime = m
	}
	return idx.inTx(func(tx *sql.Tx) error {
		return indexCardTx(tx, card, fileMtime, projectContext)
	})
}

// cardFileMtime stats cards/<id>.json next to the index.
func (idx *Index) cardFileMtime(cardID string) (time.Time, bool) {
	if idx.cardsDir == "" {
		return time.Time{}, false
	}
	name := cardID + model.CardFileExt
	if _, ok := model.CardIDFromFileName(name); !ok {
		return time.Time{}, false
	}
	m, err := statMtime(filepath.Join(idx.cardsDir, name))
	return m, err == nil
}

// statMtime is the one way the index reads a card file's mtime. It stats
// the file rather than trusting os.DirEntry.Info: on Windows that comes
// from the directory listing, whose timestamps can lag the file's own —
// the two would never compare equal and the card would reindex forever.
func statMtime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime().UTC(), nil
}

// indexCardTx upserts one card's row, tags and FTS entry. The upsert keeps
// the row's rid (and the agent columns UpdateAgentIndex writes) stable, so
// the FTS entry is replaced by rowid — an O(log n) lookup.
func indexCardTx(tx *sql.Tx, card *model.Card, fileMtime time.Time, projectContext string) error {
	var rid int64
	err := tx.QueryRow(`
		INSERT INTO cards (id, type, title, context_level, due_date, created_at, updated_at, file_mtime, project_context)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			title = excluded.title,
			context_level = excluded.context_level,
			due_date = excluded.due_date,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			file_mtime = excluded.file_mtime,
			project_context = excluded.project_context
		RETURNING rid`,
		card.ID, card.Type, card.Title, string(card.ContextLevel),
		formatNullableTime(card.DueDate),
		card.CreatedAt.Format(time.RFC3339),
		card.UpdatedAt.Format(time.RFC3339),
		formatMtime(fileMtime),
		projectContext,
	).Scan(&rid)
	if err != nil {
		return fmt.Errorf("upsert card: %w", err)
	}

	// Rebuild tags for this card
	if _, err := tx.Exec("DELETE FROM tags WHERE card_id = ?", card.ID); err != nil {
		return fmt.Errorf("delete old tags: %w", err)
	}
	for _, tag := range card.Tags {
		if _, err := tx.Exec("INSERT OR IGNORE INTO tags (card_id, tag) VALUES (?, ?)", card.ID, tag); err != nil {
			return fmt.Errorf("insert tag: %w", err)
		}
	}

	// Rebuild FTS entry
	if _, err := tx.Exec("DELETE FROM cards_fts WHERE rowid = ?", rid); err != nil {
		return fmt.Errorf("delete old fts: %w", err)
	}

	// Build searchable content from fields, prepend project context
	content := buildSearchContent(card)
	if projectContext != "" {
		content = projectContext + " " + content
	}

	if _, err := tx.Exec("INSERT INTO cards_fts (rowid, id, title, content, tags) VALUES (?, ?, ?, ?, ?)",
		rid, card.ID, card.Title, content, strings.Join(card.Tags, " ")); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}
	return nil
}

// GetCardProjectContext returns the stored project context for a card, or "" if not found.
func (idx *Index) GetCardProjectContext(cardID string) string {
	var ctx string
	err := idx.db.QueryRow("SELECT project_context FROM cards WHERE id = ?", cardID).Scan(&ctx)
	if err != nil {
		return ""
	}
	return ctx
}

// RemoveCard removes a card from the index entirely.
func (idx *Index) RemoveCard(cardID string) error {
	return idx.inTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			"DELETE FROM cards_fts WHERE rowid = (SELECT rid FROM cards WHERE id = ?)",
			"DELETE FROM cards WHERE id = ?",
			"DELETE FROM tags WHERE card_id = ?",
			"DELETE FROM pins WHERE card_id = ?",
		} {
			if _, err := tx.Exec(stmt, cardID); err != nil {
				return fmt.Errorf("remove card: %w", err)
			}
		}
		return nil
	})
}

// UpdateAgentIndex updates the agent-related columns for a card in the index.
func (idx *Index) UpdateAgentIndex(cardID string, enabled bool, status string, nextRunAt string) error {
	_, err := idx.db.Exec(
		"UPDATE cards SET agent_enabled = ?, agent_status = ?, next_run_at = ? WHERE id = ?",
		enabled, status, nextRunAt, cardID,
	)
	return err
}

// --- Pin Indexing ---

// IndexPins replaces all pin entries for a card.
func (idx *Index) IndexPins(cardID string, pins []model.Pin) error {
	return idx.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM pins WHERE card_id = ?", cardID); err != nil {
			return err
		}
		return insertPinsTx(tx, pins)
	})
}

// insertPinsTx inserts one card's pins.
func insertPinsTx(tx *sql.Tx, pins []model.Pin) error {
	// A pin file naming one category twice must not poison the index for
	// the whole card (every later pin write for it would fail on the
	// unique key): the first occurrence wins, matching repo.Revalidate.
	seen := make(map[string]bool, len(pins))
	for _, p := range pins {
		key := p.ProjectID + "\x00" + p.CategoryID
		if seen[key] {
			continue
		}
		seen[key] = true
		_, err := tx.Exec(`
			INSERT INTO pins (card_id, project_id, category_id, position, pinned_at)
			VALUES (?, ?, ?, ?, ?)`,
			p.CardID, p.ProjectID, p.CategoryID, p.Position,
			p.PinnedAt.Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Queries ---

// SearchResult represents a single search hit.
type SearchResult struct {
	CardID         string
	Title          string
	Type           string
	Rank           float64
	ProjectContext string
}

// Search performs a full-text search across the index.
// Each word in the query gets a prefix wildcard so partial matches work (e.g. "Prem" → "Prem*").
func (idx *Index) Search(query string, limit int) ([]SearchResult, error) {
	return idx.search(query, limit, false)
}

// SearchOrphanedCards performs a full-text search limited to orphaned (inbox) cards.
func (idx *Index) SearchOrphanedCards(query string, limit int) ([]SearchResult, error) {
	return idx.search(query, limit, true)
}

// search runs one FTS query; orphanedOnly limits it to cards with no pins.
func (idx *Index) search(query string, limit int, orphanedOnly bool) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}
	ftsQuery := ftsPrefixQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	sqlQuery := `
		SELECT f.id, f.title, c.type, rank, c.project_context
		FROM cards_fts f
		JOIN cards c ON c.rid = f.rowid AND c.id = f.id
		WHERE cards_fts MATCH ?`
	if orphanedOnly {
		sqlQuery += `
		  AND NOT EXISTS (SELECT 1 FROM pins p WHERE p.card_id = c.id)`
	}
	sqlQuery += `
		ORDER BY rank
		LIMIT ?`

	rows, err := idx.db.Query(sqlQuery, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.CardID, &r.Title, &r.Type, &r.Rank, &r.ProjectContext); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ftsPrefixQuery turns free text into an FTS5 MATCH expression, or "" for
// blank input. Each word becomes a quoted FTS5 string with a prefix
// wildcard: `"non-fiction"*`. Unquoted, FTS5 reads `-` as NOT, `:` as a
// column filter and so on — "Non-Fiction" failed with "no such column:
// Fiction" (field report 2026-09-20). A double quote inside a term is
// doubled, which is FTS5's own escape.
func ftsPrefixQuery(query string) string {
	words := strings.Fields(query)
	for i, w := range words {
		w = strings.TrimRight(w, "*")
		words[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(words, " ")
}

// ListCardIDsInCategory returns card IDs pinned to a specific category,
// ordered by position. Keys on category_id alone — older rows where
// project_id != category_id (legacy buggy writes) are still returned,
// and DISTINCT collapses any duplicate (card_id, *, category_id) rows
// that may exist from cross-version writes.
func (idx *Index) ListCardIDsInCategory(categoryID string) ([]string, error) {
	return idx.queryIDs(`
		SELECT DISTINCT card_id FROM pins
		WHERE category_id = ?
		ORDER BY position`,
		categoryID,
	)
}

// ListCardIDsByType returns card IDs of a given type.
func (idx *Index) ListCardIDsByType(cardType string) ([]string, error) {
	return idx.queryIDs("SELECT id FROM cards WHERE type = ? ORDER BY updated_at DESC", cardType)
}

// ListCardIDsByTag returns card IDs that have a given tag.
func (idx *Index) ListCardIDsByTag(tag string) ([]string, error) {
	return idx.queryIDs("SELECT card_id FROM tags WHERE tag = ?", tag)
}

// ListOrphanedCardIDs returns IDs of cards that have no pins.
func (idx *Index) ListOrphanedCardIDs() ([]string, error) {
	return idx.queryIDs(`
		SELECT c.id FROM cards c
		WHERE NOT EXISTS (SELECT 1 FROM pins p WHERE p.card_id = c.id)
		ORDER BY c.updated_at DESC`)
}

// queryIDs runs a query selecting a single text column.
func (idx *Index) queryIDs(query string, args ...any) ([]string, error) {
	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// indexedCard is what IncrementalRefresh needs to know about an indexed card.
type indexedCard struct {
	mtime          time.Time
	projectContext string
}

// indexedCards returns the refresh bookkeeping for every indexed card in
// one query. An unparseable mtime is left zero, so the card is reindexed.
func (idx *Index) indexedCards() (map[string]indexedCard, error) {
	rows, err := idx.db.Query("SELECT id, file_mtime, project_context FROM cards")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make(map[string]indexedCard)
	for rows.Next() {
		var id, mtime string
		var c indexedCard
		if err := rows.Scan(&id, &mtime, &c.projectContext); err != nil {
			return nil, err
		}
		c.mtime, _ = time.Parse(time.RFC3339Nano, mtime)
		cards[id] = c
	}
	return cards, rows.Err()
}

// CardCount returns the number of cards in the index.
func (idx *Index) CardCount() (int, error) {
	var count int
	err := idx.db.QueryRow("SELECT COUNT(*) FROM cards").Scan(&count)
	return count, err
}

// --- Helpers ---

func formatNullableTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

// formatMtime stores a file mtime at full precision: IncrementalRefresh
// compares it for equality with the file's nanosecond ModTime, which a
// seconds-precision RFC3339 value could never match.
func formatMtime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// buildSearchContent flattens the searchable text out of a card for the
// FTS index (the title has its own column): the intrinsic description,
// then every block — text/url block values contribute their string
// verbatim; checklist/list block items contribute their per-item text.
// Anything else (numbers, dates, media URLs) is intentionally skipped —
// searching for "42" across every numeric field would be more noise than
// signal.
func buildSearchContent(card *model.Card) string {
	var parts []string
	if card.Description != "" {
		parts = append(parts, card.Description)
	}
	for _, b := range card.Blocks {
		switch b.Type {
		case model.BlockText, model.BlockURL:
			if s, ok := b.Value.(string); ok && s != "" {
				parts = append(parts, s)
			}
		case model.BlockChecklist, model.BlockList:
			items, ok := b.Value.([]any)
			if !ok {
				continue
			}
			for _, raw := range items {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := m["text"].(string); t != "" {
					parts = append(parts, t)
				}
			}
		case model.BlockWorkspaceFiles:
			// A card is findable by the files it's about.
			items, ok := b.Value.([]any)
			if !ok {
				continue
			}
			for _, raw := range items {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if p, _ := m["path"].(string); p != "" {
					parts = append(parts, p)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}
