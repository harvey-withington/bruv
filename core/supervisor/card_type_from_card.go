package supervisor

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"bruv/internal/model"

	"github.com/google/uuid"
)

// CreateCardTypeFromCard crystallises a card's current layout into a new
// reusable card type. The selected blocks (by ID) become the type's
// template — labels, types and meta (e.g. select options) preserved,
// values stripped — and the originating card is switched to the new type.
//
// keepValueIDs is the subset of blockIDs whose current value should be
// baked into the template instead of blanked — e.g. a checklist whose items
// form a reusable structure. Checklist items kept this way start unchecked.
//
// Keys are the subtle bit. Template fields need stable schema keys, but a
// freeform block added by hand has an empty key. We derive a key for each
// selected block (its existing key, or one slugged from its label) and
// assign that SAME key to the card's block before switching type, so
// ApplyTypeBlocks reconciles by key instead of appending duplicate empty
// fields. Net result: the card keeps its values, the template gets blank
// (or, for keepValueIDs, predefined) ones, and there are no duplicates.
func (r *Runtime) CreateCardTypeFromCard(cardID, name, icon, color string, blockIDs, keepValueIDs []string) (CardTypeInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CardTypeInfo{}, fmt.Errorf("type name is required")
	}
	card, err := r.Card.Get(cardID)
	if err != nil {
		return CardTypeInfo{}, err
	}

	// Materialise the catalog (seed types + starter templates) up front so
	// the new type's auto-slugged id is checked against the seeds — otherwise
	// naming a type after a not-yet-seeded seed ("Episode", "Feature") would
	// later collide when ListCardTypes appends the seed.
	_ = r.Catalog.ListCardTypes()

	templateBlocks, cardBlocks := buildTypeTemplateFromCard(card.Blocks, blockIDs, keepValueIDs)

	tmpl, err := r.Catalog.CreateCardTemplate(name, templateBlocks)
	if err != nil {
		return CardTypeInfo{}, fmt.Errorf("create template: %w", err)
	}
	typ, err := r.Catalog.CreateUserCardType(name, color, "", "", tmpl.ID)
	if err != nil {
		return CardTypeInfo{}, fmt.Errorf("create card type: %w", err)
	}
	if strings.TrimSpace(icon) != "" {
		if _, err := r.Catalog.UpdateUserCardTypeIcon(typ.ID, icon); err != nil {
			// Non-fatal — the type exists, the icon just didn't stick.
			slog.Warn("create type from card: set icon failed", "type", typ.ID, "err", err)
		}
	}

	// Assign the derived keys to the card's selected blocks BEFORE switching
	// type, so the template merge matches by key (no duplicate fields).
	if _, err := r.Card.UpdateBlocks(cardID, cardBlocks); err != nil {
		return CardTypeInfo{}, fmt.Errorf("key card blocks: %w", err)
	}
	if _, err := r.Card.UpdateType(cardID, typ.ID); err != nil {
		return CardTypeInfo{}, fmt.Errorf("switch card type: %w", err)
	}

	return CardTypeInfo{
		ID:          typ.ID,
		Label:       typ.Label,
		Color:       typ.Color,
		Icon:        strings.TrimSpace(icon),
		Description: typ.Description,
		AIHint:      typ.AIHint,
		TemplateID:  tmpl.ID,
		Builtin:     false,
	}, nil
}

// buildTypeTemplateFromCard turns a card's blocks into (1) template blocks
// (values stripped, unique keys) and (2) a copy of the card's blocks with
// those same keys assigned to the selected rows. Block order is preserved;
// unselected blocks are returned unchanged. Blocks listed in keepValueIDs
// carry their current value into the template instead of a blank one.
func buildTypeTemplateFromCard(cardBlocks []model.Block, selectedIDs, keepValueIDs []string) (templateBlocks, updatedCardBlocks []model.Block) {
	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = true
	}
	keepValue := make(map[string]bool, len(keepValueIDs))
	for _, id := range keepValueIDs {
		keepValue[id] = true
	}

	updatedCardBlocks = make([]model.Block, len(cardBlocks))
	copy(updatedCardBlocks, cardBlocks)

	used := make(map[string]bool)
	for i := range cardBlocks {
		b := cardBlocks[i]
		if !selected[b.ID] {
			continue
		}
		key := uniqueTemplateFieldKey(b.Key, b.Label, b.Type, used)
		updatedCardBlocks[i].Key = key
		templateBlocks = append(templateBlocks, model.Block{
			ID:       "blk-" + uuid.New().String()[:8],
			Type:     b.Type,
			Label:    b.Label,
			Key:      key,
			Value:    templateValueForBlock(b, keepValue[b.ID]),
			Required: b.Required,
			Meta:     cloneBlockMeta(b.Meta),
		})
	}
	return templateBlocks, updatedCardBlocks
}

// templateValueForBlock returns the value a template field should carry: a
// blank default, or — when keep is set — a deep copy of the card's current
// value so it becomes a predefined structure. Kept checklist items are reset
// to unchecked so every new card starts the list fresh.
func templateValueForBlock(b model.Block, keep bool) any {
	if !keep {
		return emptyValueForBlockType(b.Type)
	}
	cloned := cloneJSONValue(b.Value)
	if cloned == nil {
		return emptyValueForBlockType(b.Type)
	}
	if b.Type == model.BlockChecklist {
		resetChecklistItemsDone(cloned)
	}
	return cloned
}

// cloneJSONValue deep-copies a JSON-native block value so the template and
// the originating card never alias the same slice/map.
func cloneJSONValue(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// resetChecklistItemsDone clears the done flag on each checklist item so a
// templatised checklist is a blank structure, not a pre-ticked one.
func resetChecklistItemsDone(v any) {
	items, ok := v.([]any)
	if !ok {
		return
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			if _, has := m["done"]; has {
				m["done"] = false
			}
		}
	}
}

// uniqueTemplateFieldKey returns a stable, unique schema key for a template
// field: the block's existing key if it has one, else a slug of its label
// (falling back to its type). Collisions get a numeric suffix.
func uniqueTemplateFieldKey(existingKey, label, blockType string, used map[string]bool) string {
	base := existingKey
	if base == "" {
		base = slugifyFieldKey(label)
	}
	if base == "" {
		base = blockType
	}
	if base == "" {
		base = "field"
	}
	key := base
	for n := 2; used[key]; n++ {
		key = fmt.Sprintf("%s_%d", base, n)
	}
	used[key] = true
	return key
}

// slugifyFieldKey lowercases and collapses non-alphanumeric runs into single
// underscores, trimming the ends. Mirrors the frontend's labelToKey so a
// given label produces the same key on either side.
func slugifyFieldKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevUnderscore := false
	for _, ch := range s {
		switch {
		case (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9'):
			b.WriteRune(ch)
			prevUnderscore = false
		case !prevUnderscore:
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// emptyValueForBlockType returns the blank/default value for a block type —
// what a freshly-instantiated field looks like before the user fills it in.
func emptyValueForBlockType(blockType string) any {
	switch blockType {
	case model.BlockChecklist, model.BlockList, model.BlockMedia,
		model.BlockSurvey, model.BlockCheckboxGroup:
		return []any{}
	case model.BlockCheckbox:
		return false
	case model.BlockNumber, model.BlockRating, model.BlockProgress:
		return 0
	case model.BlockDivider, model.BlockImage:
		return nil
	default:
		// text, url, select, radio, date, alarm — all string-ish.
		return ""
	}
}

// cloneBlockMeta shallow-copies a block's meta so the template and the card
// don't alias the same map (e.g. select options config).
func cloneBlockMeta(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
