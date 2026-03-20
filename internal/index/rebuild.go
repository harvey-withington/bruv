package index

import (
	"bruv/internal/model"
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
func (idx *Index) FullRebuild(repoRoot string) (*RebuildStats, error) {
	start := time.Now()
	stats := &RebuildStats{}

	// Clear all tables
	for _, table := range []string{"cards", "tags", "pins", "cards_fts"} {
		if _, err := idx.db.Exec("DELETE FROM " + table); err != nil {
			return nil, fmt.Errorf("clear table %s: %w", table, err)
		}
	}

	// Build cardID → project context mapping from the filesystem
	cardContextMap := buildCardContextMap(repoRoot)

	// Index all cards (with project context)
	cardsDir := filepath.Join(repoRoot, "cards")
	if err := idx.indexCardsFromDir(cardsDir, cardContextMap, stats); err != nil {
		return nil, fmt.Errorf("index cards: %w", err)
	}

	// Index all pins
	pinsDir := filepath.Join(repoRoot, "pins")
	if err := idx.indexPinsFromDir(pinsDir, stats); err != nil {
		return nil, fmt.Errorf("index pins: %w", err)
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

	cardsDir := filepath.Join(repoRoot, "cards")

	// Build a set of card IDs currently on disk
	diskCardIDs := make(map[string]bool)
	entries, err := os.ReadDir(cardsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cards dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}

		cardID := strings.TrimSuffix(entry.Name(), ".json")
		diskCardIDs[cardID] = true

		filePath := filepath.Join(cardsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileMtime := info.ModTime().UTC()

		// Check if the index already has this card with the same mtime
		indexedMtime, err := idx.GetCardMtime(cardID)
		if err != nil {
			continue
		}

		// Re-index if mtime changed OR if project context needs updating
		ctx := cardContextMap[cardID]
		storedCtx := idx.GetCardProjectContext(cardID)
		if !indexedMtime.IsZero() && indexedMtime.Equal(fileMtime) && ctx == storedCtx {
			stats.CardsSkipped++
			continue
		}

		// Card is new or modified or context changed — re-index
		card, err := readCardFile(filePath)
		if err != nil {
			continue
		}

		if err := idx.IndexCard(card, fileMtime, ctx); err != nil {
			continue
		}
		stats.CardsIndexed++
	}

	// Remove index entries for cards no longer on disk
	indexedIDs, err := idx.ListIndexedCardIDs()
	if err != nil {
		return nil, fmt.Errorf("list indexed cards: %w", err)
	}

	for _, id := range indexedIDs {
		if !diskCardIDs[id] {
			if err := idx.RemoveCard(id); err != nil {
				continue
			}
			stats.CardsRemoved++
		}
	}

	// Re-index all pins (fast operation, not worth incremental tracking)
	pinsDir := filepath.Join(repoRoot, "pins")
	if err := idx.rebuildPins(pinsDir, stats); err != nil {
		return nil, fmt.Errorf("rebuild pins: %w", err)
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// --- Internal helpers ---

func (idx *Index) indexCardsFromDir(cardsDir string, cardContextMap map[string]string, stats *RebuildStats) error {
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}

		filePath := filepath.Join(cardsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		card, err := readCardFile(filePath)
		if err != nil {
			continue
		}

		cardID := strings.TrimSuffix(entry.Name(), ".json")
		ctx := cardContextMap[cardID]
		if err := idx.IndexCard(card, info.ModTime().UTC(), ctx); err != nil {
			continue
		}
		stats.CardsIndexed++
	}

	return nil
}

func (idx *Index) indexPinsFromDir(pinsDir string, stats *RebuildStats) error {
	return idx.rebuildPins(pinsDir, stats)
}

func (idx *Index) rebuildPins(pinsDir string, stats *RebuildStats) error {
	// Clear existing pins
	if _, err := idx.db.Exec("DELETE FROM pins"); err != nil {
		return err
	}

	entries, err := os.ReadDir(pinsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		cardID := entry.Name()
		pinFilePath := filepath.Join(pinsDir, cardID, "pins.json")

		data, err := os.ReadFile(pinFilePath)
		if err != nil {
			continue
		}

		var pinFile model.PinFile
		if err := json.Unmarshal(data, &pinFile); err != nil {
			continue
		}

		if err := idx.IndexPins(cardID, pinFile.Pins); err != nil {
			continue
		}
		stats.PinsIndexed += len(pinFile.Pins)
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
				ctx := brand + " \u203a " + stream + " \u203a " + project

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

func readCardFile(path string) (*model.Card, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var card model.Card
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, err
	}
	return &card, nil
}
