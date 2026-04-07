package repo

import (
	"fmt"
	"os"
	"strings"
)

// RevalidateStats tracks what was repaired during a revalidation pass.
type RevalidateStats struct {
	StalePinsRemoved   int
	OrphanedPinDirs    int
	OrphanedChatFiles  int
}

func (s RevalidateStats) String() string {
	parts := []string{}
	if s.StalePinsRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d stale pins removed", s.StalePinsRemoved))
	}
	if s.OrphanedPinDirs > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned pin dirs removed", s.OrphanedPinDirs))
	}
	if s.OrphanedChatFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned chat files removed", s.OrphanedChatFiles))
	}
	if len(parts) == 0 {
		return "nothing to repair"
	}
	return strings.Join(parts, ", ")
}

// Revalidate scans the repository for inconsistencies and auto-repairs them.
// Should be called on repository open, before the index is refreshed.
func (r *Repository) Revalidate() (*RevalidateStats, error) {
	stats := &RevalidateStats{}

	r.repairStalePins(stats)
	r.repairOrphanedPinDirs(stats)
	r.repairOrphanedChatFiles(stats)

	return stats, nil
}

// repairStalePins removes pin entries that reference categories no longer on disk.
func (r *Repository) repairStalePins(stats *RevalidateStats) {
	// Build set of all valid category IDs
	validCategoryIDs := r.collectAllCategoryIDs()
	if len(validCategoryIDs) == 0 {
		return
	}

	cardIDs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return
	}

	for _, cardID := range cardIDs {
		pinFile, err := r.loadPinFile(cardID)
		if err != nil || len(pinFile.Pins) == 0 {
			continue
		}

		filtered := pinFile.Pins[:0]
		for _, p := range pinFile.Pins {
			if validCategoryIDs[p.CategoryID] {
				filtered = append(filtered, p)
			} else {
				stats.StalePinsRemoved++
			}
		}

		if len(filtered) == len(pinFile.Pins) {
			continue // nothing changed
		}

		pinFile.Pins = filtered
		if len(filtered) == 0 {
			// No pins left — remove the pin file and dir
			pinsDir := r.pinsDirPath(cardID)
			_ = os.RemoveAll(pinsDir)
		} else {
			_ = r.savePinFile(pinFile)
		}
	}
}

// repairOrphanedPinDirs removes pin directories for cards that no longer exist.
func (r *Repository) repairOrphanedPinDirs(stats *RevalidateStats) {
	cardIDs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return
	}

	for _, cardID := range cardIDs {
		if !fileExists(r.cardFilePath(cardID)) {
			_ = os.RemoveAll(r.pinsDirPath(cardID))
			stats.OrphanedPinDirs++
		}
	}
}

// repairOrphanedChatFiles removes .messages.json files for cards that no longer exist.
func (r *Repository) repairOrphanedChatFiles(stats *RevalidateStats) {
	entries, err := os.ReadDir(r.cardsPath())
	if err != nil {
		return
	}

	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".messages.json") {
			cardID := strings.TrimSuffix(name, ".messages.json")
			if !fileExists(r.cardFilePath(cardID)) {
				_ = os.Remove(r.chatFilePath(cardID))
				stats.OrphanedChatFiles++
			}
		} else if strings.HasSuffix(name, ".agent.json") {
			cardID := strings.TrimSuffix(name, ".agent.json")
			if !fileExists(r.cardFilePath(cardID)) {
				_ = os.Remove(r.agentFilePath(cardID))
			}
		}
	}
}

// collectAllCategoryIDs walks the full brand/stream/project/category hierarchy
// and returns a set of all category IDs that exist on disk.
func (r *Repository) collectAllCategoryIDs() map[string]bool {
	ids := make(map[string]bool)

	brands, err := r.ListBrands()
	if err != nil {
		return ids
	}
	for _, b := range brands {
		streams, _ := r.ListStreams(b.Slug)
		for _, s := range streams {
			projects, _ := r.ListProjects(b.Slug, s.Slug)
			for _, p := range projects {
				cats, _ := r.ListCategories(b.Slug, s.Slug, p.Slug)
				for _, c := range cats {
					ids[c.ID] = true
				}
			}
		}
	}
	return ids
}
