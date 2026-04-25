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

// GetProjectLabels loads labels for a project. Returns empty slice if file doesn't exist.
func (r *Repository) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	var f projectLabelsFile
	err := readJSON(r.labelsPath(brandSlug, streamSlug, projectSlug), &f)
	if err != nil {
		return []model.Label{}, nil
	}
	return f.Labels, nil
}

func (r *Repository) saveProjectLabels(brandSlug, streamSlug, projectSlug string, labels []model.Label) error {
	return writeJSON(r.labelsPath(brandSlug, streamSlug, projectSlug), projectLabelsFile{Labels: labels})
}

// AddProjectLabel appends a new label to the project and returns the updated list.
// If no color is provided, the global tags.json color for that name is used for
// consistency; otherwise a palette color is auto-assigned and written back to tags.json.
func (r *Repository) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)

	if color == "" {
		// Prefer an existing global color so the same tag looks the same everywhere.
		tc, _ := r.GetTagColors()
		if c, ok := tc[name]; ok && c != "" {
			color = c
		} else {
			color = assignLabelColor(labels)
			// Persist back to global tag colors for future consistency.
			tc[name] = color
			_ = writeJSON(r.tagsPath(), tc)
		}
	}

	labels = append(labels, model.Label{
		ID:    uuid.New().String(),
		Name:  name,
		Color: color,
	})

	if err := r.saveProjectLabels(brandSlug, streamSlug, projectSlug, labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// RemoveProjectLabel removes a label by ID and returns the updated list.
func (r *Repository) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	found := false
	filtered := make([]model.Label, 0, len(labels))
	for _, l := range labels {
		if l.ID == labelID {
			found = true
			continue
		}
		filtered = append(filtered, l)
	}
	if !found {
		return nil, fmt.Errorf("label %q not found", labelID)
	}
	if err := r.saveProjectLabels(brandSlug, streamSlug, projectSlug, filtered); err != nil {
		return nil, err
	}
	return filtered, nil
}

// SetProjectLabelIcon sets or clears the icon on a project label by ID.
func (r *Repository) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	found := false
	for i, l := range labels {
		if l.ID == labelID {
			labels[i].Icon = icon
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("label %q not found", labelID)
	}
	if err := r.saveProjectLabels(brandSlug, streamSlug, projectSlug, labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// UpdateProjectLabel updates a label's name and/or color by ID and returns the updated list.
func (r *Repository) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	labels, _ := r.GetProjectLabels(brandSlug, streamSlug, projectSlug)
	found := false
	for i, l := range labels {
		if l.ID == labelID {
			if name != "" {
				labels[i].Name = name
			}
			if color != "" {
				labels[i].Color = color
			}
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("label %q not found", labelID)
	}
	if err := r.saveProjectLabels(brandSlug, streamSlug, projectSlug, labels); err != nil {
		return nil, err
	}
	return labels, nil
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
