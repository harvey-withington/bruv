// Package events is the transport-agnostic event bus. Domain services
// publish events here; transports (Wails IPC today, HTTP WebSocket
// later) subscribe and fan out to their wire.
//
// The single-process in-memory implementation (MemBus) is the only one
// that exists today. It's the foundation for the Wails→HTTP/WS
// migration in phase 3 — once a WebSocket transport is added, it just
// becomes another subscriber on the same bus, and no domain code needs
// to change.
//
// Semantics:
//   - Publish is non-blocking and fire-and-forget. If a subscriber's
//     channel is full (slow consumer) the event is dropped for that
//     subscriber only — the publish path never blocks the caller.
//   - Per-topic monotonic event IDs are assigned inside the bus so
//     future WebSocket resume-cursor logic (?since=N) has a stable
//     ordering to resume from.
//   - Events are delivered in publish order per subscriber. Relative
//     order across topics is preserved within a single subscriber.
package events

import (
	"sync"
	"time"
)

// Event is a single published message. Payload is an arbitrary
// JSON-serialisable value (typically a map or a domain struct).
type Event struct {
	ID      uint64    `json:"id"`
	Topic   string    `json:"topic"`
	Payload any       `json:"payload"`
	At      time.Time `json:"at"`
}

// Bus is the publish/subscribe contract. A single MemBus implementation
// lives in this package; further implementations (WebSocket broadcast,
// test stub, etc.) will conform to the same interface.
type Bus interface {
	// Publish fans an event out to every active subscriber.
	// Non-blocking: slow consumers have events dropped for them,
	// but the caller is never delayed.
	Publish(topic string, payload any)

	// Subscribe returns a read channel delivering every event
	// published after the subscription, plus an unsubscribe func.
	// The unsubscribe closes the channel; calling it twice is safe.
	// Publish never sends on a closed channel, whatever the timing.
	Subscribe() (<-chan Event, func())
}

// MemBus is an in-process fanout bus.
//
// Delivery happens while holding mu. Sends are non-blocking selects so
// holding the lock never stalls on a slow consumer, and it is what makes
// unsubscribe safe: a subscriber channel is only ever closed under mu,
// after it has been removed from subs, so Publish can never send on a
// closed channel (which would panic and take the process down).
type MemBus struct {
	mu     sync.Mutex
	subs   []chan Event
	nextID uint64
	bufSz  int
	closed bool
}

// NewMemBus returns a ready-to-use in-memory bus. bufferSize is the
// per-subscriber channel capacity; events beyond it are dropped on
// slow consumers. 128 is a reasonable default for typical UI event
// volume (card:updated storms during bulk imports are the stress case).
func NewMemBus(bufferSize int) *MemBus {
	if bufferSize <= 0 {
		bufferSize = 128
	}
	return &MemBus{bufSz: bufferSize}
}

// Publish delivers payload to every subscriber under the given topic.
// A nil receiver is a no-op — lets unit tests that construct *App
// directly without wiring the bus skip event delivery without crashing.
// Publishing after Close is a no-op.
func (b *MemBus) Publish(topic string, payload any) {
	if b == nil {
		return
	}
	at := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	// IDs are assigned under the lock so delivery order matches ID
	// order for every subscriber, even with concurrent publishers.
	b.nextID++
	ev := Event{
		ID:      b.nextID,
		Topic:   topic,
		Payload: payload,
		At:      at,
	}
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// Subscriber is behind; drop this event for it.
			// Could log/count here in future if drop rate matters.
		}
	}
}

// Subscribe returns a channel and an unsubscribe function. The
// channel is buffered; pull from it in a goroutine to avoid drops.
// A nil receiver (or a closed bus) returns a closed channel so callers
// can range-loop safely (useful for tests).
func (b *MemBus) Subscribe() (<-chan Event, func()) {
	if b == nil {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan Event, b.bufSz)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.subs = append(b.subs, ch)
	b.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			for i, c := range b.subs {
				if c == ch {
					b.subs = append(b.subs[:i], b.subs[i+1:]...)
					close(ch)
					return
				}
			}
			// Not found: Close already closed it.
		})
	}
	return ch, unsub
}

// Close ends every subscription (their channels close, so range loops
// and selects terminate), turns later Publish calls into no-ops and
// hands later Subscribe calls an already-closed channel. Idempotent.
// A runtime closes its bus on shutdown so SSE clients bound to it
// disconnect and reconnect to the replacement instead of heartbeating
// against a dead runtime forever.
func (b *MemBus) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}

// Compile-time check that MemBus satisfies Bus.
var _ Bus = (*MemBus)(nil)
