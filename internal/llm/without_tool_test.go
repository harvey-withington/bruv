package llm

import "testing"

// A filed card never sees suggest_pin (Harvey, 2026-09-17).
func TestWithoutToolDropsOnlyTheNamedTool(t *testing.T) {
	defs := CardTools([]string{"task"}, nil, nil)
	if !hasTool(defs, "suggest_pin") {
		t.Fatal("an Inbox card must still be offered suggest_pin")
	}
	filed := WithoutTool(defs, "suggest_pin")
	if hasTool(filed, "suggest_pin") {
		t.Error("suggest_pin must be gone for a filed card")
	}
	if len(filed) != len(defs)-1 {
		t.Errorf("exactly one tool should be removed: %d → %d", len(defs), len(filed))
	}
	for i, d := range filed {
		if i > 0 && d.Name == filed[i-1].Name {
			t.Error("order/identity of the other tools must be preserved")
		}
	}
}

func hasTool(defs []ToolDef, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}
