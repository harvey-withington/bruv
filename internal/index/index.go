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
}

// Open opens (or creates) the SQLite index at the given path with WAL mode.
func Open(dbPath string) (*Index, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}

	// Enable WAL mode for crash safety and concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Performance tuning
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}

	idx := &Index{db: db, path: dbPath}
	if err := idx.createTables(); err != nil {
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

// createTables sets up the index schema.
func (idx *Index) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS cards (
		id              TEXT PRIMARY KEY,
		type            TEXT NOT NULL,
		title           TEXT NOT NULL,
		context_level   TEXT NOT NULL DEFAULT 'project',
		due_date        TEXT,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		file_mtime      TEXT NOT NULL,
		project_context TEXT NOT NULL DEFAULT ''
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

	CREATE VIRTUAL TABLE IF NOT EXISTS cards_fts USING fts5(
		id UNINDEXED,
		title,
		content,
		tags,
		tokenize='porter unicode61'
	);
	`
	_, err := idx.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add project_context column if missing (existing databases)
	idx.db.Exec("ALTER TABLE cards ADD COLUMN project_context TEXT NOT NULL DEFAULT ''")

	// Migration: add agent columns for Phase 2 scheduler support
	idx.db.Exec("ALTER TABLE cards ADD COLUMN agent_enabled BOOLEAN DEFAULT 0")
	idx.db.Exec("ALTER TABLE cards ADD COLUMN agent_status TEXT DEFAULT ''")
	idx.db.Exec("ALTER TABLE cards ADD COLUMN next_run_at TEXT DEFAULT ''")
	idx.db.Exec("CREATE INDEX IF NOT EXISTS idx_cards_agent_next_run ON cards(next_run_at) WHERE agent_enabled = 1")

	return nil
}

// --- Card Indexing ---

// IndexCard inserts or replaces a card in the index.
// projectContext is an optional string of brand/stream/project names that gets
// prepended to the FTS content so cards are searchable by project name.
func (idx *Index) IndexCard(card *model.Card, fileMtime time.Time, projectContext string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Upsert card metadata
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO cards (id, type, title, context_level, due_date, created_at, updated_at, file_mtime, project_context)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		card.ID, card.Type, card.Title, string(card.ContextLevel),
		formatNullableTime(card.DueDate),
		card.CreatedAt.Format(time.RFC3339),
		card.UpdatedAt.Format(time.RFC3339),
		fileMtime.Format(time.RFC3339),
		projectContext,
	)
	if err != nil {
		return fmt.Errorf("upsert card: %w", err)
	}

	// Rebuild tags for this card
	if _, err := tx.Exec("DELETE FROM tags WHERE card_id = ?", card.ID); err != nil {
		return fmt.Errorf("delete old tags: %w", err)
	}
	for _, tag := range card.Tags {
		if _, err := tx.Exec("INSERT INTO tags (card_id, tag) VALUES (?, ?)", card.ID, tag); err != nil {
			return fmt.Errorf("insert tag: %w", err)
		}
	}

	// Rebuild FTS entry
	if _, err := tx.Exec("DELETE FROM cards_fts WHERE id = ?", card.ID); err != nil {
		return fmt.Errorf("delete old fts: %w", err)
	}

	// Build searchable content from fields, prepend project context
	content := buildSearchContent(card)
	if projectContext != "" {
		content = projectContext + " " + content
	}
	tagsStr := joinTags(card.Tags)

	if _, err := tx.Exec("INSERT INTO cards_fts (id, title, content, tags) VALUES (?, ?, ?, ?)",
		card.ID, card.Title, content, tagsStr); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}

	return tx.Commit()
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
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM cards WHERE id = ?", cardID)
	tx.Exec("DELETE FROM tags WHERE card_id = ?", cardID)
	tx.Exec("DELETE FROM pins WHERE card_id = ?", cardID)
	tx.Exec("DELETE FROM cards_fts WHERE id = ?", cardID)

	return tx.Commit()
}

// DueAgent represents a card with an agent that is due to run.
type DueAgent struct {
	CardID    string
	NextRunAt time.Time
}

// QueryDueAgents returns agent cards that are due to run.
func (idx *Index) QueryDueAgents(now time.Time) ([]DueAgent, error) {
	rows, err := idx.db.Query(`
		SELECT id, next_run_at FROM cards
		WHERE agent_enabled = 1
		  AND agent_status != 'running'
		  AND next_run_at != ''
		  AND next_run_at <= ?
		ORDER BY next_run_at ASC
		LIMIT 10`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("query due agents: %w", err)
	}
	defer rows.Close()

	var agents []DueAgent
	for rows.Next() {
		var da DueAgent
		var nra string
		if err := rows.Scan(&da.CardID, &nra); err != nil {
			return nil, fmt.Errorf("scan due agent: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, nra); err == nil {
			da.NextRunAt = t
		}
		agents = append(agents, da)
	}
	return agents, rows.Err()
}

// ResetStaleAgentStatus clears any 'running' status left over from a previous crash.
// Called on startup before the scheduler begins polling.
func (idx *Index) ResetStaleAgentStatus() {
	idx.db.Exec("UPDATE cards SET agent_status = 'idle' WHERE agent_status = 'running'")
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
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM pins WHERE card_id = ?", cardID); err != nil {
		return err
	}

	for _, p := range pins {
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

	return tx.Commit()
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
	if limit <= 0 {
		limit = 50
	}

	// Build prefix query: split into words, append * to each for prefix matching
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}
	for i, w := range words {
		w = strings.TrimRight(w, "*")
		words[i] = w + "*"
	}
	ftsQuery := strings.Join(words, " ")

	rows, err := idx.db.Query(`
		SELECT f.id, f.title, c.type, rank, c.project_context
		FROM cards_fts f
		JOIN cards c ON c.id = f.id
		WHERE cards_fts MATCH ?
		ORDER BY rank
		LIMIT ?`,
		ftsQuery, limit,
	)
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

// SearchOrphanedCards performs a full-text search limited to orphaned (inbox) cards.
func (idx *Index) SearchOrphanedCards(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}
	for i, w := range words {
		w = strings.TrimRight(w, "*")
		words[i] = w + "*"
	}
	ftsQuery := strings.Join(words, " ")

	rows, err := idx.db.Query(`
		SELECT f.id, f.title, c.type, rank, c.project_context
		FROM cards_fts f
		JOIN cards c ON c.id = f.id
		LEFT JOIN pins p ON p.card_id = c.id
		WHERE cards_fts MATCH ? AND p.card_id IS NULL
		ORDER BY rank
		LIMIT ?`,
		ftsQuery, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search orphaned query: %w", err)
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

// ListCardIDsInCategory returns card IDs pinned to a specific project/category, ordered by position.
func (idx *Index) ListCardIDsInCategory(projectID, categoryID string) ([]string, error) {
	rows, err := idx.db.Query(`
		SELECT card_id FROM pins
		WHERE project_id = ? AND category_id = ?
		ORDER BY position`,
		projectID, categoryID,
	)
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

// ListCardIDsByType returns card IDs of a given type.
func (idx *Index) ListCardIDsByType(cardType string) ([]string, error) {
	rows, err := idx.db.Query("SELECT id FROM cards WHERE type = ? ORDER BY updated_at DESC", cardType)
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

// ListCardIDsByTag returns card IDs that have a given tag.
func (idx *Index) ListCardIDsByTag(tag string) ([]string, error) {
	rows, err := idx.db.Query("SELECT card_id FROM tags WHERE tag = ?", tag)
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

// GetCardMtime returns the indexed file mtime for a card, or zero time if not found.
func (idx *Index) GetCardMtime(cardID string) (time.Time, error) {
	var mtimeStr string
	err := idx.db.QueryRow("SELECT file_mtime FROM cards WHERE id = ?", cardID).Scan(&mtimeStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, mtimeStr)
}

// ListIndexedCardIDs returns all card IDs currently in the index.
func (idx *Index) ListIndexedCardIDs() ([]string, error) {
	rows, err := idx.db.Query("SELECT id FROM cards")
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

// ListOrphanedCardIDs returns IDs of cards that have no pins.
func (idx *Index) ListOrphanedCardIDs() ([]string, error) {
	rows, err := idx.db.Query(`
		SELECT c.id FROM cards c
		LEFT JOIN pins p ON p.card_id = c.id
		WHERE p.card_id IS NULL
		ORDER BY c.updated_at DESC`)
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

// buildSearchContent flattens the searchable text out of a card's
// blocks for the FTS index. Walks every block: text/url block values
// contribute their string verbatim; checklist/list block items
// contribute their per-item text. Anything else (numbers, dates,
// media URLs) is intentionally skipped — searching for "42" across
// every numeric field would be more noise than signal.
func buildSearchContent(card *model.Card) string {
	var parts []string
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
		}
	}
	return strings.Join(parts, " ")
}

func joinTags(tags []string) string {
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += " "
		}
		result += t
	}
	return result
}
