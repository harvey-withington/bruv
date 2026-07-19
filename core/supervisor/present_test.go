package supervisor

import (
	"encoding/json"
	"strings"
	"testing"

	"bruv/internal/model"
)

func TestSignPresentURL(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	url, err := rt.SignPresentURL(card.ID)
	if err != nil {
		t.Fatalf("SignPresentURL: %v", err)
	}
	if !strings.HasPrefix(url, "/present/") || !strings.Contains(url, card.ID) ||
		!strings.Contains(url, "exp=") || !strings.Contains(url, "sig=") {
		t.Fatalf("unexpected present URL: %s", url)
	}
	if _, err := rt.SignPresentURL(""); err == nil {
		t.Fatal("empty cardID should error")
	}
}

func TestPresentCardJSON_ResolvesBindingsAndAttachments(t *testing.T) {
	rt := newTestRuntime(t)

	// Linked card carrying the source-of-truth content.
	linked, err := rt.CreateCard("", "Source card")
	if err != nil {
		t.Fatalf("CreateCard linked: %v", err)
	}
	if _, err := rt.UpdateCardBlocks(linked.ID, []model.Block{
		{ID: "lb1", Type: model.BlockText, Label: "Quote", Value: "Stay hungry"},
		{ID: "lb2", Type: model.BlockChecklist, Label: "Points", Value: []any{
			map[string]any{"id": "c1", "text": "one", "done": false},
			map[string]any{"id": "c2", "text": "two", "done": true},
		}},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks linked: %v", err)
	}

	// Deck card: one slide binds quote→lb1, another uses an attachment ref.
	deckCard, err := rt.CreateCard("", "Deck card")
	if err != nil {
		t.Fatalf("CreateCard deck: %v", err)
	}
	deckValue := map[string]any{
		"slides": []any{
			map[string]any{
				"id": "s1", "contentTypeId": "quote",
				"cardId":   linked.ID,
				"bindings": map[string]any{"quote": "lb1"},
				"values":   map[string]any{"author": "Someone"},
			},
			map[string]any{
				"id": "s2", "contentTypeId": "image",
				"values": map[string]any{"image": "attachment:" + deckCard.ID + "/att123", "caption": "Pic"},
			},
		},
		"currentIndex": 0,
	}
	if _, err := rt.UpdateCardBlocks(deckCard.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: deckValue},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks deck: %v", err)
	}

	raw, ok := rt.PresentCardJSON(deckCard.ID)
	if !ok {
		t.Fatal("PresentCardJSON returned ok=false")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blocks := m["blocks"].([]any)
	var slides []any
	for _, b := range blocks {
		bm := b.(map[string]any)
		if bm["type"] == model.BlockSlideDeck {
			slides = bm["value"].(map[string]any)["slides"].([]any)
		}
	}
	if len(slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(slides))
	}

	s1 := slides[0].(map[string]any)["values"].(map[string]any)
	if s1["quote"] != "Stay hungry" {
		t.Errorf("bound quote not resolved: %v", s1)
	}
	if s1["author"] != "Someone" {
		t.Errorf("literal author lost: %v", s1)
	}

	s2 := slides[1].(map[string]any)["values"].(map[string]any)
	img, _ := s2["image"].(string)
	if !strings.HasPrefix(img, "/repos/") || !strings.Contains(img, "/attachments/"+deckCard.ID+"/att123") ||
		!strings.Contains(img, "sig=") {
		t.Errorf("attachment ref not signed: %q", img)
	}
	if s2["caption"] != "Pic" {
		t.Errorf("caption lost: %v", s2)
	}

	// The LIVE card must be untouched (resolver works on a deep copy).
	fresh, err := rt.GetCard(deckCard.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	freshVal, _ := fresh.Blocks[0].Value.(map[string]any)
	freshSlides, _ := freshVal["slides"].([]any)
	fs1, _ := freshSlides[0].(map[string]any)
	fv1, _ := fs1["values"].(map[string]any)
	if _, resolved := fv1["quote"]; resolved {
		t.Error("live card mutated: bound value written back to store")
	}

	// Unknown card → ok=false.
	if _, ok := rt.PresentCardJSON("nope"); ok {
		t.Error("unknown card should return ok=false")
	}
}
