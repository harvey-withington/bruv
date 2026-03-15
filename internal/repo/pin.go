package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"time"
)

// PinCard pins a Card to a specific Project/Category.
func (r *Repository) PinCard(cardID, projectID, categoryID string) error {
	// Verify card exists
	if _, err := r.GetCard(cardID); err != nil {
		return err
	}

	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	// Check for duplicate pin
	for _, p := range pinFile.Pins {
		if p.ProjectID == projectID && p.CategoryID == categoryID {
			return fmt.Errorf("card %q is already pinned to project %q / category %q", cardID, projectID, categoryID)
		}
	}

	pin := model.Pin{
		CardID:     cardID,
		ProjectID:  projectID,
		CategoryID: categoryID,
		Position:   len(pinFile.Pins),
		PinnedAt:   time.Now().UTC(),
	}
	pinFile.Pins = append(pinFile.Pins, pin)

	return r.savePinFile(pinFile)
}

// UnpinCard removes a Card's pin from a specific Project/Category.
func (r *Repository) UnpinCard(cardID, projectID, categoryID string) error {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	found := false
	filtered := make([]model.Pin, 0, len(pinFile.Pins))
	for _, p := range pinFile.Pins {
		if p.ProjectID == projectID && p.CategoryID == categoryID {
			found = true
			continue
		}
		filtered = append(filtered, p)
	}

	if !found {
		return fmt.Errorf("card %q is not pinned to project %q / category %q", cardID, projectID, categoryID)
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

// ListCardsInCategory returns all card IDs pinned to a specific project/category.
// This is a scan operation — the SQLite index will make this fast in Phase 2.
func (r *Repository) ListCardsInCategory(projectID, categoryID string) ([]model.Pin, error) {
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
			if p.ProjectID == projectID && p.CategoryID == categoryID {
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
func (r *Repository) MoveCardInCategory(cardID, projectID, categoryID string, newPosition int) error {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	found := false
	for i := range pinFile.Pins {
		if pinFile.Pins[i].ProjectID == projectID && pinFile.Pins[i].CategoryID == categoryID {
			pinFile.Pins[i].Position = newPosition
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("card %q is not pinned to project %q / category %q", cardID, projectID, categoryID)
	}

	return r.savePinFile(pinFile)
}

// MoveCardToCategory moves a card from one category to another within the same project.
func (r *Repository) MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID string, newPosition int) error {
	pinFile, err := r.loadPinFile(cardID)
	if err != nil {
		return err
	}

	found := false
	for i := range pinFile.Pins {
		if pinFile.Pins[i].ProjectID == projectID && pinFile.Pins[i].CategoryID == fromCategoryID {
			pinFile.Pins[i].CategoryID = toCategoryID
			pinFile.Pins[i].Position = newPosition
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("card %q is not pinned to project %q / category %q", cardID, projectID, fromCategoryID)
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
