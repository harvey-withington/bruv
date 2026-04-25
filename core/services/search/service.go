// Package search is the SearchService — full-text search over indexed cards
// and index-backed ID lookups (pins by category/tag, orphans).
//
// First service extracted from the app_*.go monolith as the pilot for the
// remote-backend refactor. Pattern established here is the template for
// every other service: a narrow Deps interface declared in this package,
// implemented by the host via an adapter, and a Service struct that holds
// the Deps and exposes the domain methods. See plan/remote-backend-architecture-2026-04-24.md.
package search

import (
	"bruv/internal/index"
	"bruv/internal/repo"
	"fmt"
)

// Deps is the narrow host contract SearchService needs. Each service
// declares its own Deps so callers implement only what's actually used.
// Methods return pointers rather than snapshotting because the underlying
// repo/index are re-pointed when repos open and close.
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
}

// Service performs full-text search and index-backed ID lookups.
type Service struct {
	deps Deps
}

// New constructs a SearchService. The Deps are consulted on every call,
// so the service survives repo open/close without being reconstructed.
func New(deps Deps) *Service {
	return &Service{deps: deps}
}

// GetCardProjectContext returns the stored project-hierarchy breadcrumb
// for a card (e.g. "Brand > Stream > Project"), or "" if unknown.
func (s *Service) GetCardProjectContext(cardID string) string {
	idx := s.deps.Index()
	if idx == nil {
		return ""
	}
	return idx.GetCardProjectContext(cardID)
}

// SearchCards performs a full-text search across all indexed cards.
func (s *Service) SearchCards(query string, limit int) ([]index.SearchResult, error) {
	idx := s.deps.Index()
	if idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return idx.Search(query, limit)
}

// SearchOrphanedCards performs a full-text search limited to orphaned
// (inbox) cards.
func (s *Service) SearchOrphanedCards(query string, limit int) ([]index.SearchResult, error) {
	idx := s.deps.Index()
	if idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return idx.SearchOrphanedCards(query, limit)
}

// RebuildIndex drops and rebuilds the entire SQLite index from disk.
func (s *Service) RebuildIndex() (*index.RebuildStats, error) {
	idx, r := s.deps.Index(), s.deps.Repo()
	if idx == nil || r == nil {
		return nil, fmt.Errorf("no repository or index open")
	}
	return idx.FullRebuild(r.Root)
}

// RefreshIndex incrementally updates the index for changed/new/deleted cards.
func (s *Service) RefreshIndex() (*index.RebuildStats, error) {
	idx, r := s.deps.Index(), s.deps.Repo()
	if idx == nil || r == nil {
		return nil, fmt.Errorf("no repository or index open")
	}
	return idx.IncrementalRefresh(r.Root)
}

// ListCardIDsInCategory returns card IDs pinned to a project/category.
func (s *Service) ListCardIDsInCategory(projectID, categoryID string) ([]string, error) {
	idx := s.deps.Index()
	if idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return idx.ListCardIDsInCategory(projectID, categoryID)
}

// ListOrphanedCardIDs returns IDs of cards that have no pins (Inbox cards).
func (s *Service) ListOrphanedCardIDs() ([]string, error) {
	idx := s.deps.Index()
	if idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return idx.ListOrphanedCardIDs()
}

// ListCardIDsByTag returns card IDs with a given tag.
func (s *Service) ListCardIDsByTag(tag string) ([]string, error) {
	idx := s.deps.Index()
	if idx == nil {
		return nil, fmt.Errorf("no index available")
	}
	return idx.ListCardIDsByTag(tag)
}
