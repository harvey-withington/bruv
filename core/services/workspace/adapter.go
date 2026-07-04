// Package workspace is the WorkspaceService: attach/detach, adapter indexing,
// Tier 1/2 file access, and template operations (create-from-template, import,
// editor). Vault-side only — the checkout lifecycle (materialize/check-in)
// lives in core/workspace and mounts device-side from M3.
package workspace

import (
	"context"
	"fmt"
	"path"
	"strings"

	wsengine "bruv/core/workspace"
	"bruv/internal/model"
)

// Adapter is a read-only inspector that understands a project structure and
// produces the index/summary. It never writes to the workspace. Adapters are
// deliberately the only extension surface for Workspaces: small, read-only,
// safe.
type Adapter interface {
	// Name is the identifier stored in workspace.json ("plain-folder", …).
	Name() string
	// Detect returns a confidence score (0..1) that this adapter fits the
	// given tree (a raw FS.List result, marker dirs included).
	Detect(tree []model.WorkspaceEntry) float64
	// Index produces the structured index. The service stamps WorkspaceID
	// and GeneratedAt.
	Index(ctx context.Context, fs wsengine.FS) (*model.WorkspaceIndex, error)
}

// adapters in detection order; ties broken by score.
func builtinAdapters() []Adapter {
	return []Adapter{&obsidianAdapter{}, &gitAdapter{}, &plainAdapter{}}
}

// adapterByName resolves a stored adapter name, falling back to plain-folder
// so an index refresh never fails on an unknown/renamed adapter.
func adapterByName(name string) Adapter {
	for _, a := range builtinAdapters() {
		if a.Name() == name {
			return a
		}
	}
	return &plainAdapter{}
}

// detectAdapter picks the highest-scoring adapter for a tree.
func detectAdapter(tree []model.WorkspaceEntry) Adapter {
	best, bestScore := Adapter(&plainAdapter{}), -1.0
	for _, a := range builtinAdapters() {
		if score := a.Detect(tree); score > bestScore {
			best, bestScore = a, score
		}
	}
	return best
}

// hasRootEntry reports whether the tree has a root-level entry with the name.
func hasRootEntry(tree []model.WorkspaceEntry, name string, dir bool) bool {
	for _, e := range tree {
		if e.Path == name && e.IsDir == dir {
			return true
		}
	}
	return false
}

// dropStateDirs removes VCS/app state marker entries (and anything under
// them) from the tree stored in the index.
func dropStateDirs(tree []model.WorkspaceEntry, names ...string) []model.WorkspaceEntry {
	out := make([]model.WorkspaceEntry, 0, len(tree))
	for _, e := range tree {
		skip := false
		for _, n := range names {
			if e.Path == n || strings.HasPrefix(e.Path, n+"/") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}

// treeStats summarizes a tree for adapter summaries.
func treeStats(tree []model.WorkspaceEntry) (files, dirs int, bytes int64) {
	for _, e := range tree {
		if e.IsDir {
			dirs++
		} else {
			files++
			bytes += e.Size
		}
	}
	return
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// findRootFile returns the root-level file whose name matches (case-
// insensitive) one of the candidates, e.g. a README.
func findRootFile(tree []model.WorkspaceEntry, candidates ...string) (string, bool) {
	for _, e := range tree {
		if e.IsDir || strings.Contains(e.Path, "/") {
			continue
		}
		for _, c := range candidates {
			if strings.EqualFold(e.Path, c) {
				return e.Path, true
			}
		}
	}
	return "", false
}

// extHistogram returns "ext (count)" fragments for the most common file
// extensions by total size — the cheap language-breakdown approximation.
func extHistogram(tree []model.WorkspaceEntry, top int) string {
	sizes := map[string]int64{}
	for _, e := range tree {
		if e.IsDir {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(e.Path), "."))
		if ext == "" {
			ext = "(none)"
		}
		sizes[ext] += e.Size
	}
	type kv struct {
		ext  string
		size int64
	}
	var all []kv
	for k, v := range sizes {
		all = append(all, kv{k, v})
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].size > all[i].size {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	if len(all) > top {
		all = all[:top]
	}
	parts := make([]string, 0, len(all))
	for _, e := range all {
		parts = append(parts, fmt.Sprintf("%s (%s)", e.ext, humanBytes(e.size)))
	}
	return strings.Join(parts, ", ")
}
