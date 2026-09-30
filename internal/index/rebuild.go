package index

import (
	"bruv/internal/model"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RebuildStats tracks what happened during a rebuild.
type RebuildStats struct {
	CardsIndexed int
	CardsRemoved int
	CardsSkipped int
	PinsIndexed  int
	Duration     time.Duration
}

// FullRebuild drops all index data and rebuilds from the file store.
//
// Files are read first and the index is swapped in one transaction, so
// concurrent readers see the old index or the new one — never empty
// tables mid-rebuild — and the write lock is held only for the SQL.
func (idx *Index) FullRebuild(repoRoot string) (*RebuildStats, error) {
	start := time.Now()
	stats := &RebuildStats{}

	// Build cardID → project context mapping from the filesystem
	cardContextMap := buildCardContextMap(repoRoot)

	cards, err := readCardFiles(filepath.Join(repoRoot, "cards"))
	if err != nil {
		return nil, fmt.Errorf("index cards: %w", err)
	}
	pins, err := readPinFiles(filepath.Join(repoRoot, "pins"))
	if err != nil {
		return nil, fmt.Errorf("index pins: %w", err)
	}

	err = idx.inTx(func(tx *sql.Tx) error {
		for _, table := range []string{"cards", "tags", "pins", "cards_fts"} {
			if _, err := tx.Exec("DELETE FROM " + table); err != nil {
				return fmt.Errorf("clear table %s: %w", table, err)
			}
		}
		for _, dc := range cards {
			// One bad card must not sink the rebuild; SQLite rolls back
			// just the failed statement, not the transaction.
			if err := indexCardTx(tx, dc.card, dc.mtime, cardContextMap[dc.card.ID]); err != nil {
				continue
			}
			stats.CardsIndexed++
		}
		return replacePinsTx(tx, pins, stats)
	})
	if err != nil {
		return nil, err
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// IncrementalRefresh updates the index for cards whose file mtime has changed
// since they were last indexed. Also removes index entries for deleted cards.
func (idx *Index) IncrementalRefresh(repoRoot string) (*RebuildStats, error) {
	start := time.Now()
	stats := &RebuildStats{}

	// Build cardID → project context mapping from the filesystem
	cardContextMap := buildCardContextMap(repoRoot)

	indexed, err := idx.indexedCards()
	if err != nil {
		return nil, fmt.Errorf("list indexed cards: %w", err)
	}

	cardsDir := filepath.Join(repoRoot, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cards dir: %w", err)
	}

	// Card IDs currently on disk — counted even when a file can't be read
	// right now, so a transient failure doesn't drop the card from the index.
	diskCardIDs := make(map[string]bool)
	for _, entry := range entries {
		cardID, ok := model.CardIDFromFileName(entry.Name())
		if entry.IsDir() || !ok {
			continue
		}
		diskCardIDs[cardID] = true

		path := filepath.Join(cardsDir, entry.Name())
		fileMtime, err := statMtime(path)
		if err != nil {
			continue
		}

		// Re-index only if the card is new, its file changed, or its
		// project context changed.
		ctx := cardContextMap[cardID]
		if prev, ok := indexed[cardID]; ok && prev.mtime.Equal(fileMtime) && prev.projectContext == ctx {
			stats.CardsSkipped++
			continue
		}

		card, err := readCardFile(path, cardID)
		if err != nil {
			continue
		}
		if err := idx.inTx(func(tx *sql.Tx) error {
			return indexCardTx(tx, card, fileMtime, ctx)
		}); err != nil {
			continue
		}
		stats.CardsIndexed++
	}

	// Remove index entries for cards no longer on disk
	for id := range indexed {
		if !diskCardIDs[id] {
			if err := idx.RemoveCard(id); err != nil {
				continue
			}
			stats.CardsRemoved++
		}
	}

	// Re-index all pins (fast operation, not worth incremental tracking)
	pins, err := readPinFiles(filepath.Join(repoRoot, "pins"))
	if err != nil {
		return nil, fmt.Errorf("rebuild pins: %w", err)
	}
	if err := idx.inTx(func(tx *sql.Tx) error {
		return replacePinsTx(tx, pins, stats)
	}); err != nil {
		return nil, fmt.Errorf("rebuild pins: %w", err)
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// --- Internal helpers ---

// diskCard is a card read from its file, with the file's mtime.
type diskCard struct {
	card  *model.Card
	mtime time.Time
}

// readCardFiles reads every card file (sidecars and temps excluded) in
// cardsDir. Unreadable or unparseable files are skipped.
func readCardFiles(cardsDir string) ([]diskCard, error) {
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cards []diskCard
	for _, entry := range entries {
		cardID, ok := model.CardIDFromFileName(entry.Name())
		if entry.IsDir() || !ok {
			continue
		}
		path := filepath.Join(cardsDir, entry.Name())
		mtime, err := statMtime(path)
		if err != nil {
			continue
		}
		card, err := readCardFile(path, cardID)
		if err != nil {
			continue
		}
		cards = append(cards, diskCard{card: card, mtime: mtime})
	}
	return cards, nil
}

// cardPins is one card's pin file.
type cardPins struct {
	cardID string
	pins   []model.Pin
}

// readPinFiles reads every pins/<cardID>/pins.json. Unreadable or
// unparseable pin files are skipped.
func readPinFiles(pinsDir string) ([]cardPins, error) {
	entries, err := os.ReadDir(pinsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var all []cardPins
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cardID := entry.Name()
		data, err := os.ReadFile(filepath.Join(pinsDir, cardID, "pins.json"))
		if err != nil {
			continue
		}
		var pinFile model.PinFile
		if err := json.Unmarshal(data, &pinFile); err != nil {
			continue
		}
		all = append(all, cardPins{cardID: cardID, pins: pinFile.Pins})
	}
	return all, nil
}

// replacePinsTx swaps the whole pins table for the given pin files inside
// tx, so readers (ListCardIDsInCategory, used by category move/copy) see
// the old pins or the new ones — never an empty table mid-refresh.
func replacePinsTx(tx *sql.Tx, all []cardPins, stats *RebuildStats) error {
	if _, err := tx.Exec("DELETE FROM pins"); err != nil {
		return err
	}
	for _, cp := range all {
		if err := insertPinsTx(tx, cp.pins); err != nil {
			continue
		}
		stats.PinsIndexed += len(cp.pins)
	}
	return nil
}

// buildCardContextMap walks the brand/stream/project/category hierarchy and pin
// files to build a mapping from cardID → "BrandName > StreamName > ProjectName".
// This context is stored in project_context and also prepended (space-separated) to FTS content.
func buildCardContextMap(repoRoot string) map[string]string {
	result := make(map[string]string)

	// Step 1: Build categoryID → "Brand Stream Project" from the hierarchy
	catCtx := make(map[string]string)

	brandsDir := filepath.Join(repoRoot, "brands")
	brandDirs, _ := os.ReadDir(brandsDir)
	for _, bd := range brandDirs {
		if !bd.IsDir() {
			continue
		}
		brand := readJSONName(filepath.Join(brandsDir, bd.Name(), "brand.json"))
		if brand == "" {
			continue
		}

		streamsDir := filepath.Join(brandsDir, bd.Name(), "streams")
		streamDirs, _ := os.ReadDir(streamsDir)
		for _, sd := range streamDirs {
			if !sd.IsDir() {
				continue
			}
			stream := readJSONName(filepath.Join(streamsDir, sd.Name(), "stream.json"))
			if stream == "" {
				continue
			}

			projectsDir := filepath.Join(streamsDir, sd.Name(), "projects")
			projDirs, _ := os.ReadDir(projectsDir)
			for _, pd := range projDirs {
				if !pd.IsDir() {
					continue
				}
				project := readJSONName(filepath.Join(projectsDir, pd.Name(), "project.json"))
				if project == "" {
					continue
				}
				ctx := brand + " › " + stream + " › " + project

				// Read categories to map their IDs
				catsDir := filepath.Join(projectsDir, pd.Name(), "categories")
				catFiles, _ := os.ReadDir(catsDir)
				for _, cf := range catFiles {
					if cf.IsDir() || !strings.HasSuffix(cf.Name(), ".json") {
						continue
					}
					catID := readJSONID(filepath.Join(catsDir, cf.Name()))
					if catID != "" {
						catCtx[catID] = ctx
					}
				}
			}
		}
	}

	// Step 2: Walk pin files to map cardID → project context via categoryID
	pinsDir := filepath.Join(repoRoot, "pins")
	pinDirs, _ := os.ReadDir(pinsDir)
	for _, pd := range pinDirs {
		if !pd.IsDir() {
			continue
		}
		cardID := pd.Name()
		data, err := os.ReadFile(filepath.Join(pinsDir, cardID, "pins.json"))
		if err != nil {
			continue
		}
		var pinFile model.PinFile
		if err := json.Unmarshal(data, &pinFile); err != nil {
			continue
		}
		// Collect unique contexts from all pins
		seen := make(map[string]bool)
		var contexts []string
		for _, p := range pinFile.Pins {
			ctx := catCtx[p.CategoryID]
			if ctx != "" && !seen[ctx] {
				seen[ctx] = true
				contexts = append(contexts, ctx)
			}
		}
		if len(contexts) > 0 {
			result[cardID] = strings.Join(contexts, " ")
		}
	}

	return result
}

// readJSONName reads a JSON file and returns its "name" field.
func readJSONName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var obj struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &obj) != nil {
		return ""
	}
	return obj.Name
}

// readJSONID reads a JSON file and returns its "id" field.
func readJSONID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &obj) != nil {
		return ""
	}
	return obj.ID
}

// readCardFile parses a card file. The card is indexed under cardID, the
// ID its file name gives — the one the repo addresses it by and the one
// IncrementalRefresh tracks — even if the JSON's own id disagrees.
func readCardFile(path, cardID string) (*model.Card, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var card model.Card
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, err
	}
	card.ID = cardID
	return &card, nil
}
