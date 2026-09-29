package tools

// Setting and adding card fields — the one implementation behind the
// native set_card_fields / add_card_blocks tools (MCP, chat, agents) and
// card chat's set_fields / add_field, which are thin adapters over them.

import (
	"fmt"
	"sort"
	"strings"

	"bruv/internal/model"
)

// FieldUpdate is the outcome of ApplyFieldValues.
type FieldUpdate struct {
	Updated []string // block keys whose value changed, in card order
	Unknown []string // keys the card has no field for (skipped, reported back)
	// Description is set when the caller passed "description" and the card
	// has no block keyed so: it's the card's intrinsic description, not a
	// field, so the caller saves it through the description path.
	Description *string
}

// ApplyFieldValues writes values into the card's blocks by key. A key the
// card lacks but its type's schema defines is added first (schemaBlocks),
// so a model can fill a typed field that hasn't been created yet. Values
// are shaped and validated per block (select options, rating range …): a
// value a block rejects is an error, and the card must then not be saved.
// Keys the card has no field for are skipped and listed in Unknown, so one
// typo doesn't lose the other values.
func ApplyFieldValues(card *model.Card, schemaBlocks []model.Block, fields map[string]any) (FieldUpdate, error) {
	var out FieldUpdate
	values := make(map[string]any, len(fields))
	for k, v := range fields {
		values[k] = v
	}
	if desc, ok := values["description"].(string); ok && !hasBlockKey(card, "description") {
		out.Description = &desc
		delete(values, "description")
	}
	for _, sb := range schemaBlocks {
		if _, want := values[sb.Key]; want && sb.Key != "" && !hasBlockKey(card, sb.Key) {
			card.Blocks = append(card.Blocks, sb)
		}
	}
	for i := range card.Blocks {
		key := card.Blocks[i].Key
		val, ok := values[key]
		if key == "" || !ok {
			continue
		}
		coerced, err := CoerceBlockValueForBlock(&card.Blocks[i], val)
		if err != nil {
			return out, fmt.Errorf("field %q: %w", key, err)
		}
		card.Blocks[i].Value = coerced
		out.Updated = append(out.Updated, key)
		delete(values, key)
	}
	for k := range values {
		out.Unknown = append(out.Unknown, k)
	}
	sort.Strings(out.Unknown)
	if len(out.Updated) == 0 && out.Description == nil {
		return out, fmt.Errorf("no matching field keys; available keys: %s", strings.Join(BlockKeys(card), ", "))
	}
	return out, nil
}

// CheckNewBlockKeys refuses blocks whose key the card (or the batch)
// already has: two blocks with one key make key-based updates ambiguous.
func CheckNewBlockKeys(card *model.Card, blocks []model.Block) error {
	seen := map[string]bool{}
	for _, b := range card.Blocks {
		if b.Key != "" {
			seen[b.Key] = true
		}
	}
	for _, b := range blocks {
		if b.Key == "" {
			continue
		}
		if seen[b.Key] {
			return fmt.Errorf("the card already has a field with key %q — use set_card_fields to change it", b.Key)
		}
		seen[b.Key] = true
	}
	return nil
}

// DefaultBlockValue is the empty value a new block of blockType starts with.
func DefaultBlockValue(blockType string) any {
	switch blockType {
	case model.BlockChecklist:
		return []any{}
	case model.BlockList:
		return coerceList(nil)
	case model.BlockCheckbox:
		return false
	case model.BlockNumber:
		return 0.0
	}
	return ""
}

// BlockKeys lists the card's keyed blocks, in card order.
func BlockKeys(card *model.Card) []string {
	var keys []string
	for _, b := range card.Blocks {
		if b.Key != "" {
			keys = append(keys, b.Key)
		}
	}
	return keys
}

func hasBlockKey(card *model.Card, key string) bool {
	for _, b := range card.Blocks {
		if b.Key == key {
			return true
		}
	}
	return false
}
