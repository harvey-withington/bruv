package supervisor

import (
	"strings"
	"testing"
)

func TestPresentGate(t *testing.T) {
	rt := newTestRuntime(t)
	card, err := rt.CreateCard("", "Deck")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	listed := func() bool {
		ids, err := rt.ListPresentingCards()
		if err != nil {
			t.Fatalf("ListPresentingCards: %v", err)
		}
		for _, id := range ids {
			if id == card.ID {
				return true
			}
		}
		return false
	}

	// Gate closed: /present-data serves the not-presenting payload — for
	// existing AND unknown cards identically (no existence probing).
	raw, ok := rt.PresentCardJSON(card.ID)
	if !ok || !strings.Contains(string(raw), `"presenting":false`) {
		t.Fatalf("closed gate should serve not-presenting payload, got ok=%v %s", ok, raw)
	}
	rawUnknown, okUnknown := rt.PresentCardJSON("nope")
	if !okUnknown || string(rawUnknown) != string(raw) {
		t.Fatalf("closed gate must not distinguish unknown cards: ok=%v %s", okUnknown, rawUnknown)
	}
	if listed() {
		t.Fatal("card should not be listed before starting")
	}

	// Start → listed, content served.
	if err := rt.SetPresenting(card.ID, true); err != nil {
		t.Fatalf("SetPresenting(true): %v", err)
	}
	if !listed() {
		t.Fatal("card should be listed while presenting")
	}
	raw, ok = rt.PresentCardJSON(card.ID)
	if !ok || strings.Contains(string(raw), `"presenting":false`) {
		t.Fatalf("open gate should serve the card, got ok=%v %s", ok, raw)
	}

	// Idempotent start, then stop → gated again.
	if err := rt.SetPresenting(card.ID, true); err != nil {
		t.Fatalf("idempotent start: %v", err)
	}
	if err := rt.SetPresenting(card.ID, false); err != nil {
		t.Fatalf("SetPresenting(false): %v", err)
	}
	if listed() {
		t.Fatal("card should not be listed after stopping")
	}
	if raw, ok = rt.PresentCardJSON(card.ID); !ok || !strings.Contains(string(raw), `"presenting":false`) {
		t.Fatalf("stopped gate should serve not-presenting payload, got ok=%v %s", ok, raw)
	}

	// Starting an unknown card errors; stopping one is a no-op.
	if err := rt.SetPresenting("nope", true); err == nil {
		t.Error("starting an unknown card should error")
	}
	if err := rt.SetPresenting("nope", false); err != nil {
		t.Errorf("stopping an unknown card should be a no-op, got %v", err)
	}
}
