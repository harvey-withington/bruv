package agent

import (
	"strings"
	"time"
)

// IsRateLimitError reports whether a run error text looks like a
// provider rate-limit response. Matches the format produced by
// llm.RateLimitError.Error() as well as raw HTTP 429 mentions from
// providers that surface the error text verbatim rather than wrapping
// it in our typed error.
//
// Extracted from app_agent.go so the rate-limit classification can be
// unit-tested and reused from any code path that needs to decide
// "should I honour the provider's backoff or use ours?".
func IsRateLimitError(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "http 429") ||
		strings.Contains(lower, "(429)") ||
		strings.Contains(lower, "too many requests")
}

// ParseRetryAfter extracts the "retry after X" duration that
// llm.RateLimitError.Error() embeds when the provider sent a
// Retry-After header. Returns 0 if no hint is found or parsing fails.
// The expected format is "retry after 2m0s" or similar — any value
// time.ParseDuration can parse works, terminated by a comma or
// closing paren.
func ParseRetryAfter(s string) time.Duration {
	const marker = "retry after "
	lower := strings.ToLower(s)
	idx := strings.Index(lower, marker)
	if idx == -1 {
		return 0
	}
	tail := s[idx+len(marker):]
	end := strings.IndexAny(tail, "),")
	if end == -1 {
		return 0
	}
	dur, err := time.ParseDuration(strings.TrimSpace(tail[:end]))
	if err != nil {
		return 0
	}
	return dur
}

// RetryDelay computes how long to wait before the next retry given
// the error text, a user-configured base backoff in minutes, and the
// current retry count (1-indexed — first retry is count=1).
//
// Two regimes:
//
//   - Generic failures: linear backoff — baseBackoffMins * retryCount.
//     Simple and predictable; the user sets the cadence via the
//     MaxRetries / RetryBackoffMins config pair.
//
//   - Rate-limit failures: ignore the user's cadence entirely and
//     honour the provider's Retry-After hint if present. No hint ->
//     exponential backoff with a 15-minute floor, doubling per
//     retry, capped at 2 hours. Add 10% jitter on top so parallel
//     agents don't stampede back at the same instant.
//
// Extracted so the retry policy is covered by unit tests instead of
// living inline in a 300-line function.
func RetryDelay(errStr string, baseBackoffMins, retryCount int) time.Duration {
	if !IsRateLimitError(errStr) {
		backoff := baseBackoffMins
		if backoff == 0 {
			backoff = 5
		}
		return time.Duration(backoff*retryCount) * time.Minute
	}
	hint := ParseRetryAfter(errStr)
	if hint <= 0 {
		hint = 15 * time.Minute
		for i := 1; i < retryCount && hint < 2*time.Hour; i++ {
			hint *= 2
		}
	}
	// 10% jitter so simultaneously rate-limited agents don't retry
	// in lockstep and hammer the provider all over again.
	return hint + hint/10
}
