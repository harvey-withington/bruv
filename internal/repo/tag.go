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

// tagsPath returns the location of the repo-global tag color cache.
// Lives at the repo root so the cross-project color identity of a
// named tag travels when the repo is shared. Per-project tag lists
// live separately at <project>/tags.json (see label.go).
func (r *Repository) tagsPath() string {
	return filepath.Join(r.Root, tagsFile)
}

// GetTagColors loads the tag→color map from disk. A missing file is an
// empty map; any other read/parse failure is returned as an error.
func (r *Repository) GetTagColors() (TagColors, error) {
	path := r.tagsPath()
	if !fileExists(path) {
		return make(TagColors), nil
	}
	tc := make(TagColors)
	if err := readJSON(path, &tc); err != nil {
		return nil, err
	}
	if tc == nil { // file held JSON null
		tc = make(TagColors)
	}
	return tc, nil
}

// mutateTagColors runs fn on a fresh read of the tag colors under the file
// lock and saves the map when fn reports a change. A failed read aborts
// before fn runs, so an unreadable file is never overwritten with only
// the new entry.
func (r *Repository) mutateTagColors(fn func(TagColors) (changed bool, err error)) (TagColors, error) {
	path := r.tagsPath()
	unlock := lockPath(path)
	defer unlock()

	tc, err := r.GetTagColors()
	if err != nil {
		return nil, err
	}
	changed, err := fn(tc)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := writeJSON(path, tc); err != nil {
			return nil, err
		}
	}
	return tc, nil
}

// SetTagColor sets the color for a given tag and persists to disk.
func (r *Repository) SetTagColor(tag, color string) (TagColors, error) {
	return r.mutateTagColors(func(tc TagColors) (bool, error) {
		tc[tag] = color
		return true, nil
	})
}

// AssignTagColor picks the next unused palette color for a tag and saves it.
// If the tag already has a color, returns the existing mapping unchanged.
func (r *Repository) AssignTagColor(tag string) (TagColors, error) {
	return r.mutateTagColors(func(tc TagColors) (bool, error) {
		// Already has a color
		if _, ok := tc[tag]; ok {
			return false, nil
		}

		// Count usage of each palette color
		usage := make(map[string]int, len(TagPalette))
		for _, c := range tc {
			usage[c]++
		}

		// Pick first unused palette color, or least-used if all taken
		chosen := TagPalette[0]
		minCount := usage[TagPalette[0]]
		for _, c := range TagPalette {
			if usage[c] == 0 {
				chosen = c
				break
			}
			if usage[c] < minCount {
				minCount = usage[c]
				chosen = c
			}
		}

		tc[tag] = chosen
		return true, nil
	})
}

// RemoveTagColor removes a tag's color assignment.
func (r *Repository) RemoveTagColor(tag string) (TagColors, error) {
	return r.mutateTagColors(func(tc TagColors) (bool, error) {
		delete(tc, tag)
		return true, nil
	})
}
