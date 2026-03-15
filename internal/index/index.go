package index

import (
	"bruv/internal/model"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
		id            TEXT PRIMARY KEY,
		type          TEXT NOT NULL,
		title         TEXT NOT NULL,
		context_level TEXT NOT NULL DEFAULT 'project',
		due_date      TEXT,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL,
		file_mtime    TEXT NOT NULL
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
	return err
}

// --- Card Indexing ---

// IndexCard inserts or replaces a card in the index.
func (idx *Index) IndexCard(card *model.Card, fileMtime time.Time) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Upsert card metadata
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO cards (id, type, title, context_level, due_date, created_at, updated_at, file_mtime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		card.ID, card.Type, card.Title, string(card.ContextLevel),
		formatNullableTime(card.DueDate),
		card.CreatedAt.Format(time.RFC3339),
		card.UpdatedAt.Format(time.RFC3339),
		fileMtime.Format(time.RFC3339),
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

	// Build searchable content from fields
	content := buildSearchContent(card)
	tagsStr := joinTags(card.Tags)

	if _, err := tx.Exec("INSERT INTO cards_fts (id, title, content, tags) VALUES (?, ?, ?, ?)",
		card.ID, card.Title, content, tagsStr); err != nil {
		return fmt.Errorf("insert fts: %w", err)
	}

	return tx.Commit()
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
	CardID string
	Title  string
	Type   string
	Rank   float64
}

// Search performs a full-text search across the index.
func (idx *Index) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := idx.db.Query(`
		SELECT f.id, f.title, c.type, rank
		FROM cards_fts f
		JOIN cards c ON c.id = f.id
		WHERE cards_fts MATCH ?
		ORDER BY rank
		LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.CardID, &r.Title, &r.Type, &r.Rank); err != nil {
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

func buildSearchContent(card *model.Card) string {
	var parts []string
	for _, v := range card.Fields {
		if s, ok := v.(string); ok {
			parts = append(parts, s)
		}
	}
	for _, item := range card.Checklist {
		parts = append(parts, item.Text)
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
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
