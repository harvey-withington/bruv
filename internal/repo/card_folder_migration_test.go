package repo

// A pre-2026-09 card with an intrinsic Card Folder loads as a card with a
// Workspace Files block holding that folder — the on-disk shape keeps
// working, the feature it belonged to doesn't.

import (
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/model"
)

func TestLegacyCardFolderBecomesWorkspaceFilesBlock(t *testing.T) {
	r, err := InitAt(filepath.Join(t.TempDir(), "vault"), "Vault")
	if err != nil {
		t.Fatal(err)
	}
	card, err := r.CreateCard("", "Patient Zero")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.cardFilePath(card.ID))
	if err != nil {
		t.Fatal(err)
	}
	// Splice the legacy field in as an old build would have written it.
	legacy := string(raw[:len(raw)-1]) // drop closing brace
	legacy = legacy[:lastNonSpace(legacy)+1] + `,"folder":{"workspace_id":"ws-1","path":"Episodes/EP002"}}`
	if err := os.WriteFile(r.cardFilePath(card.ID), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := r.GetCard(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	var block *model.Block
	for i := range loaded.Blocks {
		if loaded.Blocks[i].Type == model.BlockWorkspaceFiles {
			block = &loaded.Blocks[i]
		}
	}
	if block == nil {
		t.Fatalf("no workspace_files block after migration: %+v", loaded.Blocks)
	}
	entries, ok := block.Value.([]model.WorkspaceFileEntry)
	if !ok || len(entries) != 1 {
		t.Fatalf("block value = %#v", block.Value)
	}
	if entries[0].WorkspaceID != "ws-1" || entries[0].Path != "Episodes/EP002" || !entries[0].IsDir || entries[0].ID == "" {
		t.Errorf("entry = %+v", entries[0])
	}
}

func lastNonSpace(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\n', '\r', '\t':
			continue
		}
		return i
	}
	return -1
}
