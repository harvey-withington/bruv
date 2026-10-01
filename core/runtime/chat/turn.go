package chat

import (
	"context"
	"errors"
	"time"
)

// chatTurnTimeout bounds a chat turn that nobody stops. Generous: a
// warn-budget turn may think and write up to the output ceiling, and the
// user can press Stop at any point.
const chatTurnTimeout = 30 * time.Minute

// errStopped is a turn's cancel cause when the user presses Stop, so the
// loop can tell it from a timeout or shutdown.
var errStopped = errors.New("stopped by the user")

type activeTurn struct {
	cancel context.CancelCauseFunc
}

// beginTurn registers a chat turn so Stop can reach it, returning the
// turn's context and the func that ends it (call it when the turn ends).
func (rt *Runtime) beginTurn(chatID string) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(rt.deps.Ctx())
	ctx, cancelTimeout := context.WithTimeout(ctx, chatTurnTimeout)
	turn := &activeTurn{cancel: cancel}

	rt.turnsMu.Lock()
	rt.turns[chatID] = turn
	rt.turnsMu.Unlock()

	return ctx, func() {
		rt.turnsMu.Lock()
		if rt.turns[chatID] == turn {
			delete(rt.turns, chatID)
		}
		rt.turnsMu.Unlock()
		cancelTimeout()
		cancel(nil)
	}
}

// Stop ends the turn running in chatID, reporting whether one was. The
// turn closes with a "stopped" notice; output already generated is paid
// for, nothing further is.
func (rt *Runtime) Stop(chatID string) bool {
	rt.turnsMu.Lock()
	turn := rt.turns[chatID]
	rt.turnsMu.Unlock()
	if turn == nil {
		return false
	}
	turn.cancel(errStopped)
	return true
}

// stoppedByUser reports whether ctx ended because the user pressed Stop.
func stoppedByUser(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errStopped)
}
