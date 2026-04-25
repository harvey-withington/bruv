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
	"sync/atomic"
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
	Subscribe() (<-chan Event, func())
}

// MemBus is an in-process fanout bus.
type MemBus struct {
	mu     sync.Mutex
	subs   []chan Event
	nextID uint64
	bufSz  int
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
func (b *MemBus) Publish(topic string, payload any) {
	if b == nil {
		return
	}
	id := atomic.AddUint64(&b.nextID, 1)
	ev := Event{
		ID:      id,
		Topic:   topic,
		Payload: payload,
		At:      time.Now(),
	}

	// Snapshot subs under lock, then deliver outside so a slow
	// consumer's nonblocking-send can't tie up the publish path.
	b.mu.Lock()
	subs := make([]chan Event, len(b.subs))
	copy(subs, b.subs)
	b.mu.Unlock()

	for _, ch := range subs {
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
// Nil receiver returns a closed channel so callers can range-loop
// safely (useful for tests).
func (b *MemBus) Subscribe() (<-chan Event, func()) {
	if b == nil {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan Event, b.bufSz)

	b.mu.Lock()
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
		})
	}
	return ch, unsub
}

// Compile-time check that MemBus satisfies Bus.
var _ Bus = (*MemBus)(nil)
