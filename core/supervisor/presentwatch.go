package supervisor

// Present-activity tracking: a card is "presenting" while its /present-data
// endpoint is being polled (the output page polls every 1.5s; OBS or a
// browser tab both count). Purely in-memory — an idle timer per card flips
// the state off ~5s after the last poll, so closing the output page (or the
// signed URL expiring) clears the indicator without any explicit teardown.
//
// Transitions publish present:active / present:idle so open UIs follow live;
// ListPresentingCards serves the initial state on board load.

import "time"

// presentIdleTimeout is how long after the last /present-data poll a card
// stops counting as presenting. Var (not const) so tests can shrink it.
var presentIdleTimeout = 5 * time.Second

// notePresentPoll records one output-page poll for cardID, publishing
// present:active on the idle→active transition. Called from
// PresentCardJSON — the only code path /present-data reaches.
func (r *Runtime) notePresentPoll(cardID string) {
	r.presentMu.Lock()
	if t, active := r.presentPolls[cardID]; active {
		t.Reset(presentIdleTimeout)
		r.presentMu.Unlock()
		return
	}
	if r.presentPolls == nil {
		r.presentPolls = make(map[string]*time.Timer)
	}
	r.presentPolls[cardID] = time.AfterFunc(presentIdleTimeout, func() {
		r.presentMu.Lock()
		delete(r.presentPolls, cardID)
		r.presentMu.Unlock()
		r.bus.Publish("present:idle", map[string]any{"cardID": cardID})
	})
	r.presentMu.Unlock()
	r.bus.Publish("present:active", map[string]any{"cardID": cardID})
}

// ListPresentingCards returns the IDs of cards whose output page is being
// actively polled right now. Feeds the board's presenting indicators on
// load; live transitions arrive via the events above.
func (r *Runtime) ListPresentingCards() ([]string, error) {
	r.presentMu.Lock()
	defer r.presentMu.Unlock()
	out := make([]string, 0, len(r.presentPolls))
	for id := range r.presentPolls {
		out = append(out, id)
	}
	return out, nil
}
