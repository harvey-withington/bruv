package boardtools_test

// set_card_fields / add_card_blocks are the one field implementation for
// MCP, chat (whose set_fields / add_field adapt onto them) and agents.

import (
	"encoding/json"
	"strings"
	"testing"

	"bruv/internal/model"
)

func blockByKey(c *model.Card, key string) *model.Block {
	for i := range c.Blocks {
		if c.Blocks[i].Key == key {
			return &c.Blocks[i]
		}
	}
	return nil
}

func TestSetCardFieldsRecreatesSchemaFieldAndRedirectsDescription(t *testing.T) {
	rt := newBoard(t)
	// A schema field other than "description", which is always the card's
	// intrinsic description rather than a block.
	key := ""
	for _, b := range rt.SchemaBlocks("episode") {
		if b.Key != "" && b.Key != "description" {
			key = b.Key
			break
		}
	}
	if key == "" {
		t.Skip("no schema-backed card type in the built-in registry")
	}
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Ep 1", "card_type": "episode"})
	id := created["card_id"].(string)

	// Drop the schema field so set_card_fields has to add it back.
	card, _ := rt.GetCard(id)
	kept := card.Blocks[:0]
	for _, b := range card.Blocks {
		if b.Key != key {
			kept = append(kept, b)
		}
	}
	if _, err := rt.UpdateCardBlocks(id, kept); err != nil {
		t.Fatal(err)
	}

	out, text, isErr := call(t, rt, nil, "set_card_fields", map[string]any{
		"card_id": id, "fields": map[string]any{key: "x", "description": "About this episode", "nope": 1},
	})
	if isErr {
		t.Fatalf("set_card_fields: %s", text)
	}
	card, _ = rt.GetCard(id)
	if blockByKey(card, key) == nil {
		t.Errorf("schema field %q was not recreated", key)
	}
	if card.Description != "About this episode" || blockByKey(card, "description") != nil {
		t.Errorf("description should land on the card itself, got %q", card.Description)
	}
	if skipped, _ := json.Marshal(out["skipped_unknown_keys"]); string(skipped) != `["nope"]` {
		t.Errorf("unknown keys should be reported, not fail the call: %s", skipped)
	}
}

func TestSetCardFieldsRejectsAnInvalidValueAndSavesNothing(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Watcher", "card_type": "agent"})
	id := created["card_id"].(string)
	card, _ := rt.GetCard(id)
	status := blockByKey(card, "status")
	if status == nil {
		t.Skip("agent template has no status select")
	}
	before, _ := json.Marshal(card.Blocks)
	if _, text, isErr := call(t, rt, nil, "set_card_fields", map[string]any{
		"card_id": id, "fields": map[string]any{"status": "not-an-option", "last_run": "ran"},
	}); !isErr || !strings.Contains(text, "status") {
		t.Fatalf("a select value outside its options must be refused, got %s", text)
	}
	card, _ = rt.GetCard(id)
	if after, _ := json.Marshal(card.Blocks); string(after) != string(before) {
		t.Error("a refused call must not save any of its values")
	}
}

func TestAddCardBlocksRefusesDuplicateKeysAndFillsDefaults(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Trip"})
	id := created["card_id"].(string)

	if _, text, isErr := call(t, rt, nil, "add_card_blocks", map[string]any{
		"card_id": id, "blocks": []any{map[string]any{"type": "checklist", "label": "Packing", "key": "packing"}},
	}); isErr {
		t.Fatalf("add_card_blocks: %s", text)
	}
	card, _ := rt.GetCard(id)
	if b := blockByKey(card, "packing"); b == nil {
		t.Fatal("block not added")
	} else if items, ok := b.Value.([]any); !ok || len(items) != 0 {
		t.Errorf("a checklist added without a value should start empty, got %#v", b.Value)
	}

	if _, text, isErr := call(t, rt, nil, "add_card_blocks", map[string]any{
		"card_id": id, "blocks": []any{map[string]any{"type": "text", "label": "Packing again", "key": "packing"}},
	}); !isErr || !strings.Contains(text, "already has a field") {
		t.Errorf("a second block with the same key must be refused, got %s", text)
	}
}
