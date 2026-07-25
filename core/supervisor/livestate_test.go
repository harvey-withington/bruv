package supervisor

import (
	"encoding/json"
	"testing"
	"time"

	"bruv/core/events"
	"bruv/internal/model"
)

func TestBlockLiveState_RoundTripAndValidation(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := rt.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: map[string]any{
			"slides": []any{map[string]any{"id": "s1", "contentTypeId": "title", "values": map[string]any{"title": "One"}}},
		}},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}

	// Absent state reads as nil, not an error.
	got, err := rt.GetBlockLiveState(card.ID, "d1")
	if err != nil {
		t.Fatalf("GetBlockLiveState: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil before any Set, got %v", got)
	}

	// The set is broadcast as block:live.
	ch, unsub := rt.bus.Subscribe()
	defer unsub()

	if err := rt.SetBlockLiveState(card.ID, "d1", map[string]any{"currentIndex": 2}); err != nil {
		t.Fatalf("SetBlockLiveState: %v", err)
	}
	got, _ = rt.GetBlockLiveState(card.ID, "d1")
	if got == nil || got["currentIndex"] != 2 {
		t.Fatalf("round-trip failed: %v", got)
	}

	// Drain until block:live arrives — the repo watcher publishes its own
	// events for the file writes above, so the first event isn't guaranteed
	// to be ours.
	var ev events.Event
	deadline := time.After(2 * time.Second)
	for ev.Topic != "block:live" {
		select {
		case ev = <-ch:
		case <-deadline:
			t.Fatal("timed out waiting for block:live event")
		}
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok || payload["cardID"] != card.ID || payload["blockID"] != "d1" {
		t.Fatalf("unexpected event payload: %#v", ev.Payload)
	}

	// Bogus IDs must not grow the map.
	if err := rt.SetBlockLiveState("nope", "d1", map[string]any{"currentIndex": 1}); err == nil {
		t.Error("unknown card should error")
	}
	if err := rt.SetBlockLiveState(card.ID, "nope", map[string]any{"currentIndex": 1}); err == nil {
		t.Error("unknown block should error")
	}
	if err := rt.SetBlockLiveState("", "", nil); err == nil {
		t.Error("empty IDs should error")
	}
}

func TestPresentCardJSON_OverlaysLiveIndex(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := rt.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "d1", Type: model.BlockSlideDeck, Label: "Deck", Value: map[string]any{
			"slides": []any{
				map[string]any{"id": "s1", "contentTypeId": "title", "values": map[string]any{"title": "One"}},
				map[string]any{"id": "s2", "contentTypeId": "title", "values": map[string]any{"title": "Two"}},
			},
		}},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}

	deckValue := func() map[string]any {
		raw, ok := rt.PresentCardJSON(card.ID)
		if !ok {
			t.Fatal("PresentCardJSON returned ok=false")
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, b := range m["blocks"].([]any) {
			bm := b.(map[string]any)
			if bm["type"] == model.BlockSlideDeck {
				return bm["value"].(map[string]any)
			}
		}
		t.Fatal("deck block missing from present JSON")
		return nil
	}

	// No live state yet → no overlay (page defaults to slide 0).
	if _, present := deckValue()["currentIndex"]; present {
		t.Error("no live state set, currentIndex should be absent")
	}

	// Live state overlays — this is also the "start at slide N" path:
	// seed the position, then open the present URL. Extra keys (video
	// play commands, future controls) pass through untouched; the deck's
	// own persisted keys must never be shadowed.
	if err := rt.SetBlockLiveState(card.ID, "d1", map[string]any{
		"currentIndex": 1,
		"videoSeq":     3,
		"videoAction":  "play",
		"slides":       "must not shadow",
	}); err != nil {
		t.Fatalf("SetBlockLiveState: %v", err)
	}
	val := deckValue()
	if got := val["currentIndex"]; got != float64(1) {
		t.Errorf("live index not overlaid: %v", got)
	}
	if val["videoSeq"] != float64(3) || val["videoAction"] != "play" {
		t.Errorf("live video command not overlaid: %v", val)
	}
	if _, isString := val["slides"].(string); isString {
		t.Error("live state shadowed the persisted slides key")
	}

	// The persisted card stays untouched.
	fresh, err := rt.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	freshVal, _ := fresh.Blocks[0].Value.(map[string]any)
	if _, present := freshVal["currentIndex"]; present {
		t.Error("live index leaked into the persisted block value")
	}
}
