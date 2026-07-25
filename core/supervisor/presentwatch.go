package supervisor

// Presentation gate: presenting is an explicit, presenter-controlled state
// (the Present button starts and stops it) — not an inference from output-
// page polls. The signed present URL stays valid for its 12h window, but
// /present-data serves deck content ONLY while the gate is open: closing it
// is the "stop broadcasting" the button promises. Output pages degrade to a
// waiting state and RESUME when the gate reopens, so an OBS Browser Source
// can be configured before going live. In-memory by design — a server
// restart closes every gate (conservative default: nothing broadcasts
// until a presenter says so).

import "fmt"

// SetPresenting opens or closes the presentation gate for a card,
// publishing present:active / present:idle on transitions (the same
// topics the board indicators and Present button follow).
func (r *Runtime) SetPresenting(cardID string, active bool) error {
	if cardID == "" {
		return fmt.Errorf("cardID is required")
	}
	// Validate on start so bogus IDs can't grow the map; stopping an
	// unknown card is a harmless no-op.
	if active {
		if _, err := r.Card.Get(cardID); err != nil {
			return err
		}
	}
	r.presentMu.Lock()
	was := r.presentGate[cardID]
	if active == was {
		r.presentMu.Unlock()
		return nil
	}
	if r.presentGate == nil {
		r.presentGate = make(map[string]bool)
	}
	if active {
		r.presentGate[cardID] = true
	} else {
		delete(r.presentGate, cardID)
	}
	r.presentMu.Unlock()
	if active {
		r.bus.Publish("present:active", map[string]any{"cardID": cardID})
	} else {
		r.bus.Publish("present:idle", map[string]any{"cardID": cardID})
	}
	return nil
}

// isPresenting reports whether the card's gate is open.
func (r *Runtime) isPresenting(cardID string) bool {
	r.presentMu.Lock()
	defer r.presentMu.Unlock()
	return r.presentGate[cardID]
}

// ListPresentingCards returns the cards whose presentation gate is open.
// Feeds the board's presenting indicators on load; live transitions arrive
// via the events above.
func (r *Runtime) ListPresentingCards() ([]string, error) {
	r.presentMu.Lock()
	defer r.presentMu.Unlock()
	out := make([]string, 0, len(r.presentGate))
	for id := range r.presentGate {
		out = append(out, id)
	}
	return out, nil
}
