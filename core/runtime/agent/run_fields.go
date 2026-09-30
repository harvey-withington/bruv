package agent

// The runtime stamps each run's outcome onto the agent card's tracking
// blocks — Status, Last Run, Last Run At, and Error — so the card shows
// what happened even when the model never reports (e.g. it ran out of
// turns). Previously these changed only if the model called update_self.
//
// Rules:
//   - Blocks are matched by key on whatever card carries the agent; no
//     card type is assumed. A missing block is simply skipped.
//   - A value the block rejects (a select without that option) is
//     skipped, never forced, and never fails the run.
//   - At the end of a run, a block the model changed during the run is
//     left alone: the model's own value wins.

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bruv/core/runtime/tools"
	"bruv/internal/model"
	"bruv/internal/repo"
)

// Tracking-block keys the runtime writes. The prompt leaves them out of
// the model's must-update list (promptfmt.SystemManagedAgentFields).
const (
	fieldStatus    = "status"
	fieldLastRun   = "last_run"
	fieldLastRunAt = "last_run_at"
	fieldError     = "error"
)

var trackedFields = []string{fieldStatus, fieldLastRun, fieldLastRunAt, fieldError}

// lastRunSummaryLimit keeps the Last Run block to a readable summary;
// the full report stays in the run history.
const lastRunSummaryLimit = 600

// fieldSnapshot is the tracked blocks' values at one point in a run,
// keyed by block key; a missing block has no entry.
type fieldSnapshot map[string]string

// snapshotFields records the current values of the tracked blocks.
func snapshotFields(card *model.Card) fieldSnapshot {
	snap := fieldSnapshot{}
	for _, key := range trackedFields {
		for _, b := range card.Blocks {
			if b.Key == key {
				snap[key] = fmt.Sprint(b.Value)
				break
			}
		}
	}
	return snap
}

// changedSince lists the tracked blocks whose value differs from before.
func changedSince(before, now fieldSnapshot) map[string]bool {
	changed := map[string]bool{}
	for key, v := range now {
		if prev, ok := before[key]; ok && prev != v {
			changed[key] = true
		}
	}
	return changed
}

// applyRunStamp writes values (block key → value) into the card's
// matching blocks, skipping keys in keep, and reports whether anything
// changed.
func applyRunStamp(card *model.Card, values map[string]any, keep map[string]bool) bool {
	changed := false
	for key, value := range values {
		if keep[key] {
			continue
		}
		for i := range card.Blocks {
			if card.Blocks[i].Key != key {
				continue
			}
			coerced, err := tools.CoerceBlockValueForBlock(&card.Blocks[i], value)
			if err == nil && coerced != nil {
				card.Blocks[i].Value = coerced
				changed = true
			}
			break
		}
	}
	return changed
}

// startValues is written when a run begins.
func startValues() map[string]any {
	return map[string]any{fieldStatus: "running"}
}

// finalValues builds the end-of-run values from the run record.
func finalValues(run model.AgentRun, finishedAt time.Time) map[string]any {
	v := map[string]any{fieldLastRunAt: finishedAt.Format(time.RFC3339)}
	switch run.Status {
	case "success":
		v[fieldStatus], v[fieldError] = "success", ""
		if run.Summary != "" {
			v[fieldLastRun] = truncateSummary(run.Summary)
		}
	case "cancelled":
		v[fieldStatus], v[fieldLastRun] = "idle", "Cancelled."
	default:
		v[fieldStatus], v[fieldError] = "failed", run.Error
		summary := "Failed: " + run.Error
		if run.Summary != "" {
			summary += "\n\n" + run.Summary
		}
		v[fieldLastRun] = truncateSummary(summary)
	}
	return v
}

func truncateSummary(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > lastRunSummaryLimit {
		return strings.TrimSpace(string(r[:lastRunSummaryLimit])) + "…"
	}
	return s
}

// stampCard writes values onto the card and saves it when something
// changed. A tracked block whose value differs from since (a snapshot
// taken earlier in the run; nil for none) was changed by the model and is
// left alone. Best-effort: bookkeeping never fails a run. Returns the
// tracked values as they stand afterwards.
func (rt *Runtime) stampCard(cardID string, values map[string]any, since fieldSnapshot) fieldSnapshot {
	changed := false
	card, err := rt.deps.Repo().MutateCard(cardID, func(card *model.Card) error {
		keep := changedSince(since, snapshotFields(card))
		if changed = applyRunStamp(card, values, keep); !changed {
			return repo.ErrNoChange
		}
		return nil
	})
	if err != nil {
		slog.Warn("agent run: stamp card failed", "card_id", cardID, "err", err)
		return nil
	}
	if changed {
		if rt.deps.Index() != nil {
			rt.idxIncrementalRefresh()
		}
		rt.emitCardUpdated(cardID)
	}
	return snapshotFields(card)
}

// finishStamp writes the end-of-run values, keeping any tracked block
// the model changed since afterStart (the snapshot stampCard returned
// when the run began).
func (rt *Runtime) finishStamp(cardID string, run model.AgentRun, finishedAt time.Time, afterStart fieldSnapshot) {
	rt.stampCard(cardID, finalValues(run, finishedAt), afterStart)
}
