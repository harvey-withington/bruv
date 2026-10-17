package card

// Revision-guarded content edits — the server half of offline cards.
//
// A device that edited a card while offline knows the revision its copy
// was taken at. CommitEdit applies its edit only if the card is still at
// that revision; otherwise nothing is written and the current card comes
// back so the device can merge against it and try again. The check and
// the write share the card's file lock (it runs inside Edit), so nothing
// can land between them.

import (
	"errors"
	"strings"
	"time"

	"bruv/internal/model"
)

var errRevMoved = errors.New("card revision moved")

// CommitEdit applies edit to the card if it is at revision expectedRev.
func (s *Service) CommitEdit(id string, expectedRev int64, edit model.CardEdit) (*model.CardCommitResult, error) {
	var due *time.Time // parsed up front so a bad date fails before the lock
	if edit.DueDate != nil && strings.TrimSpace(*edit.DueDate) != "" {
		t, err := ParseDueDate(*edit.DueDate)
		if err != nil {
			return nil, err
		}
		due = &t
	}
	card, _, err := s.Edit(id, func(c *model.Card) error {
		if c.Rev != expectedRev {
			return errRevMoved
		}
		if edit.Title != nil {
			if strings.TrimSpace(*edit.Title) == "" {
				return errors.New("card title cannot be empty")
			}
			c.Title = *edit.Title
		}
		if edit.Description != nil {
			c.Description = *edit.Description
		}
		if edit.DueDate != nil {
			c.DueDate = due
		}
		if edit.Tags != nil {
			c.Tags = *edit.Tags
		}
		if edit.Blocks != nil {
			c.Blocks = *edit.Blocks
		}
		return nil
	})
	if errors.Is(err, errRevMoved) {
		current, gerr := s.deps.Repo().GetCard(id)
		if gerr != nil {
			return nil, gerr
		}
		return &model.CardCommitResult{Conflict: true, Card: current}, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.CardCommitResult{Card: card}, nil
}
