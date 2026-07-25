package supervisor

import (
	"testing"
	"time"
)

func TestPresentWatch_ActiveAndIdle(t *testing.T) {
	old := presentIdleTimeout
	presentIdleTimeout = 150 * time.Millisecond
	t.Cleanup(func() { presentIdleTimeout = old })

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

	if listed() {
		t.Fatal("card should not be presenting before any poll")
	}

	// A resolve counts as a poll → active.
	if _, ok := rt.PresentCardJSON(card.ID); !ok {
		t.Fatal("PresentCardJSON failed")
	}
	if !listed() {
		t.Fatal("card should be presenting after a poll")
	}

	// Repeated polls keep it active past the first timeout window.
	time.Sleep(100 * time.Millisecond)
	rt.notePresentPoll(card.ID)
	time.Sleep(100 * time.Millisecond)
	if !listed() {
		t.Fatal("repolling should keep the card presenting")
	}

	// Quiet → idle after the timeout.
	deadline := time.Now().Add(2 * time.Second)
	for listed() {
		if time.Now().After(deadline) {
			t.Fatal("card never went idle after polls stopped")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
