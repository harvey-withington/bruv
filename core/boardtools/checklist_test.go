package boardtools_test

// From dogfooding the UNCLI chat (2026-11-04): appending to a checklist
// kept the items and ticks already there, get_card names labels instead
// of returning raw ids, and "&" comes back as "&".

import (
	"strings"
	"testing"

	"bruv/internal/model"
)

func TestAddChecklistItemsKeepsWhatsThere(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Trip"})
	id := created["card_id"].(string)
	if _, text, isErr := call(t, rt, nil, "add_card_blocks", map[string]any{
		"card_id": id,
		"blocks": []any{map[string]any{"type": "checklist", "label": "Packing", "value": []any{
			map[string]any{"text": "Passport", "done": true}, "Charger & cables",
		}}},
	}); isErr {
		t.Fatalf("add_card_blocks with {text, done} items: %s", text)
	}
	before := checklistItems(t, rt.GetCard, id)

	out, text, isErr := call(t, rt, nil, "add_checklist_items", map[string]any{
		"card_id": id, "checklist": "packing", "items": []any{"Sunscreen", map[string]any{"text": "Adapter", "done": true}},
	})
	if isErr || out["items_added"] != float64(2) {
		t.Fatalf("add_checklist_items: %s", text)
	}
	after := checklistItems(t, rt.GetCard, id)
	if len(after) != 4 || after[0]["id"] != before[0]["id"] || after[0]["done"] != true || after[2]["text"] != "Sunscreen" || after[3]["done"] != true {
		t.Errorf("items after append = %v", after)
	}

	if _, text, isErr := call(t, rt, nil, "add_checklist_items", map[string]any{"card_id": id, "checklist": "Groceries", "items": []any{"x"}}); !isErr || !strings.Contains(text, `"Packing"`) {
		t.Errorf("unknown checklist should name the card's checklists: %s", text)
	}

	// get_card: "&" as is.
	_, got, _ := call(t, rt, nil, "get_card", map[string]any{"card_id": id})
	if !strings.Contains(got, "Charger & cables") {
		t.Errorf("get_card escaped '&': %s", got)
	}
}

func TestGetCardNamesLabels(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{
		"title": "Labelled", "brand": "Home", "stream": "Admin", "project": "Travel", "category": "Ideas",
	})
	id := created["card_id"].(string)
	crumbs, err := rt.GetCardPinBreadcrumbs(id)
	if err != nil || len(crumbs) == 0 {
		t.Fatalf("breadcrumbs: %v %v", crumbs, err)
	}
	p := crumbs[0]
	labels, err := rt.AddProjectLabel(p.BrandSlug, p.StreamSlug, p.ProjectSlug, "Urgent", "#ff0000")
	if err != nil || len(labels) == 0 {
		t.Fatalf("AddProjectLabel: %v", err)
	}
	if _, err := rt.UpdateCardLabels(id, []string{labels[0].ID, "unknown-label"}); err != nil {
		t.Fatal(err)
	}
	out, text, _ := call(t, rt, nil, "get_card", map[string]any{"card_id": id})
	names, _ := out["label_names"].([]any)
	if len(names) != 2 || names[0] != "Urgent" || names[1] != "unknown-label" {
		t.Errorf("label_names = %v (%s)", names, text)
	}
}

func checklistItems(t *testing.T, get func(string) (*model.Card, error), id string) []map[string]any {
	t.Helper()
	card, err := get(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range card.Blocks {
		if b.Type != model.BlockChecklist {
			continue
		}
		raw, _ := b.Value.([]any)
		items := make([]map[string]any, 0, len(raw))
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				items = append(items, m)
			}
		}
		return items
	}
	t.Fatal("no checklist")
	return nil
}
