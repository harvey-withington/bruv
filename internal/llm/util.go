package llm

import (
	"net/http"
	"strconv"
	"time"
)

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
