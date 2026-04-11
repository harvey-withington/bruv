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
		Checklist:    []model.ChecklistItem{},
		Attachments:  []string{},
		Tags:         []string{},
		Blocks:       []model.Block{},
	}

	if err := writeJSON(r.cardFilePath(card.ID), card); err != nil {
		return nil, fmt.Errorf("write card: %w", err)
	}

	return card, nil
}

// GetCard reads a Card by its ID.
// Legacy cards with Fields/Checklist/Attachments are automatically migrated to Blocks on read.
func (r *Repository) GetCard(id string) (*model.Card, error) {
	path := r.cardFilePath(id)
	if !fileExists(path) {
		return nil, fmt.Errorf("card %q not found", id)
	}

	var card model.Card
	if err := readJSON(path, &card); err != nil {
		return nil, err
	}

	// Auto-migrate legacy cards to block model
	model.MigrateCardToBlocks(&card, nil)

	// Migrate removed block types (number, date, checkbox, select, radio, image, video) → text/url
	legacyMigrated := model.MigrateLegacyBlockTypes(&card)

	// Backfill empty block keys from labels (manual UI-added blocks)
	keysBackfilled := model.BackfillBlockKeys(&card)

	if legacyMigrated || keysBackfilled {
		_ = writeJSON(path, &card)
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

// AddChecklistItem adds a new checklist item to a Card.
func (r *Repository) AddChecklistItem(cardID, text string) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		item := model.ChecklistItem{
			ID:   fmt.Sprintf("ck-%s", uuid.New().String()[:8]),
			Text: text,
			Done: false,
		}
		card.Checklist = append(card.Checklist, item)
	})
}

// ToggleChecklistItem toggles the done state of a checklist item.
func (r *Repository) ToggleChecklistItem(cardID, itemID string) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		for i := range card.Checklist {
			if card.Checklist[i].ID == itemID {
				card.Checklist[i].Done = !card.Checklist[i].Done
				return
			}
		}
	})
}

// RemoveChecklistItem removes a checklist item from a Card.
func (r *Repository) RemoveChecklistItem(cardID, itemID string) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		for i, item := range card.Checklist {
			if item.ID == itemID {
				card.Checklist = append(card.Checklist[:i], card.Checklist[i+1:]...)
				return
			}
		}
	})
}

// PromoteChecklistItem removes a checklist item from its parent card and creates
// a new Card from it. The parent card retains a reference via a tag.
func (r *Repository) PromoteChecklistItem(cardID, itemID, targetType string) (*model.Card, error) {
	// Find the checklist item text
	parentCard, err := r.GetCard(cardID)
	if err != nil {
		return nil, err
	}

	var itemText string
	found := false
	for _, item := range parentCard.Checklist {
		if item.ID == itemID {
			itemText = item.Text
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("checklist item %q not found in card %q", itemID, cardID)
	}

	// Create the new card
	newCard, err := r.CreateCard(targetType, itemText)
	if err != nil {
		return nil, fmt.Errorf("create promoted card: %w", err)
	}

	// Add a reference field linking back to the parent
	newCard.Fields["promoted_from"] = cardID

	if err := writeJSON(r.cardFilePath(newCard.ID), newCard); err != nil {
		return nil, fmt.Errorf("write promoted card: %w", err)
	}

	// Remove the checklist item from the parent
	if _, err := r.RemoveChecklistItem(cardID, itemID); err != nil {
		return nil, fmt.Errorf("remove checklist item from parent: %w", err)
	}

	return newCard, nil
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

// UpdateCardBlocks replaces a card's blocks and syncs legacy fields for backward
// compatibility (Fields["description"], Checklist).
func (r *Repository) UpdateCardBlocks(cardID string, blocks []model.Block) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		card.Blocks = blocks

		// Sync legacy Fields from text blocks with keys
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
			case model.BlockNumber:
				card.Fields[b.Key] = b.Value
			case model.BlockCheckbox:
				card.Fields[b.Key] = b.Value
			case model.BlockSelect, model.BlockRadio:
				card.Fields[b.Key] = b.Value
			case model.BlockDate:
				card.Fields[b.Key] = b.Value
			}
		}

		// Sync legacy Checklist from checklist blocks
		var checklist []model.ChecklistItem
		for _, b := range blocks {
			if b.Type != model.BlockChecklist {
				continue
			}
			items, ok := b.Value.([]any)
			if !ok {
				continue
			}
			for _, raw := range items {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				id, _ := m["id"].(string)
				text, _ := m["text"].(string)
				done, _ := m["done"].(bool)
				if id != "" && text != "" {
					checklist = append(checklist, model.ChecklistItem{ID: id, Text: text, Done: done})
				}
			}
		}
		card.Checklist = checklist
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
// data is the base64-encoded file content.
func (r *Repository) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	// Create attachments directory if needed
	dir := filepath.Join(r.Root, "attachments", cardID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create attachments dir: %w", err)
	}

	// Decode base64 data
	decoded, err := base64Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode attachment: %w", err)
	}

	id := fmt.Sprintf("att-%s", uuid.New().String()[:8])
	filePath := filepath.Join(dir, id+"_"+name)
	if err := os.WriteFile(filePath, decoded, 0o644); err != nil {
		return nil, fmt.Errorf("write attachment file: %w", err)
	}

	return r.UpdateCard(cardID, func(card *model.Card) {
		card.FileAttachments = append(card.FileAttachments, model.FileAttachment{
			ID:      id,
			Name:    name,
			Path:    filePath,
			Mime:    detectMime(name),
			Size:    int64(len(decoded)),
			AddedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})
}

// RemoveCardAttachment removes a file attachment from a card.
func (r *Repository) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	return r.UpdateCard(cardID, func(card *model.Card) {
		for i, att := range card.FileAttachments {
			if att.ID == attachmentID {
				// Remove the file from disk (best-effort)
				_ = os.Remove(att.Path)
				card.FileAttachments = append(card.FileAttachments[:i], card.FileAttachments[i+1:]...)
				return
			}
		}
	})
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
