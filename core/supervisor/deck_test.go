package supervisor

import (
	"strings"
	"testing"

	"bruv/internal/model"
)

func TestAppendDeckSlide(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := rt.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: map[string]any{
			"slides": []any{map[string]any{"id": "s1", "contentTypeId": "title", "values": map[string]any{"title": "One"}}},
			"theme":  map[string]any{"accentColor": "#ff0000"},
		}},
		{ID: "t1", Type: model.BlockText, Label: "Notes", Value: "not a deck"},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}

	// Append a minimal post slide — coercion must stamp an id and keep only
	// known post fields.
	if _, err := rt.AppendDeckSlide(card.ID, "d1", map[string]any{
		"contentTypeId": "post",
		"templateId":    "x-post",
		"values": map[string]any{
			"author": "Harvey", "handle": "@harvey", "text": "hello",
			"bogus": "drop me",
		},
	}); err != nil {
		t.Fatalf("AppendDeckSlide: %v", err)
	}
	// Assert on a fresh disk read — the JSON round-trip is the canonical
	// shape every consumer (frontend, present page) actually sees.
	updated, err := rt.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	val, _ := updated.Blocks[0].Value.(map[string]any)
	slides, _ := val["slides"].([]any)
	if len(slides) != 2 {
		t.Fatalf("expected 2 slides after append, got %d", len(slides))
	}
	appended, _ := slides[1].(map[string]any)
	if id, _ := appended["id"].(string); !strings.HasPrefix(id, "sld-") {
		t.Errorf("appended slide missing stamped id: %v", appended["id"])
	}
	if appended["contentTypeId"] != "post" || appended["templateId"] != "x-post" {
		t.Errorf("content type / template lost: %v", appended)
	}
	vals, _ := appended["values"].(map[string]any)
	if vals["author"] != "Harvey" || vals["text"] != "hello" {
		t.Errorf("post values lost: %v", vals)
	}
	if _, present := vals["bogus"]; present {
		t.Errorf("unknown field should be filtered: %v", vals)
	}
	if theme, _ := val["theme"].(map[string]any); theme == nil || theme["accentColor"] != "#ff0000" {
		t.Errorf("theme lost on append: %v", val["theme"])
	}
	// Existing slide untouched.
	first, _ := slides[0].(map[string]any)
	if first["id"] != "s1" {
		t.Errorf("existing slide disturbed: %v", first)
	}

	// Error cases.
	if _, err := rt.AppendDeckSlide("nope", "d1", map[string]any{"contentTypeId": "title"}); err == nil {
		t.Error("unknown card should error")
	}
	if _, err := rt.AppendDeckSlide(card.ID, "nope", map[string]any{"contentTypeId": "title"}); err == nil {
		t.Error("unknown block should error")
	}
	if _, err := rt.AppendDeckSlide(card.ID, "t1", map[string]any{"contentTypeId": "title"}); err == nil {
		t.Error("non-deck block should error")
	}
	if _, err := rt.AppendDeckSlide(card.ID, "d1", nil); err == nil {
		t.Error("empty slide should error")
	}
}

func TestAppendDeckSlide_EmptyDeckValue(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	// A deck block whose value never got initialised (nil) — the AI/agent
	// creation path can produce this shape.
	if _, err := rt.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: nil},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}
	if _, err := rt.AppendDeckSlide(card.ID, "d1", map[string]any{
		"contentTypeId": "title", "values": map[string]any{"title": "First"},
	}); err != nil {
		t.Fatalf("AppendDeckSlide on nil value: %v", err)
	}
	updated, err := rt.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	val, _ := updated.Blocks[0].Value.(map[string]any)
	slides, _ := val["slides"].([]any)
	if len(slides) != 1 {
		t.Fatalf("expected 1 slide, got %d", len(slides))
	}
}
