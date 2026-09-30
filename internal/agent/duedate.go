package agent

import (
	"bruv/internal/fsutil"
	"bruv/internal/logging"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DueDateScanner polls for cards with approaching due dates and dispatches notifications.
type DueDateScanner struct {
	mu      sync.Mutex
	stopCh  chan struct{}
	done    chan struct{} // closed when the poll goroutine exits
	started bool
	stopped bool

	cardsDir     string
	notifiedPath string
	legacyPath   string               // pre-per-repo shared state file, read once as a fallback
	notified     map[string]time.Time // "cardID:threshold" → when notified

	thresholds []time.Duration // e.g. 24h, 1h, 0
	channels   string          // notification channels
	enabled    bool

	notifyFn         func(cardID, cardTitle string, threshold time.Duration, overdue bool)
	markAlarmFiredFn func(cardID, blockID string)
}

// NewDueDateScanner creates a new scanner.
//
// The notified state lives in its own file per repo (keyed by the cards
// directory), so two loaded runtimes never overwrite each other's state.
func NewDueDateScanner(cardsDir string, configDir string, notifyFn func(cardID, cardTitle string, threshold time.Duration, overdue bool), markAlarmFiredFn func(cardID, blockID string)) *DueDateScanner {
	s := &DueDateScanner{
		cardsDir:         cardsDir,
		notifiedPath:     filepath.Join(configDir, "due_notified", repoStateKey(cardsDir)+".json"),
		legacyPath:       filepath.Join(configDir, "due_notified.json"),
		notified:         make(map[string]time.Time),
		notifyFn:         notifyFn,
		markAlarmFiredFn: markAlarmFiredFn,
		stopCh:           make(chan struct{}),
		done:             make(chan struct{}),
	}
	s.loadNotified()
	return s
}

// repoStateKey derives a stable per-repo file key from the repo's cards
// directory.
func repoStateKey(cardsDir string) string {
	if abs, err := filepath.Abs(cardsDir); err == nil {
		cardsDir = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(cardsDir)))
	return hex.EncodeToString(sum[:8])
}

// Configure updates the scanner settings.
func (s *DueDateScanner) Configure(enabled bool, thresholdStrs []string, channels string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	s.channels = channels
	s.thresholds = nil
	for _, ts := range thresholdStrs {
		switch ts {
		case "24h":
			s.thresholds = append(s.thresholds, 24*time.Hour)
		case "1h":
			s.thresholds = append(s.thresholds, time.Hour)
		case "0":
			s.thresholds = append(s.thresholds, 0)
		case "overdue":
			s.thresholds = append(s.thresholds, -1) // sentinel for overdue
		}
	}
}

// Start begins the scanner poll loop. No-op once stopped or if already
// started.
func (s *DueDateScanner) Start() {
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go func() {
		defer close(s.done)
		defer logging.Recover("due-date-scanner")
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		// Initial scan after short delay
		select {
		case <-s.stopCh:
			return
		case <-time.After(5 * time.Second):
		}
		s.scan()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.scan()
			}
		}
	}()
}

// Stop stops the scanner and waits for an in-progress scan to finish,
// so no card write (alarm-fired stamp) or state save happens after the
// caller goes on to close the repo.
func (s *DueDateScanner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	started := s.started
	s.mu.Unlock()
	close(s.stopCh)
	if started {
		<-s.done
	}
}

// stopping reports whether Stop has been called; scan checks it between
// cards so a stop doesn't wait for a whole directory walk.
func (s *DueDateScanner) stopping() bool {
	select {
	case <-s.stopCh:
		return true
	default:
		return false
	}
}

func (s *DueDateScanner) scan() {
	s.mu.Lock()
	if !s.enabled || len(s.thresholds) == 0 {
		s.mu.Unlock()
		return
	}
	thresholds := make([]time.Duration, len(s.thresholds))
	copy(thresholds, s.thresholds)
	s.mu.Unlock()

	now := time.Now()
	entries, err := os.ReadDir(s.cardsDir)
	if err != nil {
		return
	}

	changed := false
	for _, e := range entries {
		if s.stopping() {
			break
		}
		name := e.Name()
		// Only look at card JSON files (not .agent.json, .messages.json, etc.)
		if filepath.Ext(name) != ".json" {
			continue
		}
		// Skip non-card files (UUIDs are longer)
		if len(name) < 10 {
			continue
		}
		// Skip agent/chat/pin files that have double extensions like .agent.json
		base := name[:len(name)-5] // remove .json
		if filepath.Ext(base) != "" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.cardsDir, name))
		if err != nil {
			continue
		}

		var card struct {
			ID      string     `json:"id"`
			Title   string     `json:"title"`
			DueDate *time.Time `json:"due_date"`
			Blocks  []struct {
				ID    string         `json:"id"`
				Type  string         `json:"type"`
				Label string         `json:"label"`
				Meta  map[string]any `json:"meta,omitempty"`
			} `json:"blocks"`
		}
		if err := json.Unmarshal(data, &card); err != nil {
			continue
		}

		// --- Alarm block scanning ---
		for _, blk := range card.Blocks {
			if blk.Type != "alarm" {
				continue
			}
			// Check if already fired
			if fired, ok := blk.Meta["alarm_fired"].(bool); ok && fired {
				continue
			}
			rawTime, ok := blk.Meta["alarm_time"].(string)
			if !ok || rawTime == "" {
				continue
			}
			alarmTime, err := time.Parse(time.RFC3339, rawTime)
			if err != nil {
				continue
			}

			alarmKey := fmt.Sprintf("%s:alarm:%s", card.ID, blk.ID)
			s.mu.Lock()
			_, alreadyNotified := s.notified[alarmKey]
			s.mu.Unlock()
			if alreadyNotified {
				continue
			}

			if now.After(alarmTime) || alarmTime.Sub(now) <= time.Minute {
				// Use -2 as sentinel for alarm threshold
				s.notifyFn(card.ID, card.Title, -2, false)
				s.mu.Lock()
				s.notified[alarmKey] = now
				s.mu.Unlock()
				changed = true
				if s.markAlarmFiredFn != nil {
					s.markAlarmFiredFn(card.ID, blk.ID)
				}
			}
		}

		if card.DueDate == nil {
			continue
		}

		due := *card.DueDate
		for _, threshold := range thresholds {
			key := fmt.Sprintf("%s:%v", card.ID, threshold)

			s.mu.Lock()
			_, alreadyNotified := s.notified[key]
			s.mu.Unlock()
			if alreadyNotified {
				continue
			}

			if threshold == -1 {
				// Overdue: notify if past due
				if now.After(due) {
					s.notifyFn(card.ID, card.Title, threshold, true)
					s.mu.Lock()
					s.notified[key] = now
					s.mu.Unlock()
					changed = true
				}
			} else if threshold == 0 {
				// At due: notify within 5 minutes of due time
				diff := due.Sub(now)
				if diff <= 5*time.Minute && diff >= -5*time.Minute {
					s.notifyFn(card.ID, card.Title, threshold, false)
					s.mu.Lock()
					s.notified[key] = now
					s.mu.Unlock()
					changed = true
				}
			} else {
				// Before due: notify when time until due crosses the threshold
				diff := due.Sub(now)
				if diff > 0 && diff <= threshold {
					s.notifyFn(card.ID, card.Title, threshold, false)
					s.mu.Lock()
					s.notified[key] = now
					s.mu.Unlock()
					changed = true
				}
			}
		}
	}

	if changed {
		s.saveNotified()
	}
}

func (s *DueDateScanner) loadNotified() {
	data, err := os.ReadFile(s.notifiedPath)
	if os.IsNotExist(err) {
		// First run since state went per-repo: seed from the old shared
		// file. Its keys are card IDs (globally unique), so another
		// repo's entries are inert here.
		data, err = os.ReadFile(s.legacyPath)
	}
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.notified)
}

func (s *DueDateScanner) saveNotified() {
	s.mu.Lock()
	data, err := json.MarshalIndent(s.notified, "", "  ")
	s.mu.Unlock()
	if err != nil {
		slog.Warn("duedate marshal notified failed", "err", err)
		return
	}
	if err := fsutil.WriteFileAtomic(s.notifiedPath, data, 0o644); err != nil {
		slog.Warn("duedate save notified failed", "path", s.notifiedPath, "err", err)
	}
}
