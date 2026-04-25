package promptfmt

import (
	"bruv/internal/model"
	"strings"
	"testing"
)

func TestAvailableIconListHasKnownEntries(t *testing.T) {
	list := AvailableIconList()
	for _, name := range []string{"folder", "star", "calendar", "bell", "code"} {
		if !strings.Contains(list, name) {
			t.Errorf("icon list missing known entry %q", name)
		}
	}
}

func TestRenderCategoryHeaderIncludesID(t *testing.T) {
	cat := model.Category{
		ID:   "cat-xyz",
		Name: "To Review",
	}
	out := RenderCategoryHeader(cat)
	if !strings.Contains(out, "cat-xyz") {
		t.Errorf("output missing ID: %q", out)
	}
	if !strings.Contains(out, "To Review") {
		t.Errorf("output missing name: %q", out)
	}
}

func TestRenderCategoryHeaderAcceptedTypes(t *testing.T) {
	cat := model.Category{
		ID:            "c",
		Name:          "Bugs",
		AcceptedTypes: []string{"task", "bug"},
	}
	out := RenderCategoryHeader(cat)
	if !strings.Contains(out, "task, bug") {
		t.Errorf("accepted types not rendered: %q", out)
	}
}

func TestFormatBlockValueForPromptEmptyValue(t *testing.T) {
	b := model.Block{Type: model.BlockText, Value: nil}
	if got := FormatBlockValueForPrompt(b); got != "" {
		t.Errorf("nil value = %q, want empty", got)
	}
}

func TestFormatBlockValueForPromptChecklist(t *testing.T) {
	b := model.Block{
		Type: model.BlockChecklist,
		Value: []any{
			map[string]any{"text": "buy milk", "done": true},
			map[string]any{"text": "walk dog", "done": false},
		},
	}
	out := FormatBlockValueForPrompt(b)
	if !strings.Contains(out, "[x] buy milk") {
		t.Errorf("done item not marked: %q", out)
	}
	if !strings.Contains(out, "[ ] walk dog") {
		t.Errorf("undone item not marked: %q", out)
	}
}

func TestFormatBlockValueForPromptTextTruncates(t *testing.T) {
	long := strings.Repeat("a", 500)
	b := model.Block{Type: model.BlockText, Value: long}
	out := FormatBlockValueForPrompt(b)
	if len(out) >= 500 {
		t.Errorf("text not truncated; length %d", len(out))
	}
	if !strings.HasSuffix(out, "…") {
		t.Errorf("truncation marker missing: %q", out[len(out)-5:])
	}
}

func TestCollectAgentFieldsOrdering(t *testing.T) {
	// Known fields should come out in the canonical order regardless
	// of block order on the card — this is load-bearing for prompt
	// caching.
	card := &model.Card{
		Blocks: []model.Block{
			{Key: "findings", Type: model.BlockText},
			{Key: "status", Type: model.BlockSelect},
			{Key: "last_run", Type: model.BlockText},
		},
	}
	fields := CollectAgentFields(card)
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	if fields[0].Key != "status" {
		t.Errorf("first field = %q, want status (canonical order)", fields[0].Key)
	}
	if fields[1].Key != "last_run" {
		t.Errorf("second field = %q, want last_run", fields[1].Key)
	}
	if fields[2].Key != "findings" {
		t.Errorf("third field = %q, want findings", fields[2].Key)
	}
}

func TestCollectAgentFieldsCustomTracking(t *testing.T) {
	// Custom keys that look like tracking fields (by block type)
	// should come along; freeform text/markdown blocks should not.
	card := &model.Card{
		Blocks: []model.Block{
			{Key: "health_score", Type: model.BlockNumber},
			{Key: "notes", Type: model.BlockText}, // freeform — skip
			{Key: "rating", Type: model.BlockRating},
			{Key: "", Type: model.BlockNumber}, // no key — skip
		},
	}
	fields := CollectAgentFields(card)
	var keys []string
	for _, f := range fields {
		keys = append(keys, f.Key)
	}
	wantPresent := []string{"health_score", "rating"}
	wantAbsent := []string{"notes"}
	for _, k := range wantPresent {
		found := false
		for _, got := range keys {
			if got == k {
				found = true
			}
		}
		if !found {
			t.Errorf("custom tracking field %q missing from %v", k, keys)
		}
	}
	for _, k := range wantAbsent {
		for _, got := range keys {
			if got == k {
				t.Errorf("freeform block %q should not be flagged as agent field", k)
			}
		}
	}
}

func TestFormatCardContentBasics(t *testing.T) {
	card := &model.Card{
		Title: "Test card",
		Type:  "task",
		Tags:  []string{"urgent", "wip"},
		Blocks: []model.Block{
			{Key: "notes", Label: "Notes", Value: "some content"},
		},
	}
	out := FormatCardContent(card)
	for _, want := range []string{"Test card", "task", "urgent, wip", "Notes", "some content"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted content missing %q:\n%s", want, out)
		}
	}
}
