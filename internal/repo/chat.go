package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
)

// LoadChat retrieves the chat history for a card.
// Returns an empty ChatFile if no chat file exists yet.
func (r *Repository) LoadChat(cardID string) (*model.ChatFile, error) {
	path := r.chatFilePath(cardID)
	if !fileExists(path) {
		return &model.ChatFile{
			CardID:   cardID,
			Messages: []model.ChatMessage{},
		}, nil
	}

	var cf model.ChatFile
	if err := readJSON(path, &cf); err != nil {
		return nil, fmt.Errorf("read chat file for card %q: %w", cardID, err)
	}
	return &cf, nil
}

// SaveChat persists the entire chat file to disk.
func (r *Repository) SaveChat(cf *model.ChatFile) error {
	return writeJSON(r.chatFilePath(cf.CardID), cf)
}

// AppendMessage adds a single message to a card's chat history.
func (r *Repository) AppendMessage(cardID string, msg model.ChatMessage) (*model.ChatFile, error) {
	cf, err := r.LoadChat(cardID)
	if err != nil {
		return nil, err
	}
	cf.Messages = append(cf.Messages, msg)
	if err := r.SaveChat(cf); err != nil {
		return nil, err
	}
	return cf, nil
}

// DeleteChat removes a card's chat file. No error if the file doesn't exist.
func (r *Repository) DeleteChat(cardID string) error {
	err := os.Remove(r.chatFilePath(cardID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete chat file for card %q: %w", cardID, err)
	}
	return nil
}
