package llm

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// parseToolArguments decodes a provider's JSON-string tool arguments.
//
// An ABSENT argument list is legitimate — plenty of tools take none, and
// providers spell that as "", "null" or "{}" — so those return an empty
// map, not an error. Anything else that fails to parse is a genuine
// problem (most often a response truncated mid-arguments) and must be
// reported: running a tool with silently-missing arguments is how
// `set_title` ends up called with no title.
func parseToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseRetryAfter parses an HTTP Retry-After header value, which can be
// either a number of seconds (e.g. "120") or an HTTP date. Returns 0 if
// the header is empty or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	// Integer seconds form
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	// HTTP date form
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
