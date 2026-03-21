package model

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// MigrateCardToBlocks populates card.Blocks from the legacy Fields, Checklist,
// and Attachments if Blocks is empty. This is idempotent: if Blocks already has
// entries, the function is a no-op. Legacy fields are preserved so the frontend
// continues to work during the transition period.
func MigrateCardToBlocks(card *Card, fieldHints map[string]FieldHint) {
	if len(card.Blocks) > 0 {
		return
	}

	hasLegacy := len(card.Fields) > 0 || len(card.Checklist) > 0 || len(card.Attachments) > 0
	if !hasLegacy {
		return
	}

	var blocks []Block

	// Migrate Fields → Blocks
	for key, val := range card.Fields {
		hint, ok := fieldHints[key]
		if !ok {
			hint = inferFieldHint(key, val)
		}
		blocks = append(blocks, Block{
			ID:       blockID(),
			Type:     hint.BlockType,
			Label:    hint.Label,
			Key:      key,
			Value:    val,
			Required: hint.Required,
			Meta:     hint.Meta,
		})
	}

	// Migrate Checklist → single checklist Block
	if len(card.Checklist) > 0 {
		items := make([]map[string]any, len(card.Checklist))
		for i, item := range card.Checklist {
			items[i] = map[string]any{
				"id":   item.ID,
				"text": item.Text,
				"done": item.Done,
			}
		}
		blocks = append(blocks, Block{
			ID:    blockID(),
			Type:  BlockChecklist,
			Label: "Checklist",
			Value: items,
		})
	}

	// Migrate Attachments → individual url/image Blocks
	for _, att := range card.Attachments {
		btype := BlockURL
		if isImagePath(att) {
			btype = BlockImage
		}
		blocks = append(blocks, Block{
			ID:    blockID(),
			Type:  btype,
			Label: "Attachment",
			Value: att,
		})
	}

	card.Blocks = blocks
}

// FieldHint provides type information for migrating a legacy field to a Block.
// Typically sourced from the card type schema via SchemaToFieldHints().
type FieldHint struct {
	BlockType string
	Label     string
	Required  bool
	Meta      map[string]any
}

// inferFieldHint guesses block type from a field's key name and value when
// no schema hint is available.
func inferFieldHint(key string, val any) FieldHint {
	label := humanizeKey(key)

	switch val.(type) {
	case bool:
		return FieldHint{BlockType: BlockCheckbox, Label: label}
	case float64, int, int64:
		return FieldHint{BlockType: BlockNumber, Label: label}
	default:
		return FieldHint{BlockType: BlockText, Label: label}
	}
}

// humanizeKey converts "recording_status" → "Recording Status".
func humanizeKey(key string) string {
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// blockID generates a short unique ID for a block.
func blockID() string {
	return fmt.Sprintf("blk-%s", uuid.New().String()[:8])
}

// isImagePath returns true if the path looks like an image file.
func isImagePath(p string) bool {
	lower := strings.ToLower(p)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".heic"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
