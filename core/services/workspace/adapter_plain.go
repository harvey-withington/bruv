package workspace

import (
	"context"
	"fmt"
	"strings"

	wsengine "bruv/core/workspace"
	"bruv/internal/model"
)

// plainAdapter is the universal fallback: tree, file types, sizes, counts,
// top-level README if present.
type plainAdapter struct{}

func (*plainAdapter) Name() string { return "plain-folder" }

// Detect scores low so any specific adapter wins.
func (*plainAdapter) Detect([]model.WorkspaceEntry) float64 { return 0.1 }

func (*plainAdapter) Index(ctx context.Context, fs wsengine.FS) (*model.WorkspaceIndex, error) {
	tree, truncated, err := fs.List(ctx)
	if err != nil {
		return nil, err
	}
	tree = dropStateDirs(tree, ".git", ".obsidian")
	files, dirs, size := treeStats(tree)

	idx := &model.WorkspaceIndex{
		Adapter: "plain-folder",
		Details: map[string]string{"types": extHistogram(tree, 5)},
		Tree:    tree,
	}
	summary := fmt.Sprintf("Folder with %d files in %d folders (%s).", files, dirs, humanBytes(size))
	if name, ok := findRootFile(tree, "README.md", "README.txt", "README"); ok {
		if raw, err := fs.Read(ctx, name, 64<<10); err == nil {
			if first := firstProseLine(string(raw)); first != "" {
				summary += " README: " + first
			}
		}
	}
	idx.Summary = summary
	if truncated {
		idx.Warnings = append(idx.Warnings, fmt.Sprintf("tree truncated at %d entries", wsengine.MaxIndexEntries))
	}
	return idx, nil
}

// firstProseLine returns the first non-empty prose line — headings are
// skipped (they usually repeat the folder/project name), capped for summary
// use.
func firstProseLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 200 {
			line = line[:200] + "…"
		}
		return line
	}
	return ""
}
