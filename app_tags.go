package main

// Tag colours and per-project labels.
//
// "Tags" (repo-wide) and "Labels" (per-project) share a storage backend
// but carry different semantics:
//   - Tags are free-text, repo-wide, and colour-mapped via tags.json.
//   - Labels are per-project, identified by ID, and map 1:1 to a tag
//     name inside their project so the UI can show the same colour
//     consistently. AddProjectLabel populates tags.json as a side-effect.
//
// Extracted from app.go so the tag-healing pass and the label CRUD all
// live in one place instead of being scattered across 500 lines of the
// god-file.

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"strings"
	"time"
)

// healTagColors walks every card in the repo and makes sure each tag
// appears (with its assigned colour) as a label in every project that
// card is pinned to. Used as a best-effort background repair on repo
// open — no errors bubble up, by design. Any failure to load/add is
// logged via the repo layer.
func (a *App) healTagColors() {
	if a.repo == nil {
		return
	}

	cards, err := a.repo.ListCards()
	if err != nil || len(cards) == 0 {
		return
	}

	// Pre-build a categoryID → (brandSlug, streamSlug, projectSlug) lookup.
	// Uses the shared flat walker so this path pays only one hierarchy
	// traversal instead of recreating the nested walk that
	// ListAllCategories already performs.
	type hierKey struct{ brand, stream, project string }
	catToHier := make(map[string]hierKey)
	flat, _ := a.repo.ListAllCategoriesFlat()
	for _, f := range flat {
		catToHier[f.Category.ID] = hierKey{f.Brand.Slug, f.Stream.Slug, f.Project.Slug}
	}

	// For each card, ensure every tag is present (with colour) in every pinned project.
	for _, card := range cards {
		if len(card.Tags) == 0 {
			continue
		}
		pins, err := a.repo.GetCardPins(card.ID)
		if err != nil {
			continue
		}
		// Collect unique projects this card is pinned to.
		seen := make(map[string]bool)
		for _, pin := range pins {
			h, ok := catToHier[pin.CategoryID]
			if !ok {
				continue
			}
			key := h.brand + "/" + h.stream + "/" + h.project
			if seen[key] {
				continue
			}
			seen[key] = true

			labels, _ := a.repo.GetProjectLabels(h.brand, h.stream, h.project)
			existing := make(map[string]bool, len(labels))
			for _, l := range labels {
				existing[strings.ToLower(l.Name)] = true
			}
			for _, tag := range card.Tags {
				if !existing[strings.ToLower(tag)] {
					// AddProjectLabel syncs colour to tags.json.
					a.repo.AddProjectLabel(h.brand, h.stream, h.project, tag, "")
				}
			}
		}
	}
}

// --- Tag colours (repo-wide) ---

func (a *App) GetTagColors() (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetTagColors()
}

func (a *App) SetTagColor(tag, color string) (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.SetTagColor(tag, color)
}

func (a *App) AssignTagColor(tag string) (map[string]string, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AssignTagColor(tag)
}

// --- Labels (per-project) ---

func (a *App) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
}

func (a *App) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	name = repo.SanitizeText(name)
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color)
}

func (a *App) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID)
}

func (a *App) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	if name != "" {
		name = repo.SanitizeText(name)
	}
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color)
}

func (a *App) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return a.repo.SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon)
}

// UpdateCardLabels replaces a card's label IDs.
func (a *App) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	card, err := a.repo.UpdateCard(id, func(c *model.Card) {
		c.Labels = labelIDs
	})
	if err == nil && a.idx != nil {
		a.logIdxErr("IndexCard", a.idx.IndexCard(card, time.Now(), a.idx.GetCardProjectContext(card.ID)))
	}
	return card, err
}
