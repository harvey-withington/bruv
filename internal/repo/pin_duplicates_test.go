package repo

// Field report 2026-09-17: a card the AI had pinned to a second category
// was then dragged into a category it was already in. MoveCardToCategory
// rewrote the source pin in place, leaving two pins to one category; the
// index refused them and the mobile board's keyed list halted on the
// duplicate. These pin the fix at both ends: the move merges, and an
// already-broken vault heals on load.

import (
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/model"
)

func TestMoveCardToCategoryMergesIntoExistingPin(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	r.CreateProject("b", "v1", "P")
	catA, _ := r.CreateCategory("b", "v1", "p", "Ideas", 0)
	catB, _ := r.CreateCategory("b", "v1", "p", "Short Stories", 1)
	card, _ := r.CreateCard("brainstorm", "Brain Bug")
	if err := r.PinCard(card.ID, catB.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.PinCard(card.ID, catA.ID); err != nil { // the AI's suggest_pin
		t.Fatal(err)
	}

	// Dragged from Ideas into Short Stories, where it already is.
	if err := r.MoveCardToCategory(card.ID, catA.ID, catB.ID, 3); err != nil {
		t.Fatalf("MoveCardToCategory: %v", err)
	}
	pins, _ := r.GetCardPins(card.ID)
	if len(pins) != 1 || pins[0].CategoryID != catB.ID || pins[0].Position != 3 {
		t.Fatalf("expected one pin in the destination at position 3, got %+v", pins)
	}

	// Same-category "move" is a reposition, never a second pin.
	if err := r.MoveCardToCategory(card.ID, catB.ID, catB.ID, 0); err != nil {
		t.Fatalf("same-category move: %v", err)
	}
	pins, _ = r.GetCardPins(card.ID)
	if len(pins) != 1 || pins[0].Position != 0 {
		t.Fatalf("same-category move must reposition the single pin, got %+v", pins)
	}
}

func TestRevalidateCollapsesDuplicatePins(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("B")
	r.CreateStream("b", "v1")
	r.CreateProject("b", "v1", "P")
	cat, _ := r.CreateCategory("b", "v1", "p", "Short Stories", 0)
	card, _ := r.CreateCard("brainstorm", "Brain Bug")
	if err := r.PinCard(card.ID, cat.ID); err != nil {
		t.Fatal(err)
	}
	// Splice in the duplicate an older build wrote.
	pf, _ := r.loadPinFile(card.ID)
	dup := pf.Pins[0]
	dup.Position = 1
	pf.Pins = append(pf.Pins, dup)
	if err := r.savePinFile(pf); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.pinsDirPath(card.ID), "pins.json")); err != nil {
		t.Fatal(err)
	}

	stats, err := r.Revalidate()
	if err != nil {
		t.Fatal(err)
	}
	if stats.DuplicatePinsRemoved != 1 {
		t.Errorf("DuplicatePinsRemoved = %d, want 1 (%s)", stats.DuplicatePinsRemoved, stats)
	}
	pins, _ := r.GetCardPins(card.ID)
	if len(pins) != 1 {
		t.Fatalf("expected the duplicate collapsed to one pin, got %+v", pins)
	}
	inCat, _ := r.ListCardsInCategory(cat.ID)
	if len(inCat) != 1 {
		t.Errorf("board should list the card once, got %d", len(inCat))
	}
	var _ model.Pin = pins[0]
}
