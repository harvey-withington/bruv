// Package repo helpers for the Suggest-mode pending-edits workflow.
//
// These are pure functions, extracted so the load-bearing accept/reject
// split logic can be unit-tested without a full App harness. The
// equivalent inline code in app.go's ApplyPendingEdits has no direct
// coverage; a subtle bug (e.g. inverted condition, missed status
// filter) would silently apply or drop edits the user meant to reject.
package repo

import "bruv/internal/model"

// SplitPendingEdits partitions a message's edits into two ordered
// lists: those the user opted to accept (via acceptIDs) and those that
// should be rejected. Already-accepted or already-rejected edits are
// skipped — only edits with Status == "pending" are eligible.
//
// Order is preserved from the input slice, which matters when edits
// are applied in sequence (later edits may depend on earlier ones
// having committed).
func SplitPendingEdits(edits []model.PendingEdit, acceptIDs []string) (toAccept, toReject []string) {
	acceptSet := make(map[string]struct{}, len(acceptIDs))
	for _, id := range acceptIDs {
		acceptSet[id] = struct{}{}
	}
	for _, e := range edits {
		if e.Status != "pending" {
			continue
		}
		if _, ok := acceptSet[e.ID]; ok {
			toAccept = append(toAccept, e.ID)
		} else {
			toReject = append(toReject, e.ID)
		}
	}
	return
}
