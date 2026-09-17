package tools

import (
	"testing"

	"bruv/internal/model"
)

func wsCard(entries ...model.WorkspaceFileEntry) *model.Card {
	return &model.Card{Blocks: []model.Block{{Type: model.BlockWorkspaceFiles, Value: entries}}}
}

func TestAllowedCardFileScopesToTheBlock(t *testing.T) {
	entries := []model.WorkspaceFileEntry{
		{WorkspaceID: "ws", Path: "notes.md"},
		{WorkspaceID: "ws", Path: "Chapters", IsDir: true},
	}
	for _, ok := range []string{"notes.md", "/notes.md", "Chapters/01.md", "Chapters/part/02.md", "Chapters"} {
		if _, allowed := allowedCardFile(entries, ok); !allowed {
			t.Errorf("%s should be readable", ok)
		}
	}
	for _, bad := range []string{"secrets.md", "Chapters2/01.md", "notes.md/x", "../notes.md"} {
		if _, allowed := allowedCardFile(entries, bad); allowed {
			t.Errorf("%s must not be readable — only the card's files are in scope", bad)
		}
	}
}

func TestCardFilePathsReadsBothValueShapes(t *testing.T) {
	typed := wsCard(model.WorkspaceFileEntry{WorkspaceID: "ws", Path: "a.md"}, model.WorkspaceFileEntry{WorkspaceID: "ws", Path: "Docs", IsDir: true})
	if got := CardFilePaths(typed); len(got) != 2 || got[0] != "a.md" || got[1] != "Docs/" {
		t.Errorf("typed paths = %v", got)
	}
	decoded := &model.Card{Blocks: []model.Block{{Type: model.BlockWorkspaceFiles, Value: []any{
		map[string]any{"workspace_id": "ws", "path": "b.md"},
		map[string]any{"workspace_id": "ws", "path": "Src", "is_dir": true},
	}}}}
	if got := CardFilePaths(decoded); len(got) != 2 || got[1] != "Src/" {
		t.Errorf("decoded paths = %v", got)
	}
	if got := cardFileEntries(decoded); len(got) != 2 || !got[1].IsDir {
		t.Errorf("decoded entries = %+v", got)
	}
	if CardFilePaths(nil) != nil || len(CardFilePaths(&model.Card{})) != 0 {
		t.Error("a card without the block names no files")
	}
}
