package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"bruv/internal/model"
)

func msg(role, content string, at time.Time) model.ChatMessage {
	return model.ChatMessage{Role: role, Content: content, Timestamp: at}
}

// How a finished RunLoop settles the run record (pre-release sweep
// 2026-09-29, §3.3/3.4).
func TestConcludeRun(t *testing.T) {
	started := time.Now().UTC()
	before, after := started.Add(-time.Hour), started.Add(time.Second)
	prev := msg(model.RoleAssistant, "last week's report", before)

	t.Run("cancel is cancelled, not a failure", func(t *testing.T) {
		run := newRun("c")
		run.StartedAt = started
		// RunLoop turns the cancelled provider call into an "Error:" message
		// and returns a nil error.
		cf := &model.ChatFile{Messages: []model.ChatMessage{prev, msg(model.RoleSystem, "Error: context canceled", after)}}
		err := concludeRun(&run, context.Canceled, nil, cf, false, 25)
		if run.Status != "cancelled" || !errors.Is(err, context.Canceled) {
			t.Errorf("status %q err %v, want cancelled", run.Status, err)
		}
	})

	t.Run("provider error is a failure", func(t *testing.T) {
		run := newRun("c")
		run.StartedAt = started
		cf := &model.ChatFile{Messages: []model.ChatMessage{msg(model.RoleSystem, "Error: 502 bad gateway", after)}}
		_ = concludeRun(&run, nil, nil, cf, false, 25)
		if run.Status != "failure" || run.Error != "502 bad gateway" {
			t.Errorf("status %q error %q", run.Status, run.Error)
		}
	})

	t.Run("summary never comes from an earlier run", func(t *testing.T) {
		run := newRun("c")
		run.StartedAt = started
		cf := &model.ChatFile{Messages: []model.ChatMessage{prev, msg(model.RoleAssistant, "", after)}}
		_ = concludeRun(&run, nil, nil, cf, false, 25)
		if run.Summary != "" {
			t.Errorf("summary %q leaked from a previous run", run.Summary)
		}
		if run.Status != "success" {
			t.Errorf("status %q, want success", run.Status)
		}
	})

	t.Run("no reply saved for this run is a failure", func(t *testing.T) {
		run := newRun("c")
		run.StartedAt = started
		cf := &model.ChatFile{Messages: []model.ChatMessage{prev}}
		_ = concludeRun(&run, nil, nil, cf, false, 25)
		if run.Status != "failure" || run.Summary != "" {
			t.Errorf("status %q summary %q, want a failure with no borrowed summary", run.Status, run.Summary)
		}
	})

	t.Run("loop error is a failure", func(t *testing.T) {
		run := newRun("c")
		if err := concludeRun(&run, nil, errors.New("save chat reply: disk full"), nil, false, 25); err == nil || run.Status != "failure" {
			t.Errorf("status %q err %v", run.Status, err)
		}
	})

	t.Run("success and exhausted", func(t *testing.T) {
		run := newRun("c")
		run.StartedAt = started
		cf := &model.ChatFile{Messages: []model.ChatMessage{msg(model.RoleAssistant, "done", after)}}
		_ = concludeRun(&run, nil, nil, cf, false, 25)
		if run.Status != "success" || run.Error != "" || run.Summary != "done" {
			t.Errorf("got %+v", run)
		}
		run = newRun("c")
		run.StartedAt = started
		_ = concludeRun(&run, nil, nil, cf, true, 25)
		if run.Status != "failure" || run.Summary != "done" {
			t.Errorf("exhausted run: status %q summary %q", run.Status, run.Summary)
		}
	})
}

// A run that panics never reaches concludeRun; the deferred finalize
// then sees the record as it started, which must not read "success".
func TestNewRunStartsFailed(t *testing.T) {
	if run := newRun("c"); run.Status != "failure" || run.Error == "" {
		t.Errorf("new run = %+v, want failure with a reason until concluded", run)
	}
}
