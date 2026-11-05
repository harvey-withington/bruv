package boardtools_test

import (
	"strings"
	"testing"

	"bruv/core/boardtools"
)

// Every registered tool is in the instructions' index, so a client that
// loads tools lazily still learns it exists (dogfooding, 2026-11-05: the
// model never found add_card_blocks and said checklists couldn't be made).
func TestInstructionsNameEveryTool(t *testing.T) {
	text := boardtools.Instructions("Board")
	for _, name := range boardtools.Names() {
		if !strings.Contains(text, name) {
			t.Errorf("instructions don't mention %s", name)
		}
	}
	if !strings.Contains(text, `"Board" BRUV board`) {
		t.Error("instructions don't name the board")
	}
}
