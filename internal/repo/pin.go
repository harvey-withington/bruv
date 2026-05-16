package repo

import (
	"bruv/internal/model"
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
func (r *Repository) PinCard(cardID, categoryID string) error {
	// Verify card exists
	if _, err := r.GetCard(cardID); err != nil {
		return err
	}

	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	// Check for duplicate pin (category-keyed; same card can't be pinned
	// twice to the same category).
	for _, p := range pinFile.Pins {
		if p.CategoryID == categoryID {
			return fmt.Errorf("card %q is already pinned to category %q", cardID, categoryID)
		}
	}

	pin := model.Pin{
		CardID:     cardID,
		ProjectID:  categoryID, // see doc comment — kept = CategoryID
		CategoryID: categoryID,
		Position:   len(pinFile.Pins),
		PinnedAt:   time.Now().UTC(),
	}
	pinFile.Pins = append(pinFile.Pins, pin)

	return r.savePinFile(pinFile)
}

// PinCardAt pins a Card to a specific Category with an explicit position.
func (r *Repository) PinCardAt(cardID, categoryID string, position int) error {
	if _, err := r.GetCard(cardID); err != nil {
		return err
	}

	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	for _, p := range pinFile.Pins {
		if p.CategoryID == categoryID {
			return fmt.Errorf("card %q is already pinned to category %q", cardID, categoryID)
		}
	}

	pin := model.Pin{
		CardID:     cardID,
		ProjectID:  categoryID,
		CategoryID: categoryID,
		Position:   position,
		PinnedAt:   time.Now().UTC(),
	}
	pinFile.Pins = append(pinFile.Pins, pin)

	return r.savePinFile(pinFile)
}

// UnpinCard removes a Card's pin from a specific Category. Matches on
// CategoryID alone — any pin record with that CategoryID is removed,
// regardless of what its (now-vestigial) ProjectID field happens to be.
// This means stale pins from older buggy writes get cleaned up too.
func (r *Repository) UnpinCard(cardID, categoryID string) error {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

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

	// If no pins remain, remove the pin file and directory
	if len(pinFile.Pins) == 0 {
		pinsDir := r.pinsDirPath(cardID)
		if fileExists(pinsDir) {
			return os.RemoveAll(pinsDir)
		}
		return nil
	}

	return r.savePinFile(pinFile)
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
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	found := false
	for i := range pinFile.Pins {
		if pinFile.Pins[i].CategoryID == categoryID {
			pinFile.Pins[i].Position = newPosition
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("card %q is not pinned to category %q", cardID, categoryID)
	}

	return r.savePinFile(pinFile)
}

// MoveCardToCategory moves a card from one category to another. Both
// IDs are categories; ProjectID stored on the pin is set = CategoryID
// per the doc comment on PinCard.
func (r *Repository) MoveCardToCategory(cardID, fromCategoryID, toCategoryID string, newPosition int) error {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	found := false
	for i := range pinFile.Pins {
		if pinFile.Pins[i].CategoryID == fromCategoryID {
			pinFile.Pins[i].ProjectID = toCategoryID
			pinFile.Pins[i].CategoryID = toCategoryID
			pinFile.Pins[i].Position = newPosition
			found = true
			break
		}
	}

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

func (r *Repository) savePinFile(pf *model.PinFile) error {
	dir := r.pinsDirPath(pf.CardID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create pin directory: %w", err)
	}
	return writeJSON(r.pinsFilePath(pf.CardID), pf)
}
