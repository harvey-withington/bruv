package agent

// Shared body of the update_self and update_card built-ins: apply an
// LLM's intrinsic-field and block updates to one card. update_self
// targets the agent's own card; update_card targets any card by id, so
// an agent can keep the cards it files (a race card's start time,
// barrier, jockey) current instead of only reporting changes.

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"bruv/core/runtime/tools"
	"bruv/internal/model"
)

// updateCard applies title / due_date / tags / updates[] from a tool
// call's arguments to the card and saves it. A block value that breaks
// the block's constraints (e.g. a select option that doesn't exist) is
// returned as an error so the model can retry.
func (rt *Runtime) updateCard(cardID string, args map[string]any) error {
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		return err
	}
	// Optional top-level intrinsic field updates
	if newTitle, ok := args["title"].(string); ok && newTitle != "" {
		card.Title = newTitle
	}
	if newDueDate, ok := args["due_date"].(string); ok && newDueDate != "" {
		if parsed, ok := parseDueDate(newDueDate); ok {
			card.DueDate = &parsed
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
		rawValue := upd["value"]
		if key == "" {
			continue
		}
		if applyIntrinsicUpdate(card, key, rawValue) {
			continue
		}
		if err := setOrAddBlock(card, key, rawValue); err != nil {
			return err
		}
	}
	card.UpdatedAt = time.Now().UTC()
	if err := rt.deps.Repo().UpdateCardDirect(cardID, card); err != nil {
		return err
	}
	if rt.deps.Index() != nil {
		rt.idxIncrementalRefresh()
	}
	// Notify any open card detail view so it re-fetches the new content.
	rt.emitCardUpdated(cardID)
	return nil
}

// applyIntrinsicUpdate handles intrinsic fields that LLMs often put in
// the updates array instead of using the top-level parameters, so the
// real card field changes rather than a spurious text block appearing —
// but only when no real block with that key/label exists (a user-created
// "Tags" block wins). Reports whether the update was consumed.
func applyIntrinsicUpdate(card *model.Card, key string, rawValue any) bool {
	switch {
	case strings.EqualFold(key, "title") && !cardHasBlock(card, key):
		if s, ok := rawValue.(string); ok && s != "" {
			card.Title = s
		}
		return true
	case strings.EqualFold(key, "description"):
		// Description is intrinsic on the card — never a block.
		if s, ok := rawValue.(string); ok {
			card.Description = s
		} else if rawValue != nil {
			card.Description = fmt.Sprintf("%v", rawValue)
		}
		return true
	case (strings.EqualFold(key, "due_date") || strings.EqualFold(key, "due date") || strings.EqualFold(key, "duedate")) && !cardHasBlock(card, key):
		if s, ok := rawValue.(string); ok && s != "" {
			if parsed, ok := parseDueDate(s); ok {
				card.DueDate = &parsed
			}
		}
		return true
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
		return true
	}
	return false
}

// setOrAddBlock writes a value into the block matched by key, then by
// label (case-insensitive). tools.CoerceBlockValueForBlock reshapes the
// input to the block's type and applies meta-aware constraints (select
// options, rating/progress clamping, date vs date-time format). With no
// match a new text block is added — update never guesses at other types.
func setOrAddBlock(card *model.Card, key string, rawValue any) error {
	for i, b := range card.Blocks {
		if b.Key == key || strings.EqualFold(b.Label, key) {
			coerced, err := tools.CoerceBlockValueForBlock(&card.Blocks[i], rawValue)
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

// parseDueDate accepts YYYY-MM-DD or RFC 3339.
func parseDueDate(s string) (time.Time, bool) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
