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

	// Index all cards
	cardsDir := filepath.Join(repoRoot, "cards")
	if err := idx.indexCardsFromDir(cardsDir, stats); err != nil {
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

		if !indexedMtime.IsZero() && indexedMtime.Equal(fileMtime) {
			stats.CardsSkipped++
			continue
		}

		// Card is new or modified — re-index
		card, err := readCardFile(filePath)
		if err != nil {
			continue
		}

		if err := idx.IndexCard(card, fileMtime); err != nil {
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

func (idx *Index) indexCardsFromDir(cardsDir string, stats *RebuildStats) error {
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

		if err := idx.IndexCard(card, info.ModTime().UTC()); err != nil {
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
