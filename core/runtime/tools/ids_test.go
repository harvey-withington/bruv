package tools

import (
	"testing"
	"time"
)

// Items are ID-keyed state: rewriting a list or checklist must keep the
// ids the caller supplied, minting ids only for new items.
func TestCoerceKeepsSuppliedItemIDs(t *testing.T) {
	checklist := coerceChecklist([]any{
		map[string]any{"id": "cli-keep", "text": "existing", "done": true},
		"new item",
	})
	if checklist[0]["id"] != "cli-keep" || checklist[0]["done"] != true {
		t.Errorf("checklist item 0 = %v, want the supplied id kept", checklist[0])
	}
	if id, _ := checklist[1]["id"].(string); id == "" || id == "cli-keep" {
		t.Errorf("new checklist item needs its own id, got %q", id)
	}

	list := coerceList([]any{map[string]any{"id": "li-keep", "text": "existing"}, "new"})
	if list[0]["id"] != "li-keep" {
		t.Errorf("list item 0 = %v, want the supplied id kept", list[0])
	}
	if id, _ := list[1]["id"].(string); id == "" || id == "li-keep" {
		t.Errorf("new list item needs its own id, got %q", id)
	}
}

// A zone-less date-time is a wall-clock time and is read in local time
// (as agentsvc's timeArg does), not as UTC.
func TestNormaliseDateValueZonelessIsLocal(t *testing.T) {
	got, err := normaliseDateValue("2026-10-04T14:35", "date-time")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 4, 14, 35, 0, 0, time.Local).Format(time.RFC3339)
	if got != want {
		t.Errorf("got %s, want %s (local wall-clock time)", got, want)
	}
	// An explicit offset still wins.
	if got, _ := normaliseDateValue("2026-10-04T14:35:00+10:00", "date-time"); got != "2026-10-04T14:35:00+10:00" {
		t.Errorf("offset not kept: %s", got)
	}
}

func TestParseBlocksRejectsUnknownType(t *testing.T) {
	if _, err := ParseBlocks([]any{map[string]any{"type": "sparkles", "value": "x"}}); err == nil {
		t.Error("an unknown block type must be refused")
	}
	blocks, err := ParseBlocks([]any{map[string]any{"value": "untyped is text"}})
	if err != nil || len(blocks) != 1 || blocks[0].Type != "text" {
		t.Errorf("an omitted type defaults to text: %v %v", blocks, err)
	}
}
