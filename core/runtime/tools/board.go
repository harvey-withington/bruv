package tools

// Board listing shared by the MCP server's list_cards and the agent's
// built-in list_cards: one project board, grouped by category in board
// order, as compact per-card summaries.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	projectsvc "bruv/core/services/project"
	"bruv/internal/repo"
)

// CardSummary is enough to pick a card without paying for its blocks.
type CardSummary struct {
	CardID   string     `json:"card_id"`
	Title    string     `json:"title"`
	Type     string     `json:"type"`
	Position int        `json:"position"`
	DueDate  *time.Time `json:"due_date,omitempty"`
	Tags     []string   `json:"tags,omitempty"`
}

// CategoryCards is one board column.
type CategoryCards struct {
	Category   string        `json:"category"`
	CategoryID string        `json:"category_id"`
	Cards      []CardSummary `json:"cards"`
}

// ListBoard lists a project's cards by category. loc.Category is
// optional and narrows the listing to one column. Nothing is created: a
// path that doesn't exist is an error.
func ListBoard(r *repo.Repository, ps *projectsvc.Service, loc Location) ([]CategoryCards, error) {
	if loc.Brand == "" || loc.Stream == "" || loc.Project == "" {
		return nil, fmt.Errorf("brand, stream and project are required (category is optional)")
	}
	brandSlug, _, ok := FindBrand(ps, loc.Brand)
	if !ok {
		return nil, fmt.Errorf("brand %q not found", loc.Brand)
	}
	streamSlug, _, ok := FindStream(ps, brandSlug, loc.Stream)
	if !ok {
		return nil, fmt.Errorf("stream %q not found", loc.Stream)
	}
	projectSlug, _, ok := FindProject(ps, brandSlug, streamSlug, loc.Project)
	if !ok {
		return nil, fmt.Errorf("project %q not found", loc.Project)
	}
	cats, err := ps.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	out := []CategoryCards{}
	for _, cat := range cats {
		if loc.Category != "" && !matches(cat.Name, cat.Slug, loc.Category) {
			continue
		}
		pins, err := r.ListCardsInCategory(cat.ID)
		if err != nil {
			return nil, fmt.Errorf("list category %q: %w", cat.Name, err)
		}
		// The store orders by Position only, and Position is per-card (the
		// index within that card's own pin file), so cards pinned fresh
		// into the same category all tie at 0 and fall back to directory
		// order. Break ties by pin time, then id, so repeated calls agree.
		sort.SliceStable(pins, func(i, j int) bool {
			if pins[i].Position != pins[j].Position {
				return pins[i].Position < pins[j].Position
			}
			if !pins[i].PinnedAt.Equal(pins[j].PinnedAt) {
				return pins[i].PinnedAt.Before(pins[j].PinnedAt)
			}
			return pins[i].CardID < pins[j].CardID
		})
		entry := CategoryCards{Category: cat.Name, CategoryID: cat.ID, Cards: []CardSummary{}}
		for _, p := range pins {
			c, err := r.GetCard(p.CardID)
			if err != nil {
				continue // a dangling pin shouldn't sink the whole listing
			}
			entry.Cards = append(entry.Cards, CardSummary{
				CardID: c.ID, Title: c.Title, Type: c.Type, Position: p.Position, DueDate: c.DueDate, Tags: c.Tags,
			})
		}
		out = append(out, entry)
	}
	if loc.Category != "" && len(out) == 0 {
		return nil, fmt.Errorf("category %q not found in that project", loc.Category)
	}
	return out, nil
}

// LocationArgs reads brand/stream/project/category tool arguments.
func LocationArgs(a map[string]any) Location {
	str := func(k string) string {
		s, _ := a[k].(string)
		return strings.TrimSpace(s)
	}
	return Location{Brand: str("brand"), Stream: str("stream"), Project: str("project"), Category: str("category")}
}
