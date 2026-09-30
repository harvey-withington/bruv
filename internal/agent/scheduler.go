package agent

import (
	"bruv/internal/logging"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// DueAgent represents a card with an agent that is due to run.
type DueAgent struct {
	CardID    string
	NextRunAt time.Time
}

// ErrSchedulerStopped is returned by TriggerNow once Stop has begun.
var ErrSchedulerStopped = errors.New("agent scheduler is stopped")

// Scheduler polls for due agents and executes them with bounded concurrency.
type Scheduler struct {
	mu      sync.Mutex
	paused  bool
	running map[string]*claim // claimed runs (queued or executing), by card ID
	sem     chan struct{}
	stopCh  chan struct{}
	stopped bool
	// wg counts the poll loop and every claimed run. Add only ever
	// happens under mu while !stopped, so it can never race Stop's Wait.
	wg sync.WaitGroup

	queryFn func() ([]DueAgent, error)
	execFn  func(ctx context.Context, cardID string) error
}

// claim is one claimed run. cancel ends its context, whether the run
// is still queued on the semaphore or already executing; started
// flips once execFn is about to be called.
type claim struct {
	cancel  context.CancelFunc
	started bool
}

// NewScheduler creates a new agent scheduler.
// queryFn returns the list of agents due to run.
// execFn executes a single agent by card ID.
func NewScheduler(queryFn func() ([]DueAgent, error), execFn func(ctx context.Context, cardID string) error) *Scheduler {
	return &Scheduler{
		running: make(map[string]*claim),
		sem:     make(chan struct{}, 3), // max 3 concurrent agents
		stopCh:  make(chan struct{}),
		queryFn: queryFn,
		execFn:  execFn,
	}
}

// Start begins the scheduler poll loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer logging.Recover("agent-scheduler-poll")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Run an initial tick immediately
		s.tick(ctx)

		for {
			select {
			case <-s.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

// Stop signals the scheduler to stop and waits for in-flight agents to
// finish. Queued runs that haven't started yet are dropped, and no new
// run can be claimed once Stop has begun. Callers wanting a prompt stop
// cancel the context the runs were started with first (Runtime.Close
// does) — Stop itself doesn't interrupt a run that is already executing.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	close(s.stopCh)
	s.wg.Wait()
}

// Pause pauses the scheduler (in-flight agents continue, no new ones start).
func (s *Scheduler) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
}

// Resume resumes the scheduler.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
}

// IsPaused returns whether the scheduler is paused.
func (s *Scheduler) IsPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// RunningCount returns the number of agents currently executing.
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}

// IsRunning returns whether a specific card's agent is currently executing.
func (s *Scheduler) IsRunning(cardID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[cardID] != nil
}

// Cancel cancels the claimed run for cardID, if any. A run still queued
// on the concurrency semaphore will now never start; a run already
// executing has its context cancelled. found reports whether a claim
// existed; queued reports that it hadn't started yet (so no
// agent:completed event will come from the run itself).
func (s *Scheduler) Cancel(cardID string) (found, queued bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.running[cardID]
	if c == nil {
		return false, false
	}
	c.cancel()
	return true, !c.started
}

// TriggerNow runs an agent immediately, bypassing the schedule check.
func (s *Scheduler) TriggerNow(ctx context.Context, cardID string) error {
	// Check-and-set under one lock: claiming the slot only inside the
	// spawned goroutine (after sem acquire) left a window where a tick
	// and a "run now" landing together both passed the check and ran
	// the same agent twice. A claimed-but-queued agent counts as
	// running — that's the point.
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrSchedulerStopped
	}
	if s.running[cardID] != nil {
		s.mu.Unlock()
		return fmt.Errorf("agent for card %q is already running", cardID)
	}
	runCtx := s.claimLocked(ctx, cardID)
	s.mu.Unlock()

	go s.run(runCtx, cardID, "agent-trigger-")
	return nil
}

func (s *Scheduler) tick(ctx context.Context) {
	s.mu.Lock()
	if s.paused || s.stopped {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	agents, err := s.queryFn()
	if err != nil {
		slog.Warn("agent scheduler query failed", "err", err)
		return
	}

	for _, ag := range agents {
		// Same atomic check-and-set as TriggerNow — see comment there.
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return
		}
		if s.running[ag.CardID] != nil {
			s.mu.Unlock()
			continue
		}
		runCtx := s.claimLocked(ctx, ag.CardID)
		s.mu.Unlock()

		go s.run(runCtx, ag.CardID, "agent-exec-")
	}
}

// claimLocked records a claim for cardID and registers it with wg.
// Caller holds s.mu and has checked !s.stopped.
func (s *Scheduler) claimLocked(ctx context.Context, cardID string) context.Context {
	runCtx, cancel := context.WithCancel(ctx)
	s.running[cardID] = &claim{cancel: cancel}
	s.wg.Add(1)
	return runCtx
}

// run waits for a concurrency slot and executes one claimed agent. The
// wait gives up when the scheduler stops or the run is cancelled, so a
// queued run never starts after Stop / Cancel.
func (s *Scheduler) run(ctx context.Context, cardID, crashPrefix string) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if c := s.running[cardID]; c != nil {
			c.cancel()
		}
		delete(s.running, cardID)
		s.mu.Unlock()
	}()

	select {
	case s.sem <- struct{}{}: // acquire
	case <-s.stopCh:
		return
	case <-ctx.Done():
		return
	}
	defer func() { <-s.sem }() // release

	// Re-check under the lock: the slot may have been won in the same
	// instant Stop or Cancel fired.
	s.mu.Lock()
	c := s.running[cardID]
	if s.stopped || ctx.Err() != nil || c == nil {
		s.mu.Unlock()
		return
	}
	c.started = true
	s.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			logging.WriteCrash(crashPrefix+cardID, r)
		}
	}()
	_ = s.execFn(ctx, cardID)
}
