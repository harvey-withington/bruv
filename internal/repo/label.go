package repo

import (
	"bruv/internal/model"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// On-disk filename for per-project tag definitions. The Go-side type
// (model.Label) keeps its historical name so the rest of the codebase
// doesn't churn — only the user-facing artefact (the file) is "tags".
const projectTagsFile = "tags.json"

// projectLabelsFile is the on-disk format for per-project tag definitions.
// Field name "labels" is preserved on disk for backwards compatibility
// with existing repos that already have data written under that key.
type projectLabelsFile struct {
	Labels []model.Label `json:"labels"`
}

func (r *Repository) labelsPath(brandSlug, streamSlug, projectSlug string) string {
	return filepath.Join(r.projectPath(brandSlug, streamSlug, projectSlug), projectTagsFile)
}

// GetProjectLabels loads labels for a project. Returns an empty slice if
// the file doesn't exist; any other read/parse failure is an error, so a
// caller never mistakes an unreadable file for "no tags".
func (r *Repository) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	path := r.labelsPath(brandSlug, streamSlug, projectSlug)
	if !fileExists(path) {
		return []model.Label{}, nil
	}
	var f projectLabelsFile
	if err := readJSON(path, &f); err != nil {
		return nil, err
	}
	if f.Labels == nil {
		f.Labels = []model.Label{}
	}
	return f.Labels, nil
}

// mutateProjectLabels runs fn on a fresh read of the project's labels
// under the file lock and saves what it returns. A failed read aborts
// before fn runs — writing back only fn's result would erase every
// existing tag.
func (r *Repository) mutateProjectLabels(brandSlug, streamSlug, projectSlug string, fn func([]model.Label) ([]model.Label, error)) ([]model.Label, error) {
	path := r.labelsPath(brandSlug, streamSlug, projectSlug)
	unlock := lockPath(path)
	defer unlock()

	labels, err := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	labels, err = fn(labels)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(path, projectLabelsFile{Labels: labels}); err != nil {
		return nil, err
	}
	return labels, nil
}

// AddProjectLabel appends a new label to the project and returns the updated list.
// If no color is provided, the global tags.json color for that name is used for
// consistency; otherwise a palette color is auto-assigned and written back to tags.json.
func (r *Repository) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	return r.mutateProjectLabels(brandSlug, streamSlug, projectSlug, func(labels []model.Label) ([]model.Label, error) {
		if color == "" {
			// Prefer an existing global color so the same tag looks the
			// same everywhere; otherwise pick one and persist it back to
			// the global tag colors for future consistency.
			if _, err := r.mutateTagColors(func(tc TagColors) (bool, error) {
				if c, ok := tc[name]; ok && c != "" {
					color = c
					return false, nil
				}
				color = assignLabelColor(labels)
				tc[name] = color
				return true, nil
			}); err != nil {
				return nil, err
			}
		}

		return append(labels, model.Label{
			ID:    uuid.New().String(),
			Name:  name,
			Color: color,
		}), nil
	})
}

// RemoveProjectLabel removes a label by ID and returns the updated list.
func (r *Repository) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	return r.mutateProjectLabels(brandSlug, streamSlug, projectSlug, func(labels []model.Label) ([]model.Label, error) {
		filtered := make([]model.Label, 0, len(labels))
		for _, l := range labels {
			if l.ID != labelID {
				filtered = append(filtered, l)
			}
		}
		if len(filtered) == len(labels) {
			return nil, fmt.Errorf("label %q not found", labelID)
		}
		return filtered, nil
	})
}

// SetProjectLabelIcon sets or clears the icon on a project label by ID.
func (r *Repository) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	return r.mutateProjectLabels(brandSlug, streamSlug, projectSlug, func(labels []model.Label) ([]model.Label, error) {
		for i, l := range labels {
			if l.ID == labelID {
				labels[i].Icon = icon
				return labels, nil
			}
		}
		return nil, fmt.Errorf("label %q not found", labelID)
	})
}

// UpdateProjectLabel updates a label's name and/or color by ID and returns the updated list.
func (r *Repository) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	return r.mutateProjectLabels(brandSlug, streamSlug, projectSlug, func(labels []model.Label) ([]model.Label, error) {
		for i, l := range labels {
			if l.ID == labelID {
				if name != "" {
					labels[i].Name = name
				}
				if color != "" {
					labels[i].Color = color
				}
				return labels, nil
			}
		}
		return nil, fmt.Errorf("label %q not found", labelID)
	})
}

// assignLabelColor picks the next unused palette color for a label.
func assignLabelColor(labels []model.Label) string {
	usage := make(map[string]int, len(TagPalette))
	for _, l := range labels {
		usage[l.Color]++
	}
	for _, c := range TagPalette {
		if usage[c] == 0 {
			return c
		}
	}
	// All used — return least-used
	chosen := TagPalette[0]
	minCount := usage[TagPalette[0]]
	for _, c := range TagPalette {
		if usage[c] < minCount {
			minCount = usage[c]
			chosen = c
		}
	}
	return chosen
}
