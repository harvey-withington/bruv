package repo

import (
	"path/filepath"
)

// Trello-inspired label color palette (12 colors).
var TagPalette = []string{
	"#61bd4f", // green
	"#f2d600", // yellow
	"#ff9f1a", // orange
	"#eb5a46", // red
	"#c377e0", // purple
	"#0079bf", // blue
	"#00c2e0", // sky
	"#51e898", // lime
	"#ff78cb", // pink
	"#344563", // dark grey-blue
	"#b3bac5", // light grey
	"#096dd9", // dark blue
}

const tagsFile = "tags.json"

// TagColors maps tag name → hex color string.
type TagColors map[string]string

func (r *Repository) tagsPath() string {
	return filepath.Join(r.Root, bruvDir, tagsFile)
}

// GetTagColors loads the tag→color map from disk.
func (r *Repository) GetTagColors() (TagColors, error) {
	tc := make(TagColors)
	err := readJSON(r.tagsPath(), &tc)
	if err != nil {
		// File may not exist yet — return empty map
		return make(TagColors), nil
	}
	return tc, nil
}

// SetTagColor sets the color for a given tag and persists to disk.
func (r *Repository) SetTagColor(tag, color string) (TagColors, error) {
	tc, _ := r.GetTagColors()
	tc[tag] = color
	if err := writeJSON(r.tagsPath(), tc); err != nil {
		return nil, err
	}
	return tc, nil
}

// AssignTagColor picks the next unused palette color for a tag and saves it.
// If the tag already has a color, returns the existing mapping unchanged.
func (r *Repository) AssignTagColor(tag string) (TagColors, error) {
	tc, _ := r.GetTagColors()

	// Already has a color
	if _, ok := tc[tag]; ok {
		return tc, nil
	}

	// Count usage of each palette color
	usage := make(map[string]int, len(TagPalette))
	for _, c := range tc {
		usage[c]++
	}

	// Pick first unused palette color, or least-used if all taken
	chosen := TagPalette[0]
	minCount := usage[TagPalette[0]]
	found := false
	for _, c := range TagPalette {
		if usage[c] == 0 {
			chosen = c
			found = true
			break
		}
		if usage[c] < minCount {
			minCount = usage[c]
			chosen = c
		}
	}
	_ = found

	tc[tag] = chosen
	if err := writeJSON(r.tagsPath(), tc); err != nil {
		return nil, err
	}
	return tc, nil
}

// RemoveTagColor removes a tag's color assignment.
func (r *Repository) RemoveTagColor(tag string) (TagColors, error) {
	tc, _ := r.GetTagColors()
	delete(tc, tag)
	if err := writeJSON(r.tagsPath(), tc); err != nil {
		return nil, err
	}
	return tc, nil
}
