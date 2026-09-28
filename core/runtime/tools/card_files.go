package tools

// Card-scoped workspace reads (plan/2026-09-17 workspace files block.md).
//
// A card's Workspace Files block names the files the card is about, so
// card chat may read exactly those: an entry that is a file, or anything
// under an entry that is a folder. Nothing else in the workspace is
// reachable from a card — the block IS the scope. Read-only, like every
// workspace tool; AI write access stays out of scope by spec.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bruv/core/runtime/promptfmt"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// CardFilePaths lists the paths a card's Workspace Files blocks name
// (folders suffixed "/"), for the tool description. Empty when the card
// names none — the tool is then not offered at all.
func CardFilePaths(card *model.Card) []string {
	if card == nil {
		return nil
	}
	var out []string
	for _, b := range card.Blocks {
		if b.Type == model.BlockWorkspaceFiles {
			out = append(out, promptfmt.WorkspaceFilePaths(b)...)
		}
	}
	return out
}

// cardFileEntries flattens every Workspace Files block on the card.
func cardFileEntries(card *model.Card) []model.WorkspaceFileEntry {
	var out []model.WorkspaceFileEntry
	for _, b := range card.Blocks {
		if b.Type != model.BlockWorkspaceFiles {
			continue
		}
		switch items := b.Value.(type) {
		case []model.WorkspaceFileEntry:
			out = append(out, items...)
		case []any:
			for _, it := range items {
				m, ok := it.(map[string]any)
				if !ok {
					continue
				}
				p, _ := m["path"].(string)
				ws, _ := m["workspace_id"].(string)
				if p == "" || ws == "" {
					continue
				}
				isDir, _ := m["is_dir"].(bool)
				out = append(out, model.WorkspaceFileEntry{WorkspaceID: ws, Path: p, IsDir: isDir})
			}
		}
	}
	return out
}

// allowedCardFile returns the entry that grants access to path: the file
// itself, or a folder entry it sits under. Paths compare after trimming
// slashes so "Chapters/" and "Chapters/01.md" line up.
func allowedCardFile(entries []model.WorkspaceFileEntry, path string) (model.WorkspaceFileEntry, bool) {
	clean := strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	for _, e := range entries {
		ep := strings.Trim(e.Path, "/")
		if clean == ep && !e.IsDir {
			return e, true
		}
		if e.IsDir && (clean == ep || strings.HasPrefix(clean, ep+"/")) {
			return e, true
		}
	}
	return model.WorkspaceFileEntry{}, false
}

func (d *Dispatcher) toolReadCardFile(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	path, _ := tc.Arguments["path"].(string)
	if path == "" {
		return "error: path is required", nil, nil
	}
	if card == nil {
		return "error: card unavailable", nil, nil
	}
	entries := cardFileEntries(card)
	entry, ok := allowedCardFile(entries, path)
	if !ok {
		return fmt.Sprintf("error: %s is not one of this card's workspace files — only the paths listed in the card's Workspace Files block (or files inside its folders) can be read", path), nil, nil
	}
	svc := d.deps.Workspace()
	r := d.deps.Repo()
	if svc == nil || r == nil {
		return "error: workspace service unavailable", nil, nil
	}
	refs, err := r.ListWorkspaces()
	if err != nil {
		return workspaceToolError(err), nil, nil
	}
	for _, ref := range refs {
		if ref.Workspace.ID != entry.WorkspaceID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		content, err := svc.ReadFile(ctx, ref.BrandSlug, ref.StreamSlug, ref.ProjectSlug, strings.Trim(path, "/"))
		cancel()
		if err != nil {
			return workspaceToolError(err), nil, nil
		}
		if len(content) > workspaceToolReadBytes {
			content = content[:workspaceToolReadBytes] + fmt.Sprintf("\n\n[truncated — file is %d bytes, showing first %d]", len(content), workspaceToolReadBytes)
		}
		return content, nil, nil
	}
	return "error: the workspace this file belongs to is no longer attached", nil, nil
}
