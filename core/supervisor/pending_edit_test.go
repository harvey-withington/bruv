package supervisor

import (
	"testing"

	"bruv/internal/model"
)

// An accepted edit's row must tell the truth about what happened: a
// tool refusal (the "error…" result convention) is a failed row with the
// reason, never a tick (Harvey, 2026-09-25 — a refused pin showed as
// accepted).
func TestResolvePendingEdit(t *testing.T) {
	cases := []struct {
		result, status, reason string
	}{
		{"Card pinned to Harv / Home / Inbox", "accepted", ""},
		{"error: category not found", model.PendingEditFailed, "category not found"},
		{`error pinning card: category "Inbox" does not accept card type "task"`, model.PendingEditFailed, `pinning card: category "Inbox" does not accept card type "task"`},
		{"Error: provider down", model.PendingEditFailed, "provider down"},
	}
	for _, c := range cases {
		e := model.PendingEdit{Label: "Pin to Inbox", Detail: "Harv / Home / Inbox", Status: "pending"}
		resolvePendingEdit(&e, c.result)
		if e.Status != c.status {
			t.Errorf("%q: status %q, want %q", c.result, e.Status, c.status)
		}
		if e.Error != c.reason {
			t.Errorf("%q: reason %q, want %q", c.result, e.Error, c.reason)
		}
		if e.Label != "Pin to Inbox" || e.Detail != "Harv / Home / Inbox" {
			t.Errorf("%q: label/detail must survive resolution", c.result)
		}
	}
}
