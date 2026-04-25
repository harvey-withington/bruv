package repo

import (
	"bruv/internal/model"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreateCard creates a new Card in the flat cards/ directory.
func (r *Repository) CreateCard(cardType, title string) (*model.Card, error) {
	if title == "" {
		return nil, fmt.Errorf("card title cannot be empty")
	}

	now := time.Now().UTC()
	card := &model.Card{
		ID:           uuid.New().String(),
		Type:         cardType,
		Title:        title,
		CreatedAt:    now,
		UpdatedAt:    now,
		ContextLevel: model.ContextProject,
		Fields:       make(map[string]any),
		Tags:         []string{},
		Blocks:       []model.Block{},
	}

	if err := writeJSON(r.cardFilePath(card.ID), card); err != nil {
		return nil, fmt.Errorf("write card: %w", err)
	}

	return card, nil
}

// GetCard reads a Card by its ID.
func (r *Repository) GetCard(id string) (*model.Card, error) {
	path := r.cardFilePath(id)
	if !fileExists(path) {
		return nil, fmt.Errorf("card %q not found", id)
	}

	var card model.Card
	if err := readJSON(path, &card); err != nil {
		return nil, err
	}
	return &card, nil
}

// ListCards returns all Cards in the repository.
func (r *Repository) ListCards() ([]model.Card, error) {
	ids, err := listJSONFiles(r.cardsPath())
	if err != nil {
		return nil, fmt.Errorf("list card files: %w", err)
	}

	cards := make([]model.Card, 0, len(ids))
	for _, id := range ids {
		card, err := r.GetCard(id)
		if err != nil {
			continue
		}
		cards = append(cards, *card)
	}
	return cards, nil
}

// UpdateCard updates a Card using an update function.
func (r *Repository) UpdateCard(id string, update func(*model.Card)) (*model.Card, error) {
	card, err := r.GetCard(id)
	if err != nil {
		return nil, err
	}

	update(card)
	card.UpdatedAt = time.Now().UTC()

	if err := writeJSON(r.cardFilePath(id), card); err != nil {
		return nil, fmt.Errorf("write card: %w", err)
	}
	return card, nil
}

// UpdateCardDirect writes a pre-modified card directly to disk.
func (r *Repository) UpdateCardDirect(id string, card *model.Card) error {
	return writeJSON(r.cardFilePath(id), card)
}

// DeleteCard removes a Card and its associated pins.
func (r *Repository) DeleteCard(id string) error {
	cardPath := r.cardFilePath(id)
	if !fileExists(cardPath) {
		return fmt.Errorf("card %q not found", id)
	}

	// Remove the card file
	if err := os.Remove(cardPath); err != nil {
		return fmt.Errorf("delete card file: %w", err)
	}

	// Remove pins directory for this card if it exists
	pinsDir := r.pinsDirPath(id)
	if fileExists(pinsDir) {
		if err := os.RemoveAll(pinsDir); err != nil {
			return fmt.Errorf("delete card pins: %w", err)
		}
	}

	// Chat history is no longer stored in the repo — it lives in the
	// OS config folder keyed by repo ID so it stays personal when the
	// repo is shared. The app layer is responsible for calling
	// config.DeleteChatFor(repoID, id) alongside card deletion.

	// Remove comments file if it exists
	_ = os.Remove(r.commentsFilePath(id))

	// Remove agent file if it exists
	_ = os.Remove(r.agentFilePath(id))

	return nil
}

// DuplicateCard creates a copy of an existing card with a new ID.
func (r *Repository) DuplicateCard(srcCardID string) (*model.Card, error) {
	src, err := r.GetCard(srcCardID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	newCard := *src
	newCard.ID = uuid.New().String()
	newCard.CreatedAt = now
	newCard.UpdatedAt = now
	if err := writeJSON(r.cardFilePath(newCard.ID), &newCard); err != nil {
		return nil, fmt.Errorf("write duplicated card: %w", err)
	}
	return &newCard, nil
}

// UpdateCardBlocks replaces a card's blocks and mirrors keyed
// scalar block values into card.Fields. The Fields map is the
// description editor's read path (DescriptionSection consults
// card.fields.description) so each block write keeps it current.
func (r *Repository) UpdateCardBlocks(cardID string, blocks []model.Block) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		card.Blocks = blocks
		if card.Fields == nil {
			card.Fields = make(map[string]any)
		}
		for _, b := range blocks {
			if b.Key == "" {
				continue
			}
			switch b.Type {
			case model.BlockText:
				if s, ok := b.Value.(string); ok {
					card.Fields[b.Key] = s
				}
			case model.BlockNumber, model.BlockCheckbox, model.BlockSelect, model.BlockRadio, model.BlockDate:
				card.Fields[b.Key] = b.Value
			}
		}
	})
}

// ListCardsByType returns all Cards of a specific type.
func (r *Repository) ListCardsByType(cardType string) ([]model.Card, error) {
	allCards, err := r.ListCards()
	if err != nil {
		return nil, err
	}

	var filtered []model.Card
	for _, card := range allCards {
		if card.Type == cardType {
			filtered = append(filtered, card)
		}
	}
	return filtered, nil
}

// AddCardAttachment stores a file and adds it to the card's FileAttachments list.
// data is the base64-encoded file content. Bytes land at
// <repo>/attachments/<cardID>/<id> with no name suffix, so the
// server can resolve cardID+id to a file path at request time
// without re-reading the card metadata.
func (r *Repository) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	dir := filepath.Join(r.Root, "attachments", cardID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create attachments dir: %w", err)
	}

	decoded, err := base64Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode attachment: %w", err)
	}

	id := fmt.Sprintf("att-%s", uuid.New().String())
	if err := os.WriteFile(filepath.Join(dir, id), decoded, 0o644); err != nil {
		return nil, fmt.Errorf("write attachment file: %w", err)
	}

	return r.UpdateCard(cardID, func(card *model.Card) {
		card.FileAttachments = append(card.FileAttachments, model.FileAttachment{
			ID:      id,
			Name:    name,
			Mime:    detectMime(name),
			Size:    int64(len(decoded)),
			AddedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})
}

// RemoveCardAttachment removes a file attachment from a card and
// deletes the underlying file from disk.
func (r *Repository) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		for i, att := range card.FileAttachments {
			if att.ID == attachmentID {
				_ = os.Remove(filepath.Join(r.Root, "attachments", cardID, att.ID))
				card.FileAttachments = append(card.FileAttachments[:i], card.FileAttachments[i+1:]...)
				return
			}
		}
	})
}

// AttachmentPath resolves an attachment's location on disk. Returns
// the path even if the file doesn't exist — caller checks fstat. The
// server's HTTP handler uses this after verifying the request's HMAC.
func (r *Repository) AttachmentPath(cardID, attachmentID string) string {
	return filepath.Join(r.Root, "attachments", cardID, attachmentID)
}

// FindAttachment returns the metadata entry for an attachment ID on a
// card, or nil if no such entry exists. Used by the HTTP handler to
// resolve Mime + Name without leaking the rest of the card.
func (r *Repository) FindAttachment(cardID, attachmentID string) (*model.FileAttachment, error) {
	card, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}
	for i := range card.FileAttachments {
		if card.FileAttachments[i].ID == attachmentID {
			return &card.FileAttachments[i], nil
		}
	}
	return nil, fmt.Errorf("attachment %q not found on card %q", attachmentID, cardID)
}

// base64Decode decodes a base64 string.
func base64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

// detectMime returns a MIME type guess from the file extension.
func detectMime(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".webm"):
		return "video/webm"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain"
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// ListCardFiles returns the raw file paths of all card JSON files.
// Useful for index rebuilding.
func (r *Repository) ListCardFiles() ([]string, error) {
	entries, err := os.ReadDir(r.cardsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" && filepath.Ext(e.Name()) != ".tmp" {
			paths = append(paths, filepath.Join(r.cardsPath(), e.Name()))
		}
	}
	return paths, nil
}
