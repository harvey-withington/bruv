package agent

// runGuard stops an agent run from spending tokens without progress.
// Field report 2026-09-29: an agent whose source site returned 403 kept
// searching for alternatives — 56 tool calls, 19 of them failing — until
// it hit a 1,000,000-token budget. Two cheap checks cut that short:
//
//   - An identical call (same tool, same arguments) is not run twice in
//     one run; the model is told to use the earlier result instead, so a
//     page isn't fetched (and its text re-sent) again.
//   - Too many failing tool calls — in a row, or in total — end the run
//     as a failure with the reason, like the token and turn limits do.

import (
	"encoding/json"
	"fmt"
	"strings"

	"bruv/internal/llm"
)

const (
	maxConsecutiveToolErrors = 6
	maxTotalToolErrors       = 12
)

type runGuard struct {
	seen        map[string]bool
	consecutive int
	total       int
}

func newRunGuard() *runGuard { return &runGuard{seen: map[string]bool{}} }

// repeat reports whether tc exactly repeats an earlier call this run,
// returning the note to send the model instead of running it again.
func (g *runGuard) repeat(tc llm.ToolCall) (string, bool) {
	args, _ := json.Marshal(tc.Arguments) // map keys marshal sorted
	key := tc.Name + " " + string(args)
	if g.seen[key] {
		return "Skipped: you already made this exact " + tc.Name + " call in this run. " +
			"Use that earlier result, or try something different — don't repeat it.", true
	}
	g.seen[key] = true
	return "", false
}

// record notes whether a tool call failed.
func (g *runGuard) record(result string) {
	if isToolError(result) {
		g.consecutive++
		g.total++
		return
	}
	g.consecutive = 0
}

// stopReason is non-empty once the run should end.
func (g *runGuard) stopReason() string {
	switch {
	case g.consecutive >= maxConsecutiveToolErrors:
		return fmt.Sprintf("stopped after %d tool calls in a row failed — the source may be blocked or down", g.consecutive)
	case g.total >= maxTotalToolErrors:
		return fmt.Sprintf("stopped after %d tool calls failed — the source may be blocked or down", g.total)
	}
	return ""
}

// isToolError matches how tools report failure to the model: built-ins
// and native tools return "error: …", MCP tools "MCP tool … failed".
// Only those exact shapes count — a fetched page that merely starts
// with the word "Error" is a result, not a failure.
func isToolError(result string) bool {
	r := strings.ToLower(strings.TrimSpace(result))
	return strings.HasPrefix(r, "error:") || (strings.HasPrefix(r, "mcp tool") && strings.Contains(r, " failed"))
}
