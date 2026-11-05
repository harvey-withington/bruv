package boardtools_test

// From dogfooding the UNCLI chat in Suggest (2026-11-05): the model staged
// add_checklist_items for a checklist the card didn't have; it failed only
// when the user applied it. Preflight refuses it at staging, with the fix.

import (
	"strings"
	"testing"

	"bruv/core/boardtools"
)

func TestPreflight(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Trip"})
	id := created["card_id"].(string)
	call(t, rt, nil, "add_card_blocks", map[string]any{
		"card_id": id,
		"blocks":  []any{map[string]any{"type": "checklist", "label": "Packing", "key": "packing", "value": []any{"Passport"}}},
	})

	for _, tc := range []struct {
		name, tool string
		args       map[string]any
		want       string // "" = passes
	}{
		{"existing checklist", "add_checklist_items", map[string]any{"card_id": id, "checklist": "Packing", "items": []any{"Adapter"}}, ""},
		{"no such checklist", "add_checklist_items", map[string]any{"card_id": id, "checklist": "Groceries", "items": []any{"Milk"}}, "add_card_blocks"},
		{"no items", "add_checklist_items", map[string]any{"card_id": id, "checklist": "Packing"}, "items is required"},
		{"no such card", "set_card_title", map[string]any{"card_id": "nope", "title": "x"}, "no card nope"},
		{"duplicate block key", "add_card_blocks", map[string]any{"card_id": id, "blocks": []any{map[string]any{"type": "text", "key": "packing", "value": "x"}}}, "packing"},
		{"new block", "add_card_blocks", map[string]any{"card_id": id, "blocks": []any{map[string]any{"type": "text", "label": "Notes", "value": "x"}}}, ""},
		{"unknown type", "set_card_type", map[string]any{"card_id": id, "card_type": "Spaceship"}, "create_card_type"},
		{"unknown type on create", "create_card", map[string]any{"title": "x", "card_type": "Spaceship"}, "create_card_type"},
		{"not card-scoped", "create_brand", map[string]any{"name": "x"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := boardtools.Preflight(rt, tc.tool, tc.args)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("refused: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	// Preflight changed nothing.
	if items := checklistItems(t, rt.GetCard, id); len(items) != 1 {
		t.Errorf("checklist after preflight = %v", items)
	}
}
