package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// commentsFilePath returns the on-disk path for a card's comments file,
// mirroring the sibling-file pattern used by chat and agent state.
func (r *Repository) commentsFilePath(cardID string) string {
	return filepath.Join(r.Root, cardsDir, safeSeg(cardID)+".comments.json")
}

// LoadComments retrieves all comments for a card.
// Returns an empty CommentFile if no file exists yet.
func (r *Repository) LoadComments(cardID string) (*model.CommentFile, error) {
	path := r.commentsFilePath(cardID)
	if !fileExists(path) {
		return &model.CommentFile{
			CardID:   cardID,
			Comments: []model.Comment{},
		}, nil
	}

	var cf model.CommentFile
	if err := readJSON(path, &cf); err != nil {
		return nil, fmt.Errorf("read comments file for card %q: %w", cardID, err)
	}
	return &cf, nil
}

// saveComments persists the entire comment file to disk.
func (r *Repository) saveComments(cf *model.CommentFile) error {
	return writeJSON(r.commentsFilePath(cf.CardID), cf)
}

// AddCardComment appends a new comment to a card's comment history.
// If createdAt is the zero time, the current time is used; this allows
// importers to preserve original timestamps while interactive adds get "now".
func (r *Repository) AddCardComment(cardID, author, text string, createdAt time.Time) (*model.Comment, error) {
	if _, err := r.GetCard(cardID); err != nil {
		return nil, err
	}
	cf, err := r.LoadComments(cardID)
	if err != nil {
		return nil, err
	}

	ts := createdAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	comment := model.Comment{
		ID:        fmt.Sprintf("cm-%s", uuid.New().String()[:8]),
		Author:    author,
		CreatedAt: ts,
		UpdatedAt: ts,
		Text:      text,
	}
	cf.Comments = append(cf.Comments, comment)

	if err := r.saveComments(cf); err != nil {
		return nil, err
	}
	return &comment, nil
}

// UpdateCardComment edits an existing comment's text. The author is not editable.
func (r *Repository) UpdateCardComment(cardID, commentID, text string) (*model.Comment, error) {
	cf, err := r.LoadComments(cardID)
	if err != nil {
		return nil, err
	}
	for i := range cf.Comments {
		if cf.Comments[i].ID == commentID {
			cf.Comments[i].Text = text
			cf.Comments[i].UpdatedAt = time.Now().UTC()
			if err := r.saveComments(cf); err != nil {
				return nil, err
			}
			updated := cf.Comments[i]
			return &updated, nil
		}
	}
	return nil, fmt.Errorf("comment %q not found on card %q", commentID, cardID)
}

// DeleteCardComment removes a single comment by ID.
func (r *Repository) DeleteCardComment(cardID, commentID string) error {
	cf, err := r.LoadComments(cardID)
	if err != nil {
		return err
	}
	filtered := make([]model.Comment, 0, len(cf.Comments))
	found := false
	for _, c := range cf.Comments {
		if c.ID == commentID {
			found = true
			continue
		}
		filtered = append(filtered, c)
	}
	if !found {
		return fmt.Errorf("comment %q not found on card %q", commentID, cardID)
	}
	cf.Comments = filtered
	return r.saveComments(cf)
}

// DeleteCommentsFile removes a card's entire comment history from disk.
// No error if the file doesn't exist — used during card deletion.
func (r *Repository) DeleteCommentsFile(cardID string) error {
	err := os.Remove(r.commentsFilePath(cardID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete comments file for card %q: %w", cardID, err)
	}
	return nil
}
