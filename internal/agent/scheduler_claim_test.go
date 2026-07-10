package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestTriggerNowClaimsSlotImmediately: the running-slot must be
// claimed atomically with the check. Before the fix the slot was only
// set inside the spawned goroutine (after semaphore acquire), so a
// second TriggerNow — or a tick — landing in that window started a
// duplicate run for the same card.
func TestTriggerNowClaimsSlotImmediately(t *testing.T) {
	block := make(chan struct{})
	var mu sync.Mutex
	execs := 0
	s := NewScheduler(
		func() ([]DueAgent, error) { return nil, nil },
		func(ctx context.Context, cardID string) error {
			mu.Lock()
			execs++
			mu.Unlock()
			<-block
			return nil
		},
	)

	if err := s.TriggerNow(context.Background(), "card-1"); err != nil {
		t.Fatalf("first TriggerNow: %v", err)
	}
	// Immediately after the first call — before the goroutine has
	// necessarily started executing — a second trigger must be refused.
	if err := s.TriggerNow(context.Background(), "card-1"); err == nil {
		t.Fatal("second TriggerNow succeeded, want already-running error")
	}
	if !s.IsRunning("card-1") {
		t.Fatal("IsRunning = false right after TriggerNow, want true (slot claimed)")
	}

	close(block)
	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	if execs != 1 {
		t.Fatalf("execFn ran %d times, want 1", execs)
	}
}

// TestTickSkipsClaimedAgent: a tick arriving while a TriggerNow'd run
// is still queued/executing must not start a second run.
func TestTickSkipsClaimedAgent(t *testing.T) {
	block := make(chan struct{})
	var mu sync.Mutex
	execs := 0
	s := NewScheduler(
		func() ([]DueAgent, error) {
			return []DueAgent{{CardID: "card-1", NextRunAt: time.Now()}}, nil
		},
		func(ctx context.Context, cardID string) error {
			mu.Lock()
			execs++
			mu.Unlock()
			<-block
			return nil
		},
	)

	if err := s.TriggerNow(context.Background(), "card-1"); err != nil {
		t.Fatalf("TriggerNow: %v", err)
	}
	s.tick(context.Background()) // must see the claimed slot and skip

	close(block)
	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	if execs != 1 {
		t.Fatalf("execFn ran %d times, want 1", execs)
	}
}
