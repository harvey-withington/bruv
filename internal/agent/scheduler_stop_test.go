package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// waitStarted blocks until the scheduler has handed cardID's claimed
// run to execFn.
func waitStarted(t *testing.T, s *Scheduler, cardID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		c := s.running[cardID]
		started := c != nil && c.started
		s.mu.Unlock()
		if started {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("run for %q never started", cardID)
}

// fillSlots triggers len(sem) runs that block until release is closed,
// so the next claim has to queue on the semaphore.
func fillSlots(t *testing.T, s *Scheduler) {
	t.Helper()
	for i := 0; i < cap(s.sem); i++ {
		id := string(rune('a' + i))
		if err := s.TriggerNow(context.Background(), id); err != nil {
			t.Fatalf("TriggerNow %s: %v", id, err)
		}
		waitStarted(t, s, id)
	}
}

// TestStopDropsQueuedRuns: runs queued on the semaphore must not wait
// for a slot (and then start) once Stop begins — before the fix they
// ignored stopCh, so Stop waited for every queued run to execute.
func TestStopDropsQueuedRuns(t *testing.T) {
	release := make(chan struct{})
	var queuedRan atomic.Bool
	s := NewScheduler(
		func() ([]DueAgent, error) { return nil, nil },
		func(ctx context.Context, cardID string) error {
			if cardID == "queued" {
				queuedRan.Store(true)
				return nil
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	fillSlots(t, s)
	if err := s.TriggerNow(ctx, "queued"); err != nil {
		t.Fatalf("TriggerNow queued: %v", err)
	}

	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	// Stop is waiting on the three executing runs; free them.
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
	cancel()
	if queuedRan.Load() {
		t.Fatal("queued run started after Stop")
	}
	if err := s.TriggerNow(context.Background(), "late"); !errors.Is(err, ErrSchedulerStopped) {
		t.Fatalf("TriggerNow after Stop = %v, want ErrSchedulerStopped", err)
	}
}

// TestCancelQueuedRunNeverStarts: cancelling a claimed-but-queued run
// used to be a silent no-op (no cancel func existed yet), so the run
// started later anyway.
func TestCancelQueuedRunNeverStarts(t *testing.T) {
	release := make(chan struct{})
	var queuedRan atomic.Bool
	s := NewScheduler(
		func() ([]DueAgent, error) { return nil, nil },
		func(ctx context.Context, cardID string) error {
			if cardID == "queued" {
				queuedRan.Store(true)
				return nil
			}
			<-release
			return nil
		},
	)
	fillSlots(t, s)
	if err := s.TriggerNow(context.Background(), "queued"); err != nil {
		t.Fatalf("TriggerNow queued: %v", err)
	}
	found, queued := s.Cancel("queued")
	if !found || !queued {
		t.Fatalf("Cancel = (%v, %v), want (true, true)", found, queued)
	}
	close(release)
	s.Stop()
	if queuedRan.Load() {
		t.Fatal("cancelled queued run started")
	}
	if s.IsRunning("queued") {
		t.Fatal("cancelled run still claimed")
	}
}

// TestCancelExecutingRunCancelsContext: Cancel on a started run ends
// the context execFn received.
func TestCancelExecutingRunCancelsContext(t *testing.T) {
	s := NewScheduler(
		func() ([]DueAgent, error) { return nil, nil },
		func(ctx context.Context, cardID string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	)
	if err := s.TriggerNow(context.Background(), "card-1"); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, s, "card-1")
	if found, queued := s.Cancel("card-1"); !found || queued {
		t.Fatalf("Cancel = (%v, %v), want (true, false)", found, queued)
	}
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled run did not end")
	}
}

// TestTriggerDuringStopNeverPanics races TriggerNow against Stop: a
// wg.Add concurrent with wg.Wait used to be possible.
func TestTriggerDuringStopNeverPanics(t *testing.T) {
	for i := 0; i < 50; i++ {
		s := NewScheduler(
			func() ([]DueAgent, error) { return nil, nil },
			func(ctx context.Context, cardID string) error { return nil },
		)
		s.Start(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			for j := 0; j < 50; j++ {
				_ = s.TriggerNow(context.Background(), string(rune('a'+j%26)))
			}
		}()
		s.Stop()
		<-done
	}
}
