package importer

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"encoding/json"
	"fmt"
	"time"
)

// ExportFormatVersion tracks the schema of BRUV project exports. Bump when
// breaking fields. A future Import can refuse versions it doesn't understand.
const ExportFormatVersion = 1

// ProjectExport is the on-disk format for a single BRUV project snapshot.
// Intentionally self-contained so a repo can be backed up and restored without
// external references.
type ProjectExport struct {
	FormatVersion int                   `json:"format_version"`
	ExportedAt    time.Time             `json:"exported_at"`
	Source        string                `json:"source"` // "bruv"
	Brand         string                `json:"brand_slug"`
	Stream        string                `json:"stream_slug"`
	Project       model.Project         `json:"project"`
	Categories    []model.Category      `json:"categories"`
	Labels        []model.Label         `json:"labels"`
	Cards         []ExportedCard        `json:"cards"`
	TagColors     map[string]string     `json:"tag_colors,omitempty"`
}

// ExportedCard bundles a card with its pin position and comment thread so a
// re-importer can restore category membership and history.
type ExportedCard struct {
	Card       model.Card       `json:"card"`
	CategoryID string           `json:"category_id"`
	Position   int              `json:"position"`
	Comments   []model.Comment  `json:"comments,omitempty"`
}

// ExportProject reads a project out of the repository into a self-contained
// ProjectExport value. The returned bytes are pretty-printed JSON.
func ExportProject(r *repo.Repository, brandSlug, streamSlug, projectSlug string) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("nil repository")
	}
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	categories, err := r.ListCategories(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)

	cards := make([]ExportedCard, 0)
	seen := make(map[string]bool) // card dedup: a card pinned to multiple cats still exports once
	for _, cat := range categories {
		pins, _ := r.ListCardsInCategory(cat.ID)
		for _, p := range pins {
			if seen[p.CardID] {
				continue
			}
			seen[p.CardID] = true
			card, err := r.GetCard(p.CardID)
			if err != nil {
				continue
			}
			var comments []model.Comment
			if cf, err := r.LoadComments(card.ID); err == nil && len(cf.Comments) > 0 {
				comments = cf.Comments
			}
			cards = append(cards, ExportedCard{
				Card:       *card,
				CategoryID: p.CategoryID,
				Position:   p.Position,
				Comments:   comments,
			})
		}
	}

	// Only include tag colours for tags actually used by the exported cards,
	// to keep the payload tight and avoid leaking unrelated tags.
	allTagColors, _ := r.GetTagColors()
	tagColors := make(map[string]string)
	for _, ec := range cards {
		for _, tag := range ec.Card.Tags {
			if c, ok := allTagColors[tag]; ok {
				tagColors[tag] = c
			}
		}
	}

	exp := ProjectExport{
		FormatVersion: ExportFormatVersion,
		ExportedAt:    time.Now().UTC(),
		Source:        "bruv",
		Brand:         brandSlug,
		Stream:        streamSlug,
		Project:       *project,
		Categories:    categories,
		Labels:        labels,
		Cards:         cards,
		TagColors:     tagColors,
	}

	return json.MarshalIndent(exp, "", "  ")
}
