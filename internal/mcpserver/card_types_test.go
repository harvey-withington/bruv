package mcpserver

// set_card_type follows the same rules as create_card (see
// TestCreateCardTypeResolution in server_test.go): a type is matched by
// id or label, and an unknown name is REFUSED with the available types
// listed (ruling 2026-09-30) — never created. New types come only from
// the explicit create_card_type tool.

import (
	"strings"
	"testing"
)

func TestSetCardTypeResolvesByLabel(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	var out struct {
		Type string `json:"type"`
	}
	decodeJSON(t, mustCallTool(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "TASK"}), &out)
	if out.Type != "task" {
		t.Errorf("got type=%q, want existing 'task'", out.Type)
	}
}

func TestSetCardTypeRefusesUnknownType(t *testing.T) {
	h, sup := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	text, isErr := callToolRPC(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "Field Note"})
	if !isErr {
		t.Fatalf("an unknown type must be refused, got %s", text)
	}
	if !strings.Contains(text, "task") || !strings.Contains(text, "create_card_type") {
		t.Errorf("refusal should list the available types and the way to add one: %s", text)
	}
	if sup.Resolve(testRepoID).Catalog.CardTypeExists("field-note") {
		t.Error("set_card_type created a type")
	}
	card, err := sup.Resolve(testRepoID).GetCard(cardID)
	if err != nil || card.Type != "brainstorm" {
		t.Errorf("card type changed to %q (err %v)", card.Type, err)
	}
}

func TestSetCardTypeMissingCardCreatesNoType(t *testing.T) {
	h, sup := newTestHandler(t)
	if text, isErr := callToolRPC(t, h, "set_card_type", map[string]any{"card_id": "no-such-card", "card_type": "Stray"}); !isErr {
		t.Fatalf("expected an error for a missing card, got %s", text)
	}
	if sup.Resolve(testRepoID).Catalog.CardTypeExists("stray") {
		t.Error("a type was created for a card that doesn't exist")
	}
}

// create_card_type is the one way a model adds a type: it returns the new
// id (usable straight away), and refuses a label that already names a
// type by id or label, whatever the case.
func TestCreateCardTypeTool(t *testing.T) {
	h, sup := newTestHandler(t)
	var out struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Color string `json:"color"`
	}
	decodeJSON(t, mustCallTool(t, h, "create_card_type", map[string]any{"label": "Field Note", "description": "Notes from the field"}), &out)
	if out.ID != "field-note" || out.Label != "Field Note" || out.Color == "" {
		t.Fatalf("got %+v, want id field-note with a picked colour", out)
	}
	if !sup.Resolve(testRepoID).Catalog.CardTypeExists("field-note") {
		t.Fatal("created type missing from the catalog")
	}
	cardID := createTestCard(t, h, "brainstorm")
	mustCallTool(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "field note"})

	for _, dup := range []string{"FIELD NOTE", "field-note", "Task"} {
		if text, isErr := callToolRPC(t, h, "create_card_type", map[string]any{"label": dup}); !isErr {
			t.Errorf("label %q duplicates an existing type and must be refused, got %s", dup, text)
		}
	}
	if text, isErr := callToolRPC(t, h, "create_card_type", map[string]any{"label": "Recipe", "color": "orange"}); !isErr {
		t.Errorf("a non-hex colour must be refused, got %s", text)
	}
}
