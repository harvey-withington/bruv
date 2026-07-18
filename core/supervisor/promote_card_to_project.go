package supervisor

import (
	"fmt"
	"log/slog"
	"strings"

	"bruv/internal/model"
)

// PromotedProject is the result of PromoteCardToProject: the new project and
// its default category, where the originating card is pinned.
type PromotedProject struct {
	Project  *model.Project  `json:"project"`
	Category *model.Category `json:"category"`
}

// PromoteCardToProject creates a new project in the given stream — with its
// default category — and pins the originating card into it. The card is
// referenced, not moved: existing pins stay. Pinning syncs the card's tags
// into the new project's palette (standard Pin behaviour). When
// copyDescription is set, the card's description becomes the project's.
func (r *Runtime) PromoteCardToProject(cardID, brandSlug, streamSlug, name string, copyDescription bool) (*PromotedProject, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	card, err := r.Card.Get(cardID)
	if err != nil {
		return nil, err
	}

	project, category, err := r.Project.CreateProjectWithDefaultCategory(brandSlug, streamSlug, name)
	if err != nil {
		return nil, err
	}
	if category == nil {
		// Without the default category there is nowhere to pin — roll back
		// the empty project rather than leave a half-promoted state.
		_ = r.Project.DeleteProject(brandSlug, streamSlug, project.Slug)
		return nil, fmt.Errorf("create default category failed")
	}

	if err := r.Card.Pin(cardID, category.ID); err != nil {
		_ = r.Project.DeleteProject(brandSlug, streamSlug, project.Slug)
		return nil, fmt.Errorf("pin card: %w", err)
	}

	if copyDescription && strings.TrimSpace(card.Description) != "" {
		if updated, err := r.Project.UpdateProjectDescription(brandSlug, streamSlug, project.Slug, card.Description); err == nil {
			project = updated
		} else {
			// Non-fatal — the promotion succeeded, only the description copy
			// didn't stick.
			slog.Warn("promote card to project: copy description failed", "project", project.Slug, "err", err)
		}
	}

	return &PromotedProject{Project: project, Category: category}, nil
}
