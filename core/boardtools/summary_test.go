package boardtools_test

import (
	"testing"

	"bruv/core/boardtools"
)

// A staged update_card names what it changes (dogfooding, 2026-11-05: the
// review only said "update card").
func TestUpdateCardSummary(t *testing.T) {
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"updates": []any{map[string]any{"key": "Preparation to Leave", "value": []any{"x"}}}}, "Updated Preparation to Leave"},
		{map[string]any{"title": "Melbourne", "due_date": "", "updates": []any{map[string]any{"key": "notes"}, map[string]any{"key": "Flight"}}}, "Updated title → Melbourne; cleared the due date; notes, Flight"},
		{map[string]any{"tags": []any{"travel"}}, "Updated tags → travel"},
		{map[string]any{}, "Updated the card"},
	} {
		if got := boardtools.Summary("update_card", tc.args, ""); got != tc.want {
			t.Errorf("Summary(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
