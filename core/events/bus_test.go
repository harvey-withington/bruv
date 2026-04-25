package events

import (
	"sync"
	"testing"
	"time"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	b := NewMemBus(16)
	ch, unsub := b.Subscribe()
	defer unsub()

	b.Publish("card:updated", map[string]string{"cardID": "abc"})

	select {
	case ev := <-ch:
		if ev.Topic != "card:updated" {
			t.Errorf("topic = %q, want card:updated", ev.Topic)
		}
		if ev.ID != 1 {
			t.Errorf("first event ID = %d, want 1", ev.ID)
		}
		if ev.At.IsZero() {
			t.Error("event At should be set")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMultipleSubscribersAllReceive(t *testing.T) {
	b := NewMemBus(16)
	ch1, u1 := b.Subscribe()
	ch2, u2 := b.Subscribe()
	defer u1()
	defer u2()

	b.Publish("agent:started", nil)

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Topic != "agent:started" {
				t.Errorf("topic = %q", ev.Topic)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber missed event")
		}
	}
}

func TestMonotonicIDs(t *testing.T) {
	b := NewMemBus(16)
	ch, unsub := b.Subscribe()
	defer unsub()

	for i := 0; i < 5; i++ {
		b.Publish("t", i)
	}

	var ids []uint64
	for i := 0; i < 5; i++ {
		ids = append(ids, (<-ch).ID)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] != ids[i-1]+1 {
			t.Errorf("IDs not monotonic: %v", ids)
		}
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewMemBus(16)
	ch, unsub := b.Subscribe()
	unsub()

	b.Publish("after-unsub", nil)

	select {
	case ev, ok := <-ch:
		if ok {
			t.Errorf("got event after unsubscribe: %v", ev)
		}
	case <-time.After(50 * time.Millisecond):
		// expected — channel was closed and drained, nothing arrived
	}
}

func TestSlowConsumerDoesNotBlockPublisher(t *testing.T) {
	b := NewMemBus(2) // tiny buffer
	_, unsub := b.Subscribe()
	defer unsub()

	// Publish more than the buffer can hold without draining.
	// The publish path must return promptly — we assert by racing
	// against a deadline.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish("storm", i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on slow consumer")
	}
}

func TestNilReceiverSafe(t *testing.T) {
	// Unit tests that construct *App directly without a bus rely on
	// nil-receiver Publish being a no-op. Assert both methods handle
	// the nil case without panicking.
	var b *MemBus
	b.Publish("whatever", nil) // must not panic
	ch, unsub := b.Subscribe()
	defer unsub()
	// Channel must be a valid closed channel — range-loop exits immediately.
	for range ch {
		t.Fatal("expected closed channel, got a value")
	}
}

func TestConcurrentPublishSafe(t *testing.T) {
	b := NewMemBus(1024)
	ch, unsub := b.Subscribe()
	defer unsub()

	const goroutines = 8
	const perGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				b.Publish("race", j)
			}
		}()
	}
	wg.Wait()

	// Drain — exact count ≤ goroutines*perGoroutine (drops allowed
	// if buffer overrun). Verify no deadlock on drain.
	timeout := time.After(time.Second)
	drained := 0
	for drained < goroutines*perGoroutine {
		select {
		case <-ch:
			drained++
		case <-timeout:
			// Partial drain is fine — this test's aim is "no race
			// panic / no deadlock", not strict delivery.
			return
		}
	}
}
