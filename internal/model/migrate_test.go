package model

import (
	"testing"
)

func TestMigrateCardToBlocks_NoopWhenBlocksExist(t *testing.T) {
	card := &Card{
		Fields: map[string]any{"description": "hello"},
		Blocks: []Block{{ID: "existing", Type: BlockText}},
	}
	MigrateCardToBlocks(card, nil)
	if len(card.Blocks) != 1 || card.Blocks[0].ID != "existing" {
		t.Fatal("migration should be a no-op when Blocks already has entries")
	}
}

func TestMigrateCardToBlocks_NoopWhenNoLegacy(t *testing.T) {
	card := &Card{}
	MigrateCardToBlocks(card, nil)
	if len(card.Blocks) != 0 {
		t.Fatal("migration should be a no-op when no legacy data exists")
	}
}

func TestMigrateCardToBlocks_FieldsMigrated(t *testing.T) {
	card := &Card{
		Fields: map[string]any{
			"description":      "A great episode",
			"duration_minutes": float64(45),
			"published":        true,
		},
	}
	MigrateCardToBlocks(card, nil)

	if len(card.Blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(card.Blocks))
	}

	// Verify block types were inferred correctly
	blocksByKey := make(map[string]Block)
	for _, b := range card.Blocks {
		blocksByKey[b.Key] = b
	}

	if b, ok := blocksByKey["description"]; !ok || b.Type != BlockText {
		t.Error("description should be text block")
	}
	if b, ok := blocksByKey["duration_minutes"]; !ok || b.Type != BlockNumber {
		t.Error("duration_minutes should be number block")
	}
	if b, ok := blocksByKey["published"]; !ok || b.Type != BlockCheckbox {
		t.Error("published should be checkbox block")
	}
}

func TestMigrateCardToBlocks_WithFieldHints(t *testing.T) {
	card := &Card{
		Fields: map[string]any{
			"recording_status": "not_started",
		},
	}
	hints := map[string]FieldHint{
		"recording_status": {
			BlockType: BlockSelect,
			Label:     "Recording Status",
			Required:  false,
			Meta:      map[string]any{"options": []string{"not_started", "recorded", "edited"}},
		},
	}
	MigrateCardToBlocks(card, hints)

	if len(card.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(card.Blocks))
	}

	b := card.Blocks[0]
	if b.Type != BlockSelect {
		t.Errorf("expected select block, got %s", b.Type)
	}
	if b.Label != "Recording Status" {
		t.Errorf("expected label 'Recording Status', got %q", b.Label)
	}
	if b.Meta == nil {
		t.Error("expected meta with options")
	}
}

func TestMigrateCardToBlocks_ChecklistMigrated(t *testing.T) {
	card := &Card{
		Fields: map[string]any{},
		Checklist: []ChecklistItem{
			{ID: "ck-1", Text: "Buy milk", Done: false},
			{ID: "ck-2", Text: "Write code", Done: true},
		},
	}
	MigrateCardToBlocks(card, nil)

	if len(card.Blocks) != 1 {
		t.Fatalf("expected 1 checklist block, got %d blocks", len(card.Blocks))
	}

	b := card.Blocks[0]
	if b.Type != BlockChecklist {
		t.Errorf("expected checklist block, got %s", b.Type)
	}
	if b.Label != "Checklist" {
		t.Errorf("expected label 'Checklist', got %q", b.Label)
	}

	items, ok := b.Value.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any value, got %T", b.Value)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["text"] != "Buy milk" {
		t.Errorf("expected first item text 'Buy milk', got %v", items[0]["text"])
	}
	if items[1]["done"] != true {
		t.Errorf("expected second item done=true, got %v", items[1]["done"])
	}
}

func TestMigrateCardToBlocks_AttachmentsMigrated(t *testing.T) {
	card := &Card{
		Fields:      map[string]any{},
		Attachments: []string{"photo.jpg", "doc.pdf", "image.png"},
	}
	MigrateCardToBlocks(card, nil)

	if len(card.Blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(card.Blocks))
	}

	if card.Blocks[0].Type != BlockImage {
		t.Errorf("photo.jpg should be image block, got %s", card.Blocks[0].Type)
	}
	if card.Blocks[1].Type != BlockURL {
		t.Errorf("doc.pdf should be url block, got %s", card.Blocks[1].Type)
	}
	if card.Blocks[2].Type != BlockImage {
		t.Errorf("image.png should be image block, got %s", card.Blocks[2].Type)
	}
}

func TestMigrateCardToBlocks_LegacyPreserved(t *testing.T) {
	card := &Card{
		Fields:      map[string]any{"description": "hello"},
		Checklist:   []ChecklistItem{{ID: "ck-1", Text: "item", Done: false}},
		Attachments: []string{"file.txt"},
	}
	MigrateCardToBlocks(card, nil)

	// Legacy fields should still be intact
	if card.Fields["description"] != "hello" {
		t.Error("legacy Fields should be preserved")
	}
	if len(card.Checklist) != 1 {
		t.Error("legacy Checklist should be preserved")
	}
	if len(card.Attachments) != 1 {
		t.Error("legacy Attachments should be preserved")
	}
}

func TestHumanizeKey(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"recording_status", "Recording Status"},
		{"description", "Description"},
		{"show_notes", "Show Notes"},
		{"a", "A"},
		{"", ""},
	}
	for _, tt := range tests {
		got := humanizeKey(tt.input)
		if got != tt.want {
			t.Errorf("humanizeKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBlockIDFormat(t *testing.T) {
	id := blockID()
	if len(id) < 4 || id[:4] != "blk-" {
		t.Errorf("blockID() = %q, want prefix 'blk-'", id)
	}
}
