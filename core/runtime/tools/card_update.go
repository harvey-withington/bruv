package tools

// ApplyCardUpdates is the one implementation of "update a card from an
// LLM's arguments" (title / due_date / tags / updates[]). The agent's
// update_self and the native update_card tool (core/boardtools) both use
// it; each surface saves the result its own way.

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	cardsvc "bruv/core/services/card"
	"bruv/internal/model"
)

// ApplyCardUpdates applies intrinsic-field and block updates to card in
// place. A block value that breaks the block's constraints (e.g. a
// select option that doesn't exist) or a due date that isn't a date is
// returned as an error so the model can retry; card may then be partly
// updated and must not be saved.
func ApplyCardUpdates(card *model.Card, args map[string]any) error {
	if newTitle, ok := args["title"].(string); ok && newTitle != "" {
		card.Title = newTitle
	}
	if newDueDate, ok := args["due_date"].(string); ok && strings.TrimSpace(newDueDate) != "" {
		if err := setDueDate(card, newDueDate); err != nil {
			return err
		}
	}
	if newTags, ok := args["tags"].([]any); ok {
		tags := make([]string, 0, len(newTags))
		for _, t := range newTags {
			if s, ok := t.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
		if len(tags) > 0 {
			card.Tags = tags
		}
	}
	updates, _ := args["updates"].([]any)
	for _, u := range updates {
		upd, ok := u.(map[string]any)
		if !ok {
			continue
		}
		key, _ := upd["key"].(string)
		if key == "" {
			continue
		}
		consumed, err := applyIntrinsicUpdate(card, key, upd["value"])
		if err != nil {
			return err
		}
		if consumed {
			continue
		}
		if err := setOrAddBlock(card, key, upd["value"]); err != nil {
			return err
		}
	}
	return nil
}

func cardHasBlock(card *model.Card, name string) bool {
	for _, b := range card.Blocks {
		if strings.EqualFold(b.Key, name) || strings.EqualFold(b.Label, name) {
			return true
		}
	}
	return false
}

// applyIntrinsicUpdate handles intrinsic fields that LLMs often put in
// the updates array instead of using the top-level parameters, so the
// real card field changes rather than a spurious text block appearing —
// but only when no real block with that key/label exists (a user-created
// "Tags" block wins). Reports whether the update was consumed; a due
// date that isn't a date is an error.
func applyIntrinsicUpdate(card *model.Card, key string, rawValue any) (bool, error) {
	switch {
	case strings.EqualFold(key, "title") && !cardHasBlock(card, key):
		if s, ok := rawValue.(string); ok && s != "" {
			card.Title = s
		}
		return true, nil
	case strings.EqualFold(key, "description"):
		// Description is intrinsic on the card — never a block.
		if s, ok := rawValue.(string); ok {
			card.Description = s
		} else if rawValue != nil {
			card.Description = fmt.Sprintf("%v", rawValue)
		}
		return true, nil
	case (strings.EqualFold(key, "due_date") || strings.EqualFold(key, "due date") || strings.EqualFold(key, "duedate")) && !cardHasBlock(card, key):
		if s, ok := rawValue.(string); ok && strings.TrimSpace(s) != "" {
			if err := setDueDate(card, s); err != nil {
				return true, err
			}
		}
		return true, nil
	case strings.EqualFold(key, "tags") && !cardHasBlock(card, key):
		switch v := rawValue.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					card.Tags = append(card.Tags, s)
				}
			}
		case string:
			if v != "" {
				card.Tags = append(card.Tags, v)
			}
		}
		return true, nil
	}
	return false, nil
}

// setOrAddBlock writes a value into the block matched by key, then by
// label (case-insensitive). tools.CoerceBlockValueForBlock reshapes the
// input to the block's type and applies meta-aware constraints (select
// options, rating/progress clamping, date vs date-time format). With no
// match a new text block is added — update never guesses at other types.
func setOrAddBlock(card *model.Card, key string, rawValue any) error {
	for i, b := range card.Blocks {
		if b.Key == key || strings.EqualFold(b.Label, key) {
			coerced, err := CoerceBlockValueForBlock(&card.Blocks[i], rawValue)
			if err != nil {
				slog.Warn("agent card update coerce failed", "block_key", key, "block_type", b.Type, "err", err)
				return fmt.Errorf("block %q: %w", key, err)
			}
			card.Blocks[i].Value = coerced
			return nil
		}
	}
	strValue := ""
	if s, ok := rawValue.(string); ok {
		strValue = s
	} else if rawValue != nil {
		strValue = fmt.Sprintf("%v", rawValue)
	}
	card.Blocks = append(card.Blocks, model.Block{
		ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
		Type:  model.BlockText,
		Label: key,
		Key:   strings.ToLower(strings.ReplaceAll(key, " ", "_")),
		Value: strValue,
	})
	return nil
}

// setDueDate parses an LLM-supplied due date (card.ParseDueDate: RFC
// 3339, YYYY-MM-DD, or a zone-less local date-time) onto card.
func setDueDate(card *model.Card, s string) error {
	parsed, err := cardsvc.ParseDueDate(s)
	if err != nil {
		return err
	}
	card.DueDate = &parsed
	return nil
}
