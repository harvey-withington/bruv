package workspace

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	wsengine "bruv/core/workspace"
	"bruv/internal/model"
)

// obsidianAdapter indexes Obsidian vaults (or vault subtrees): note count,
// folder structure, tags. Excludes .obsidian/ state files from the index.
type obsidianAdapter struct{}

func (*obsidianAdapter) Name() string { return "obsidian-vault" }

// Detect outranks git-repo: a vault living inside a repo is still primarily
// a vault (spec §13).
func (*obsidianAdapter) Detect(tree []model.WorkspaceEntry) float64 {
	if hasRootEntry(tree, ".obsidian", true) {
		return 0.95
	}
	return 0
}

// inlineTag matches Obsidian inline tags. "# heading" doesn't match (space
// after #); "#tag" and "#a/b" do.
var inlineTag = regexp.MustCompile(`(?:^|\s)#([A-Za-z0-9_][A-Za-z0-9_/-]*)`)

const (
	tagScanMaxFiles = 200
	tagScanMaxBytes = 256 << 10
)

func (*obsidianAdapter) Index(ctx context.Context, fs wsengine.FS) (*model.WorkspaceIndex, error) {
	tree, truncated, err := fs.List(ctx)
	if err != nil {
		return nil, err
	}
	tree = dropStateDirs(tree, ".obsidian", ".git")

	notes := 0
	tags := map[string]bool{}
	scanned := 0
	for _, e := range tree {
		if e.IsDir || !strings.HasSuffix(strings.ToLower(e.Path), ".md") {
			continue
		}
		notes++
		// Bounded tag scan: enough for a useful summary, cheap on big vaults.
		if scanned >= tagScanMaxFiles || e.Size > tagScanMaxBytes {
			continue
		}
		scanned++
		raw, err := fs.Read(ctx, e.Path, tagScanMaxBytes)
		if err != nil {
			continue
		}
		for _, m := range inlineTag.FindAllStringSubmatch(string(raw), -1) {
			tags[m[1]] = true
		}
	}

	_, dirs, _ := treeStats(tree)
	idx := &model.WorkspaceIndex{
		Adapter: "obsidian-vault",
		Details: map[string]string{"notes": fmt.Sprintf("%d", notes)},
		Tree:    tree,
	}
	summary := fmt.Sprintf("Obsidian vault with %d notes in %d folders.", notes, dirs)
	if len(tags) > 0 {
		sorted := make([]string, 0, len(tags))
		for t := range tags {
			sorted = append(sorted, t)
		}
		sort.Strings(sorted)
		if len(sorted) > 15 {
			sorted = sorted[:15]
		}
		idx.Details["tags"] = strings.Join(sorted, ", ")
		summary += fmt.Sprintf(" %d distinct tags.", len(tags))
	}
	idx.Summary = summary
	if truncated {
		idx.Warnings = append(idx.Warnings, fmt.Sprintf("tree truncated at %d entries", wsengine.MaxIndexEntries))
	}
	if scanned == tagScanMaxFiles {
		idx.Warnings = append(idx.Warnings, "tag scan sampled the first 200 notes")
	}
	return idx, nil
}
