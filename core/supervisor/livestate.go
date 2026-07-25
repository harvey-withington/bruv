package supervisor

// Live block state: transient per-block runtime state that deliberately
// never touches disk. A slide deck's live presentation position is the
// motivating case — advancing a slide is session state, not a card
// edit, so it must not ride UpdateCardBlocks (which writes the card,
// logs an activity entry, and bumps UpdatedAt on every click).
//
// Semantics:
//   - Held in memory on the repo's runtime; lost on restart by design.
//   - Set validates that the card and block exist so bogus IDs can't
//     grow the map unbounded.
//   - Every Set publishes "block:live" so open surfaces (the deck
//     block's console, a future presenter remote) follow pushes; the
//     unauthenticated /present output page picks the state up via
//     PresentCardJSON, which overlays it on the resolved card.
//
// Starting a presentation at slide N needs no extra API: seed the
// state with SetBlockLiveState before opening the present URL.

import "fmt"

// SetBlockLiveState replaces the transient state for one block and
// broadcasts the change. The state object is small and owned by the
// block type (slide decks use {currentIndex}).
func (r *Runtime) SetBlockLiveState(cardID, blockID string, state map[string]any) error {
	if cardID == "" || blockID == "" {
		return fmt.Errorf("cardID and blockID are required")
	}
	card, err := r.Card.Get(cardID)
	if err != nil {
		return err
	}
	if findBlock(card.Blocks, blockID) == nil {
		return fmt.Errorf("block %s not found on card %s", blockID, cardID)
	}
	r.liveStateMu.Lock()
	if r.liveState == nil {
		r.liveState = make(map[string]map[string]any)
	}
	r.liveState[cardID+"/"+blockID] = state
	r.liveStateMu.Unlock()
	r.bus.Publish("block:live", map[string]any{
		"cardID":  cardID,
		"blockID": blockID,
		"state":   state,
	})
	return nil
}

// GetBlockLiveState returns the transient state for one block, or nil
// when none has been set (callers treat nil as "defaults").
func (r *Runtime) GetBlockLiveState(cardID, blockID string) (map[string]any, error) {
	return r.blockLiveState(cardID, blockID), nil
}

// blockLiveState is the internal nil-on-absent read used by the
// present resolver.
func (r *Runtime) blockLiveState(cardID, blockID string) map[string]any {
	r.liveStateMu.RLock()
	defer r.liveStateMu.RUnlock()
	return r.liveState[cardID+"/"+blockID]
}
