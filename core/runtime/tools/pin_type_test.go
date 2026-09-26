package tools

import (
	"strings"
	"testing"
)

func TestPinTypeConflict(t *testing.T) {
	cats := []CategoryPath{
		{CategoryID: "open", Breadcrumb: "Harv / Admin / Home / Chores"},
		{CategoryID: "inbox", Breadcrumb: "Harv / Admin / Home / Inbox", AcceptedTypes: []string{"home-todo"}},
	}
	cases := []struct {
		name, cat, typ string
		conflict       bool
	}{
		{"unrestricted category", "open", "task", false},
		{"accepted type", "inbox", "home-todo", false},
		{"refused type", "inbox", "task", true},
		{"untyped card", "inbox", "", false},
		{"unknown category is the pin tool's problem", "nope", "task", false},
		{"no category id (names path)", "", "task", false},
	}
	for _, c := range cases {
		got := PinTypeConflict(cats, c.cat, c.typ)
		if (got != "") != c.conflict {
			t.Errorf("%s: conflict=%v, got %q", c.name, c.conflict, got)
		}
		if c.conflict && (!strings.HasPrefix(got, "error:") || !strings.Contains(got, "home-todo") || !strings.Contains(got, `"task"`)) {
			t.Errorf("%s: message must name the accepted types and the card's type: %q", c.name, got)
		}
	}
}
