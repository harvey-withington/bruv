package agent

// update_self: apply the model's updates to the agent's own card. The
// update logic is shared with the native update_card tool
// (tools.ApplyCardUpdates); this path saves directly so an agent run's
// own-card writes stay one atomic file write.

import (
	"time"

	"bruv/core/runtime/tools"
)

// updateCard applies title / due_date / tags / updates[] from a tool
// call's arguments to the card and saves it.
func (rt *Runtime) updateCard(cardID string, args map[string]any) error {
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		return err
	}
	if err := tools.ApplyCardUpdates(card, args); err != nil {
		return err
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
