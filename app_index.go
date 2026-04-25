package main

// Wails-bound surface for search, index lifecycle, and two cross-cutting
// queries (ListActivityLog, ListRecentlyUpdatedCards) that read directly
// from repo + enrich with category context for the Inbox and activity
// feed views.
//
// Most methods here are thin forwarders to core/services/search — see
// that package for the domain logic. openIndex, ListActivityLog, and
// ListRecentlyUpdatedCards remain on App until their neighbouring
// services are extracted (repository lifecycle + inbox/activity,
// respectively).

import (
	"bruv/internal/index"
	"bruv/internal/model"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// --- Index lifecycle (stays on App until repository-service extraction) ---

func (a *App) openIndex(repoPath string) error {
	if a.idx != nil {
		a.idx.Close()
	}
	dbPath := filepath.Join(repoPath, ".bruv", "index.db")
	idx, err := index.Open(dbPath)
	if err != nil {
		return err
	}
	a.idx = idx
	return nil
}

// --- Search / index-backed lookups (forwarders to core/services/search) ---

// GetCardProjectContext returns the stored project hierarchy path for a card (e.g. "Brand > Stream > Project").
func (a *App) GetCardProjectContext(cardID string) string {
	return a.search.GetCardProjectContext(cardID)
}

// SearchCards performs a full-text search across all indexed cards.
func (a *App) SearchCards(query string, limit int) ([]index.SearchResult, error) {
	return a.search.SearchCards(query, limit)
}

// SearchOrphanedCards performs a full-text search limited to orphaned (inbox) cards.
func (a *App) SearchOrphanedCards(query string, limit int) ([]index.SearchResult, error) {
	return a.search.SearchOrphanedCards(query, limit)
}

// RebuildIndex drops and rebuilds the entire SQLite index from disk.
func (a *App) RebuildIndex() (*index.RebuildStats, error) {
	return a.search.RebuildIndex()
}

// RefreshIndex incrementally updates the index for changed/new/deleted cards.
func (a *App) RefreshIndex() (*index.RebuildStats, error) {
	return a.search.RefreshIndex()
}

// ListCardIDsInCategory returns card IDs pinned to a project/category via the index.
func (a *App) ListCardIDsInCategory(projectID, categoryID string) ([]string, error) {
	return a.search.ListCardIDsInCategory(projectID, categoryID)
}

// ListOrphanedCardIDs returns IDs of cards that have no pins (Inbox cards).
func (a *App) ListOrphanedCardIDs() ([]string, error) {
	return a.search.ListOrphanedCardIDs()
}

// ListCardIDsByTag returns card IDs with a given tag via the index.
func (a *App) ListCardIDsByTag(tag string) ([]string, error) {
	return a.search.ListCardIDsByTag(tag)
}

// ListActivityLog returns the most-recent limit activity entries, newest first.
func (a *App) ListActivityLog(limit int) ([]model.ActivityEntry, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 50
	}
	return a.repo.ListActivity(limit)
}

// RecentCard is a card summary enriched with its first-pin path, used by the inbox.
type RecentCard struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	UpdatedAt    time.Time `json:"updated_at"`
	Tags         []string  `json:"tags"`
	DueDate      string    `json:"due_date,omitempty"`
	BrandSlug    string    `json:"brand_slug,omitempty"`
	StreamSlug   string    `json:"stream_slug,omitempty"`
	ProjectSlug  string    `json:"project_slug,omitempty"`
	BrandName    string    `json:"brand_name,omitempty"`
	StreamName   string    `json:"stream_name,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	CategoryName string    `json:"category_name,omitempty"`
	Breadcrumb   string    `json:"breadcrumb,omitempty"`
}

// ListRecentlyUpdatedCards returns up to limit cards sorted by UpdatedAt descending.
// Orphaned cards (no pins) are excluded so every result has a navigable path.
func (a *App) ListRecentlyUpdatedCards(limit int) ([]RecentCard, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 21
	}

	all, err := a.repo.ListCards()
	if err != nil {
		return nil, err
	}

	// Pre-build a breadcrumb lookup by categoryID for fast resolution
	allCats, _ := a.ListAllCategories()
	catByID := make(map[string]CategoryPath, len(allCats))
	for _, cp := range allCats {
		catByID[cp.CategoryID] = cp
	}

	// Sort all cards newest-first by UpdatedAt
	sorted := make([]model.Card, len(all))
	copy(sorted, all)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
	})

	result := make([]RecentCard, 0, limit)
	for _, card := range sorted {
		if len(result) >= limit {
			break
		}

		// Resolve pins; skip orphaned cards
		pins, err := a.repo.GetCardPins(card.ID)
		if err != nil || len(pins) == 0 {
			continue
		}

		rc := RecentCard{
			ID:        card.ID,
			Title:     card.Title,
			Type:      card.Type,
			UpdatedAt: card.UpdatedAt,
			Tags:      card.Tags,
		}
		if card.DueDate != nil {
			rc.DueDate = card.DueDate.Format("2006-01-02")
		}

		// Enrich with first-pin path
		if cp, ok := catByID[pins[0].CategoryID]; ok {
			rc.BrandSlug = cp.BrandSlug
			rc.StreamSlug = cp.StreamSlug
			rc.ProjectSlug = cp.ProjectSlug
			rc.BrandName = cp.BrandName
			rc.StreamName = cp.StreamName
			rc.ProjectName = cp.ProjectName
			rc.CategoryName = cp.CategoryName
			rc.Breadcrumb = cp.Breadcrumb
		}

		result = append(result, rc)
	}
	return result, nil
}
