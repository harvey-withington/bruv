package agent

import (
	"strings"
	"testing"

	"bruv/internal/llm"
)

func TestRunGuardSkipsIdenticalRepeats(t *testing.T) {
	g := newRunGuard()
	fetch := llm.ToolCall{Name: "web_fetch", Arguments: map[string]any{"url": "https://example.com/a"}}
	if _, repeat := g.repeat(fetch); repeat {
		t.Fatal("the first call is not a repeat")
	}
	if note, repeat := g.repeat(fetch); !repeat || !strings.Contains(note, "already made this exact web_fetch call") {
		t.Errorf("an identical second call should be skipped, got %q", note)
	}
	other := llm.ToolCall{Name: "web_fetch", Arguments: map[string]any{"url": "https://example.com/b"}}
	if _, repeat := g.repeat(other); repeat {
		t.Error("a call with different arguments is not a repeat")
	}
}

// The field-report run: a blocked source kept every fetch failing. The
// guard ends it after a handful of consecutive failures.
func TestRunGuardStopsAfterConsecutiveFailures(t *testing.T) {
	g := newRunGuard()
	for i := 0; i < maxConsecutiveToolErrors-1; i++ {
		g.record("error: HTTP 403: 403 Forbidden")
	}
	if g.stopReason() != "" {
		t.Fatal("stopped too early")
	}
	g.record("error: HTTP 403: 403 Forbidden")
	if reason := g.stopReason(); !strings.Contains(reason, "in a row failed") {
		t.Errorf("want a consecutive-failure stop, got %q", reason)
	}
}

func TestRunGuardSuccessResetsTheStreakButNotTheTotal(t *testing.T) {
	g := newRunGuard()
	for i := 0; i < maxTotalToolErrors; i++ {
		g.record("error: fetch failed")
		g.record("some page text") // alternating: never a long streak
	}
	if reason := g.stopReason(); !strings.Contains(reason, "tool calls failed") {
		t.Errorf("want a total-failure stop, got %q", reason)
	}
}

func TestIsToolErrorMatchesOnlyToolFailureShapes(t *testing.T) {
	for result, want := range map[string]bool{
		"error: HTTP 403: 403 Forbidden":       true,
		`MCP tool "fs__read" failed: boom`:     true,
		"Errors: none found on the page":       false, // page text, not a failure
		"Error 404 — the page you wanted …":    false,
		`{"card_id":"c1","updated":["title"]}`: false,
	} {
		if got := isToolError(result); got != want {
			t.Errorf("isToolError(%q) = %v, want %v", result, got, want)
		}
	}
}
