package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"strings"
)

// RevalidateStats tracks what was repaired during a revalidation pass.
type RevalidateStats struct {
	StalePinsRemoved     int
	DuplicatePinsRemoved int
	// OrphanedPinDirs counts pin dirs whose card file is missing. They are
	// reported, never deleted: with Syncthing the pin can arrive before
	// the card, and deleting it would lose the pin for good.
	OrphanedPinDirs int
	// OrphanedAgentFiles counts .agent.json files whose card file is
	// missing — reported and kept for the same reason: the agent file can
	// sync in before its card.
	OrphanedAgentFiles int
	// StalePinCheckSkipped is set when some level of the hierarchy could
	// not be listed, so stale-pin removal was skipped rather than risk
	// deleting pins to categories that exist but weren't read.
	StalePinCheckSkipped bool
}

func (s RevalidateStats) String() string {
	parts := []string{}
	if s.StalePinsRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d stale pins removed", s.StalePinsRemoved))
	}
	if s.DuplicatePinsRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d duplicate pins removed", s.DuplicatePinsRemoved))
	}
	if s.OrphanedPinDirs > 0 {
		parts = append(parts, fmt.Sprintf("%d pin dirs without a card file (kept)", s.OrphanedPinDirs))
	}
	if s.OrphanedAgentFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d agent files without a card file (kept)", s.OrphanedAgentFiles))
	}
	if s.StalePinCheckSkipped {
		parts = append(parts, "stale-pin check skipped (hierarchy not fully readable)")
	}
	if len(parts) == 0 {
		return "nothing to repair"
	}
	return strings.Join(parts, ", ")
}

// Revalidate scans the repository for inconsistencies and auto-repairs them.
// Should be called on repository open, before the index is refreshed.
//
// Every repair is non-destructive under partial failure or partial sync:
// a pin is only dropped when it provably duplicates another or points at
// a category the complete hierarchy scan didn't find.
func (r *Repository) Revalidate() (*RevalidateStats, error) {
	stats := &RevalidateStats{}

	r.repairStalePins(stats)
	r.repairDuplicatePins(stats)
	r.reportOrphanedPinDirs(stats)
	r.reportOrphanedAgentFiles(stats)

	return stats, nil
}

// repairDuplicatePins collapses pins that name the same category twice
// (written by the pre-2026-09-17 MoveCardToCategory when the destination
// was already pinned). The first occurrence wins; the index's unique key
// would otherwise refuse every later pin write for the card.
func (r *Repository) repairDuplicatePins(stats *RevalidateStats) {
	cardIDs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return
	}
	for _, cardID := range cardIDs {
		_ = r.mutatePinFile(cardID, func(pinFile *model.PinFile) error {
			seen := map[string]bool{}
			filtered := make([]model.Pin, 0, len(pinFile.Pins))
			for _, p := range pinFile.Pins {
				if seen[p.CategoryID] {
					continue
				}
				seen[p.CategoryID] = true
				filtered = append(filtered, p)
			}
			if len(filtered) == len(pinFile.Pins) {
				return ErrNoChange
			}
			stats.DuplicatePinsRemoved += len(pinFile.Pins) - len(filtered)
			pinFile.Pins = filtered
			return nil
		})
	}
}

// repairStalePins removes pin entries that reference categories no longer
// on disk. Skipped entirely unless every level of the hierarchy listed
// cleanly — an unreadable stream/project would otherwise make all of its
// categories look deleted.
func (r *Repository) repairStalePins(stats *RevalidateStats) {
	validCategoryIDs, complete := r.collectAllCategoryIDs()
	if !complete {
		stats.StalePinCheckSkipped = true
		return
	}
	if len(validCategoryIDs) == 0 {
		return
	}

	cardIDs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return
	}

	for _, cardID := range cardIDs {
		_ = r.mutatePinFile(cardID, func(pinFile *model.PinFile) error {
			filtered := make([]model.Pin, 0, len(pinFile.Pins))
			for _, p := range pinFile.Pins {
				if validCategoryIDs[p.CategoryID] {
					filtered = append(filtered, p)
				}
			}
			if len(filtered) == len(pinFile.Pins) {
				return ErrNoChange
			}
			stats.StalePinsRemoved += len(pinFile.Pins) - len(filtered)
			pinFile.Pins = filtered // empty → mutatePinFile removes the dir
			return nil
		})
	}
}

// reportOrphanedPinDirs counts pin directories whose card file is missing.
// They are deliberately kept: under Syncthing the pin file can land
// before the card file, and a card deleted through BRUV already has its
// pins removed by DeleteCard.
func (r *Repository) reportOrphanedPinDirs(stats *RevalidateStats) {
	cardIDs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return
	}

	for _, cardID := range cardIDs {
		if !fileExists(r.cardFilePath(cardID)) {
			stats.OrphanedPinDirs++
		}
	}
}

// reportOrphanedAgentFiles counts .agent.json files whose card file is
// missing. Like orphaned pin dirs they are reported, never deleted: with
// Syncthing the agent file can arrive before its card, and deleting it
// would lose the agent for good. (The scheduler skips an agent whose card
// is missing, so a kept orphan never runs.)
func (r *Repository) reportOrphanedAgentFiles(stats *RevalidateStats) {
	entries, err := os.ReadDir(r.cardsPath())
	if err != nil {
		return
	}
	for _, e := range entries {
		cardID, ok := strings.CutSuffix(e.Name(), ".agent.json")
		if ok && !fileExists(r.cardFilePath(cardID)) {
			stats.OrphanedAgentFiles++
		}
	}
}

// collectAllCategoryIDs walks the full brand/stream/project/category
// directory tree and returns a set of all category IDs that exist on disk.
// It walks directories rather than the List* APIs because those skip
// unreadable entries silently. complete is false when any directory
// failed to list or any category file failed to read/parse, in which case
// the set is partial and must not be used to judge a pin stale.
func (r *Repository) collectAllCategoryIDs() (ids map[string]bool, complete bool) {
	ids = make(map[string]bool)

	brands, err := listSubdirs(r.brandsPath())
	if err != nil {
		return ids, false
	}
	for _, b := range brands {
		streams, err := listSubdirs(r.streamsPath(b))
		if err != nil {
			return ids, false
		}
		for _, s := range streams {
			projects, err := listSubdirs(r.projectsPath(b, s))
			if err != nil {
				return ids, false
			}
			for _, p := range projects {
				slugs, err := listJSONFiles(r.categoriesPath(b, s, p))
				if err != nil {
					return ids, false
				}
				for _, slug := range slugs {
					cat, err := r.GetCategory(b, s, p, slug)
					if err != nil {
						return ids, false
					}
					ids[cat.ID] = true
				}
			}
		}
	}
	return ids, true
}
