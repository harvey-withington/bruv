package mcpserver

// set_card_type follows the same rules as create_card (see
// TestCreateCardTypeResolution in server_test.go): a type is matched by
// id or label, an unknown name is created first so the card never holds
// a type that doesn't exist, and a missing card creates nothing.

import (
	"testing"
)

func TestSetCardTypeResolvesByLabel(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	var out struct {
		Type        string `json:"type"`
		TypeCreated bool   `json:"type_created"`
	}
	decodeJSON(t, mustCallTool(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "TASK"}), &out)
	if out.Type != "task" || out.TypeCreated {
		t.Errorf("got type=%q created=%v, want existing 'task'", out.Type, out.TypeCreated)
	}
}

func TestSetCardTypeCreatesUnknownType(t *testing.T) {
	h, sup := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	var out struct {
		Type        string `json:"type"`
		TypeCreated bool   `json:"type_created"`
	}
	decodeJSON(t, mustCallTool(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "Field Note"}), &out)
	if out.Type != "field-note" || !out.TypeCreated {
		t.Fatalf("got type=%q created=%v, want a new 'field-note' type", out.Type, out.TypeCreated)
	}
	if !sup.Resolve(testRepoID).Catalog.CardTypeExists("field-note") {
		t.Error("created type missing from the catalog")
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
