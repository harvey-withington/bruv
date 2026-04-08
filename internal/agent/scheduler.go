package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DueAgent represents a card with an agent that is due to run.
type DueAgent struct {
	CardID    string
	NextRunAt time.Time
}

// Scheduler polls for due agents and executes them with bounded concurrency.
type Scheduler struct {
	mu      sync.Mutex
	paused  bool
	running map[string]bool
	sem     chan struct{}
	stopCh  chan struct{}
	stopped bool
	wg      sync.WaitGroup

	queryFn func() ([]DueAgent, error)
	execFn  func(ctx context.Context, cardID string) error
}

// NewScheduler creates a new agent scheduler.
// queryFn returns the list of agents due to run.
// execFn executes a single agent by card ID.
func NewScheduler(queryFn func() ([]DueAgent, error), execFn func(ctx context.Context, cardID string) error) *Scheduler {
	return &Scheduler{
		running: make(map[string]bool),
		sem:     make(chan struct{}, 3), // max 3 concurrent agents
		stopCh:  make(chan struct{}),
		queryFn: queryFn,
		execFn:  execFn,
	}
}

// Start begins the scheduler poll loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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

// Stop signals the scheduler to stop and waits for in-flight agents to finish.
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
	return s.running[cardID]
}

// TriggerNow runs an agent immediately, bypassing the schedule check.
func (s *Scheduler) TriggerNow(ctx context.Context, cardID string) error {
	s.mu.Lock()
	if s.running[cardID] {
		s.mu.Unlock()
		return fmt.Errorf("agent for card %q is already running", cardID)
	}
	s.mu.Unlock()

	s.wg.Add(1)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sem <- struct{}{} // acquire
		s.mu.Lock()
		s.running[cardID] = true
		s.mu.Unlock()

		defer func() {
			if r := recover(); r != nil {
				log.Printf("agent scheduler: panic running card %s: %v\n", cardID, r)
			}
			<-s.sem // release
			s.mu.Lock()
			delete(s.running, cardID)
			s.mu.Unlock()
		}()

		_ = s.execFn(ctx, cardID)
	}()

	return nil
}

func (s *Scheduler) tick(ctx context.Context) {
	s.mu.Lock()
	if s.paused {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	agents, err := s.queryFn()
	if err != nil {
		log.Printf("agent scheduler: query error: %v\n", err)
		return
	}

	for _, ag := range agents {
		s.mu.Lock()
		alreadyRunning := s.running[ag.CardID]
		s.mu.Unlock()
		if alreadyRunning {
			continue
		}

		cardID := ag.CardID
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.sem <- struct{}{} // acquire
			s.mu.Lock()
			s.running[cardID] = true
			s.mu.Unlock()

			defer func() {
				if r := recover(); r != nil {
					log.Printf("agent scheduler: panic running card %s: %v\n", cardID, r)
				}
				<-s.sem // release
				s.mu.Lock()
				delete(s.running, cardID)
				s.mu.Unlock()
			}()

			_ = s.execFn(ctx, cardID)
		}()
	}
}
