package tools

// The files a card is about (plan/2026-09-17 workspace files block.md):
// its Workspace Files block names them, and read_card_file (boardtools)
// reads exactly those — an entry that is a file, or anything under an
// entry that is a folder. The block IS the scope.

import (
	"strings"

	"bruv/core/runtime/promptfmt"
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

// CardFileEntries flattens every Workspace Files block on the card.
func CardFileEntries(card *model.Card) []model.WorkspaceFileEntry {
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

// AllowedCardFile returns the entry that grants access to path: the file
// itself, or a folder entry it sits under. Paths compare after trimming
// slashes so "Chapters/" and "Chapters/01.md" line up.
func AllowedCardFile(entries []model.WorkspaceFileEntry, path string) (model.WorkspaceFileEntry, bool) {
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
