package supervisor

// Slide-deck mutations that must be atomic from the caller's point of view.
//
// AppendDeckSlide exists so remote producers (the web clipper foremost) can
// add a slide without read-modify-writing the whole deck over the wire — that
// would reintroduce the lost-update class the live-state rework eliminated
// for navigation. The append happens server-side in one call, and the slide
// runs through the same coercion as AI-authored decks (stable sld- id,
// content-type field filtering), so callers can send a minimal slide map.
// Deliberately tool-shaped: registering it as a chat/agent tool later needs
// no signature change.

import (
	"fmt"

	"bruv/core/runtime/tools"
	"bruv/internal/model"
)

// AppendDeckSlide appends one slide to the given slide_deck block and saves
// the card through the normal block-update path (activity log + card:updated
// both fire once — adding a slide IS a card edit, unlike navigating one).
// Returns the updated card.
func (r *Runtime) AppendDeckSlide(cardID, blockID string, slide map[string]any) (*model.Card, error) {
	if cardID == "" || blockID == "" {
		return nil, fmt.Errorf("cardID and blockID are required")
	}
	if len(slide) == 0 {
		return nil, fmt.Errorf("slide is required")
	}
	card, err := r.Card.Get(cardID)
	if err != nil {
		return nil, err
	}
	block := findBlock(card.Blocks, blockID)
	if block == nil {
		return nil, fmt.Errorf("block %s not found on card %s", blockID, cardID)
	}
	if block.Type != model.BlockSlideDeck {
		return nil, fmt.Errorf("block %s is not a slide deck", blockID)
	}

	// Rebuild the deck value with the new slide appended, then run the WHOLE
	// value through the standard slide-deck coercion so the appended slide
	// gets the same normalisation as every other authoring path.
	var slides []any
	var theme any
	if current, ok := block.Value.(map[string]any); ok {
		slides, _ = current["slides"].([]any)
		theme = current["theme"]
	}
	next := map[string]any{"slides": append(append([]any{}, slides...), slide)}
	if theme != nil {
		next["theme"] = theme
	}
	coerced, err := tools.CoerceBlockValueForBlock(block, next)
	if err != nil {
		return nil, fmt.Errorf("coerce slide: %w", err)
	}
	block.Value = coerced

	return r.Card.UpdateBlocks(cardID, card.Blocks)
}
