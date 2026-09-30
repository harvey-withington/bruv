package repo

import (
	"bruv/internal/model"
	"errors"
	"fmt"
	"os"
	"time"
)

// PinCard pins a Card to a specific Category.
//
// The Pin record on disk also carries a ProjectID field — historically
// kept as a separate composite-key element, in practice always equal to
// CategoryID across every production caller. The field is preserved for
// on-disk format compatibility (older vaults still have it set), but
// the lookup APIs below key purely on CategoryID. New writes set
// ProjectID = CategoryID for consistency.
//
// The new pin goes to the end of the category: one past the highest
// position any card holds there (0 for an empty category).
func (r *Repository) PinCard(cardID, categoryID string) error {
	existing, err := r.ListCardsInCategory(categoryID)
	if err != nil {
		return err
	}
	position := 0
	for _, p := range existing {
		if p.Position >= position {
			position = p.Position + 1
		}
	}
	return r.PinCardAt(cardID, categoryID, position)
}

// PinCardAt pins a Card to a specific Category with an explicit position.
func (r *Repository) PinCardAt(cardID, categoryID string, position int) error {
	if _, err := r.GetCard(cardID); err != nil {
		return err
	}

	return r.mutatePinFile(cardID, func(pinFile *model.PinFile) error {
		// Check for duplicate pin (category-keyed; same card can't be
		// pinned twice to the same category).
		for _, p := range pinFile.Pins {
			if p.CategoryID == categoryID {
				return fmt.Errorf("card %q is already pinned to category %q", cardID, categoryID)
			}
		}

		pinFile.Pins = append(pinFile.Pins, model.Pin{
			CardID:     cardID,
			ProjectID:  categoryID, // see PinCard doc comment — kept = CategoryID
			CategoryID: categoryID,
			Position:   position,
			PinnedAt:   time.Now().UTC(),
		})
		return nil
	})
}

// UnpinCard removes a Card's pin from a specific Category. Matches on
// CategoryID alone — any pin record with that CategoryID is removed,
// regardless of what its (now-vestigial) ProjectID field happens to be.
// This means stale pins from older buggy writes get cleaned up too.
func (r *Repository) UnpinCard(cardID, categoryID string) error {
	return r.mutatePinFile(cardID, func(pinFile *model.PinFile) error {
		found := false
		filtered := make([]model.Pin, 0, len(pinFile.Pins))
		for _, p := range pinFile.Pins {
			if p.CategoryID == categoryID {
				found = true
				continue
			}
			filtered = append(filtered, p)
		}

		if !found {
			return fmt.Errorf("card %q is not pinned to category %q", cardID, categoryID)
		}
		pinFile.Pins = filtered
		return nil
	})
}

// GetCardPins returns all pins for a Card.
func (r *Repository) GetCardPins(cardID string) ([]model.Pin, error) {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return nil, err
	}
	return pinFile.Pins, nil
}

// ListCardsInCategory returns all pin records for the given category.
// Keys on CategoryID alone — pins written by older buggy code paths
// where ProjectID held a real project ID instead of the category ID
// will still be discovered.
//
// This is a scan operation — the SQLite index makes the equivalent
// query fast in core/services/search.
func (r *Repository) ListCardsInCategory(categoryID string) ([]model.Pin, error) {
	cardDirs, err := listSubdirs(r.pinsBasePath())
	if err != nil {
		return nil, fmt.Errorf("list pin directories: %w", err)
	}

	var matched []model.Pin
	for _, cardID := range cardDirs {
		pinFile, err := r.loadPinFile(cardID)
		if err != nil {
			continue
		}
		for _, p := range pinFile.Pins {
			if p.CategoryID == categoryID {
				matched = append(matched, p)
			}
		}
	}

	// Sort by position
	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].Position < matched[i].Position {
				matched[i], matched[j] = matched[j], matched[i]
			}
		}
	}

	return matched, nil
}

// MoveCardInCategory updates a card's position within a category.
func (r *Repository) MoveCardInCategory(cardID, categoryID string, newPosition int) error {
	return r.mutatePinFile(cardID, func(pinFile *model.PinFile) error {
		for i := range pinFile.Pins {
			if pinFile.Pins[i].CategoryID == categoryID {
				pinFile.Pins[i].Position = newPosition
				return nil
			}
		}
		return fmt.Errorf("card %q is not pinned to category %q", cardID, categoryID)
	})
}

// MoveCardToCategory moves a card from one category to another. Both
// IDs are categories; ProjectID stored on the pin is set = CategoryID
// per the doc comment on PinCard.
func (r *Repository) MoveCardToCategory(cardID, fromCategoryID, toCategoryID string, newPosition int) error {
	unlock := lockPath(r.pinsFilePath(cardID))
	defer unlock()

	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	// A card already pinned to the destination (pinned there by the AI's
	// suggest_pin, say, then dragged across on a board that hadn't caught
	// up) must end up there ONCE: the source pin is dropped and the
	// existing destination pin takes the new position. Rewriting the
	// source pin in place — what this did before 2026-09-17 — produced two
	// pins to the same category, which the index's unique key refused and
	// the boards rendered twice (a keyed each with a duplicate key halts
	// the mobile page).
	alreadyThere := -1
	for i := range pinFile.Pins {
		if pinFile.Pins[i].CategoryID == toCategoryID {
			alreadyThere = i
			break
		}
	}

	found := false
	kept := pinFile.Pins[:0]
	for i := range pinFile.Pins {
		p := pinFile.Pins[i]
		if p.CategoryID == fromCategoryID && fromCategoryID != toCategoryID {
			found = true
			if alreadyThere >= 0 {
				continue // destination pin exists — this one goes
			}
			p.ProjectID = toCategoryID
			p.CategoryID = toCategoryID
			p.Position = newPosition
		} else if i == alreadyThere {
			found = found || fromCategoryID == toCategoryID
			p.Position = newPosition
		}
		kept = append(kept, p)
	}
	pinFile.Pins = kept

	if !found {
		return fmt.Errorf("card %q is not pinned to category %q", cardID, fromCategoryID)
	}

	return r.savePinFile(pinFile)
}

// Internal helpers

func (r *Repository) pinsBasePath() string {
	return r.Root + "/pins"
}

func (r *Repository) loadPinFile(cardID string) (*model.PinFile, error) {
	path := r.pinsFilePath(cardID)
	if !fileExists(path) {
		return &model.PinFile{
			CardID: cardID,
			Pins:   []model.Pin{},
		}, nil
	}

	var pf model.PinFile
	if err := readJSON(path, &pf); err != nil {
		return nil, fmt.Errorf("read pin file for card %q: %w", cardID, err)
	}
	return &pf, nil
}

// mutatePinFile runs fn on a fresh read of the card's pin file under its
// lock and saves the result; a pin file left with no pins is removed
// along with its directory. ErrNoChange from fn skips the write (nil
// error); any other error aborts without writing.
func (r *Repository) mutatePinFile(cardID string, fn func(pf *model.PinFile) error) error {
	unlock := lockPath(r.pinsFilePath(cardID))
	defer unlock()

	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}
	if err := fn(pinFile); err != nil {
		if errors.Is(err, ErrNoChange) {
			return nil
		}
		return err
	}
	if len(pinFile.Pins) == 0 {
		pinsDir := r.pinsDirPath(cardID)
		if fileExists(pinsDir) {
			return os.RemoveAll(pinsDir)
		}
		return nil
	}
	return r.savePinFile(pinFile)
}

// savePinFile writes a pin file. Callers hold lockPath(pinsFilePath)
// across their load → save (see mutatePinFile).
func (r *Repository) savePinFile(pf *model.PinFile) error {
	dir := r.pinsDirPath(pf.CardID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create pin directory: %w", err)
	}
	return writeJSON(r.pinsFilePath(pf.CardID), pf)
}
