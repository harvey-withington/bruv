package mcpserver

// Card types are never invented by MCP callers: create_card and
// set_card_type accept an existing type's id or label only, and a card
// created without a type gets the built-in default.

import (
	"testing"

	"bruv/core/services/catalog"
)

func TestCreateCardDefaultsToBuiltinType(t *testing.T) {
	h, _ := newTestHandler(t)
	var created struct {
		Type string `json:"type"`
	}
	decodeJSON(t, mustCallTool(t, h, "create_card", map[string]any{"title": "No type given"}), &created)
	if created.Type != catalog.DefaultCardType {
		t.Errorf("type = %q, want the built-in default %q", created.Type, catalog.DefaultCardType)
	}
}

func TestCreateCardResolvesTypeByLabel(t *testing.T) {
	h, _ := newTestHandler(t)
	var created struct {
		Type string `json:"type"`
	}
	decodeJSON(t, mustCallTool(t, h, "create_card", map[string]any{"title": "Labelled", "card_type": "Reference"}), &created)
	if created.Type != "reference" {
		t.Errorf("type = %q, want the canonical id 'reference'", created.Type)
	}
}

func TestCreateCardRejectsUnknownType(t *testing.T) {
	h, sup := newTestHandler(t)
	before, _ := sup.Resolve(testRepoID).ListCards()
	if text, isErr := callToolRPC(t, h, "create_card", map[string]any{"title": "Bogus", "card_type": "idea"}); !isErr {
		t.Fatalf("expected an unknown-type error, got %s", text)
	}
	after, _ := sup.Resolve(testRepoID).ListCards()
	if len(after) != len(before) {
		t.Error("a card was created despite the unknown type")
	}
}

func TestSetCardTypeRejectsUnknownType(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	if text, isErr := callToolRPC(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "idea"}); !isErr {
		t.Fatalf("expected an unknown-type error, got %s", text)
	}
	var out struct {
		Type string `json:"type"`
	}
	decodeJSON(t, mustCallTool(t, h, "set_card_type", map[string]any{"card_id": cardID, "card_type": "TASK"}), &out)
	if out.Type != "task" {
		t.Errorf("type = %q, want 'task'", out.Type)
	}
}
