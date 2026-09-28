package agent

// The runtime's own bookkeeping on the agent card: tracking blocks are
// stamped by key on any card type, missing or incompatible blocks are
// skipped without error, and a block the model changed during the run
// keeps the model's value. Findings is never touched by the runtime.

import (
	"context"
	"strings"
	"testing"
	"time"

	"bruv/internal/model"
)

func trackingBlocks() []model.Block {
	return []model.Block{
		{ID: "b1", Type: model.BlockSelect, Key: "status", Label: "Status",
			Meta: map[string]any{"options": []any{"idle", "running", "success", "failed", "disabled"}}},
		{ID: "b2", Type: model.BlockText, Key: "last_run", Label: "Last Run"},
		{ID: "b3", Type: model.BlockDate, Key: "last_run_at", Label: "Last Run At", Meta: map[string]any{"format": "date-time"}},
		{ID: "b4", Type: model.BlockText, Key: "findings", Label: "Findings", Value: "earlier finding"},
	}
}

func value(card *model.Card, key string) any {
	for _, b := range card.Blocks {
		if b.Key == key {
			return b.Value
		}
	}
	return nil
}

func TestFinalValuesStampAnyCardType(t *testing.T) {
	finished := time.Date(2026, 9, 29, 4, 30, 0, 0, time.UTC)
	card := &model.Card{Type: "task", Blocks: trackingBlocks()} // not an "agent" card
	run := model.AgentRun{Status: "success", Summary: "Filed two race cards."}

	if !applyRunStamp(card, finalValues(run, finished), nil) {
		t.Fatal("nothing stamped")
	}
	if value(card, "status") != "success" {
		t.Errorf("status = %v", value(card, "status"))
	}
	if value(card, "last_run") != "Filed two race cards." {
		t.Errorf("last_run = %v", value(card, "last_run"))
	}
	if got, _ := value(card, "last_run_at").(string); got != "2026-09-29T04:30:00Z" {
		t.Errorf("last_run_at = %q, want the full timestamp in a date-time block", got)
	}
	if value(card, "findings") != "earlier finding" {
		t.Error("findings must be left to the model")
	}
}

func TestStampSkipsMissingAndRejectedBlocks(t *testing.T) {
	// No tracking blocks at all: nothing to do, no error.
	bare := &model.Card{Blocks: []model.Block{{ID: "x", Type: model.BlockText, Key: "notes", Value: "keep"}}}
	if applyRunStamp(bare, finalValues(model.AgentRun{Status: "failure", Error: "boom"}, time.Now()), nil) {
		t.Error("stamped a card that has no tracking blocks")
	}

	// A status select without the value's option is skipped, not forced.
	card := &model.Card{Blocks: []model.Block{{ID: "s", Type: model.BlockSelect, Key: "status",
		Value: "todo", Meta: map[string]any{"options": []any{"todo", "done"}}}}}
	applyRunStamp(card, startValues(), nil)
	if value(card, "status") != "todo" {
		t.Errorf("status = %v, want the unsupported 'running' skipped", value(card, "status"))
	}
}

func TestModelChangedBlocksKeepTheModelsValue(t *testing.T) {
	card := &model.Card{Blocks: trackingBlocks()}
	applyRunStamp(card, startValues(), nil)
	afterStart := snapshotFields(card)

	// During the run the model writes its own summary and status.
	card.Blocks[0].Value = "success"
	card.Blocks[1].Value = "My own summary."

	keep := changedSince(afterStart, snapshotFields(card))
	applyRunStamp(card, finalValues(model.AgentRun{Status: "failure", Error: "ran out of turns"}, time.Now()), keep)

	if value(card, "last_run") != "My own summary." || value(card, "status") != "success" {
		t.Errorf("model values overwritten: status=%v last_run=%v", value(card, "status"), value(card, "last_run"))
	}
	if value(card, "last_run_at") == nil || value(card, "last_run_at") == "" {
		t.Error("an untouched block (last_run_at) should still be stamped")
	}
}

func TestFailedRunSummaryNamesTheError(t *testing.T) {
	v := finalValues(model.AgentRun{Status: "failure", Error: "ran out of turns (25)", Summary: "Got halfway."}, time.Now())
	s, _ := v[fieldLastRun].(string)
	if !strings.HasPrefix(s, "Failed: ran out of turns (25)") || !strings.Contains(s, "Got halfway.") {
		t.Errorf("last_run = %q", s)
	}
	if v[fieldStatus] != "failed" || v[fieldError] != "ran out of turns (25)" {
		t.Errorf("status/error = %v / %v", v[fieldStatus], v[fieldError])
	}
}

// update_card edits another card by id through the same path as
// update_self, including date-time blocks keeping their offset.
func TestUpdateCardEditsAnotherCard(t *testing.T) {
	a, r := testRuntime(t)
	agentID := testCard(t, r, "Watcher", nil)
	raceID := testCard(t, r, "Race 5", []model.Block{
		{ID: "d", Type: model.BlockDate, Key: "race_start", Label: "Race start", Meta: map[string]any{"format": "date-time"}},
	})
	agentCard, _ := r.GetCard(agentID)

	result, _ := a.executeAgentToolCall(context.Background(), agentID, agentCard, call("update_card", map[string]any{
		"card_id": raceID, "title": "Race 5 — 2:35pm",
		"updates": []any{map[string]any{"key": "race_start", "value": "2026-10-04T14:35:00+10:00"}},
	}))
	if strings.HasPrefix(result, "error") {
		t.Fatalf("update_card failed: %s", result)
	}
	race, _ := r.GetCard(raceID)
	if race.Title != "Race 5 — 2:35pm" {
		t.Errorf("title = %q", race.Title)
	}
	if got := value(race, "race_start"); got != "2026-10-04T14:35:00+10:00" {
		t.Errorf("race_start = %v, want the offset preserved", got)
	}
	if watcher, _ := r.GetCard(agentID); watcher.Title != "Watcher" {
		t.Error("update_card must not touch the agent's own card")
	}
}

func TestUpdateCardRequiresCardID(t *testing.T) {
	a, r := testRuntime(t)
	agentID := testCard(t, r, "Watcher", nil)
	agentCard, _ := r.GetCard(agentID)
	if result, _ := a.executeAgentToolCall(context.Background(), agentID, agentCard, call("update_card", map[string]any{"title": "x"})); !strings.HasPrefix(result, "error") {
		t.Errorf("expected an error without card_id, got %q", result)
	}
}
