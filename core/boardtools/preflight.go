package boardtools

// Preflight: a write staged for the user's review is applied only after
// the model's turn, too late for it to correct a call that can't work.
// So staging first checks what would make the call fail when applied —
// the card, the checklist, new block keys, the card type — and refuses it
// now, with the fix, while the model can still act on it.

import (
	"fmt"
	"strings"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/model"
)

// Preflight reports why the write name(args) would fail if applied now,
// or nil. It changes nothing and isn't exhaustive: applying still checks
// everything.
func Preflight(b Board, name string, args map[string]any) error {
	if name == "create_card" {
		spec, err := cardtools.ParseCardSpec(args)
		if err != nil {
			return err
		}
		return checkType(b, spec.Type)
	}
	if s := meta[name].scope; s != scopeCard && s != scopeCardAndLocation {
		return nil
	}
	id := argStr(args, "card_id")
	if id == "" {
		return fmt.Errorf("card_id is required")
	}
	card, err := b.GetCard(id)
	if err != nil {
		return fmt.Errorf("no card %s: %v", id, err)
	}
	switch name {
	case "add_checklist_items":
		if findChecklist(card, strings.TrimSpace(argStr(args, "checklist"))) == nil {
			return noChecklistError(card, argStr(args, "checklist"))
		}
		if len(cardtools.CoerceChecklistItems(args["items"])) == 0 {
			return fmt.Errorf("items is required: strings, or {text, done} objects")
		}
	case "add_card_blocks":
		blocks, err := cardtools.ParseBlocks(args["blocks"])
		if err != nil {
			return err
		}
		if len(blocks) == 0 {
			return fmt.Errorf("blocks is required and must be a non-empty array")
		}
		return cardtools.CheckNewBlockKeys(card, blocks)
	case "set_card_type":
		return checkType(b, argStr(args, "card_type"))
	}
	return nil
}

// checkType refuses a card type the catalog doesn't have, as applying
// would; empty is the default type.
func checkType(b Board, input string) error {
	if input == "" {
		return nil
	}
	if _, err := b.CatalogService().ResolveType(input); err != nil {
		text, _ := typeErrResult(err)
		return fmt.Errorf("%s", strings.TrimPrefix(text, "error: "))
	}
	return nil
}

// noChecklistError names the card's checklists and how to make a new one.
func noChecklistError(card *model.Card, which string) error {
	return fmt.Errorf("card %s has no checklist %q (its checklists: %s); make one with add_card_blocks, type 'checklist'",
		card.ID, which, checklistNames(card))
}
