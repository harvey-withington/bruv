package agent

import (
	"testing"
	"time"
)

func TestIsRateLimitError(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"some random network error", false},
		{"openai rate limit (HTTP 429, retry after 2m0s): too many", true},
		{"RATE LIMIT exceeded", true},
		{"anthropic (429) slow down", true},
		{"http 429 Too Many Requests", true},
		{"received Too Many Requests from upstream", true},
		{"context deadline exceeded", false},
	}
	for _, c := range cases {
		if got := IsRateLimitError(c.in); got != c.want {
			t.Errorf("IsRateLimitError(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"nothing matching", 0},
		{"rate limit exceeded", 0},
		// Format produced by llm.RateLimitError.Error() — the
		// realistic shape this function has to parse.
		{"openai rate limit (HTTP 429, retry after 2m0s): blah", 2 * time.Minute},
		{"Retry after 30s, please", 30 * time.Second},
		{"rate limit (429, retry after 1h30m0s): slow", 90 * time.Minute},
		{"retry after bogus, moving on", 0},
		{"retry after 45s)", 45 * time.Second},
		// No terminator -> we return 0 rather than guessing where the
		// duration token ends. Documented contract, not a bug.
		{"retry after 1h30m", 0},
	}
	for _, c := range cases {
		if got := ParseRetryAfter(c.in); got != c.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRetryDelay_GenericFailure_LinearBackoff(t *testing.T) {
	// baseBackoff 5 minutes, retry 1 -> 5m; retry 2 -> 10m; retry 3 -> 15m.
	cases := []struct {
		base, retry int
		want        time.Duration
	}{
		{5, 1, 5 * time.Minute},
		{5, 2, 10 * time.Minute},
		{5, 3, 15 * time.Minute},
		{10, 1, 10 * time.Minute},
		{10, 4, 40 * time.Minute},
	}
	for _, c := range cases {
		if got := RetryDelay("boom", c.base, c.retry); got != c.want {
			t.Errorf("RetryDelay(boom, base=%d, retry=%d) = %v, want %v",
				c.base, c.retry, got, c.want)
		}
	}
}

func TestRetryDelay_GenericFailure_ZeroBaseUsesDefault(t *testing.T) {
	// base 0 -> default 5 minutes.
	if got := RetryDelay("boom", 0, 1); got != 5*time.Minute {
		t.Errorf("RetryDelay with base=0 retry=1 = %v, want 5m", got)
	}
	if got := RetryDelay("boom", 0, 3); got != 15*time.Minute {
		t.Errorf("RetryDelay with base=0 retry=3 = %v, want 15m", got)
	}
}

func TestRetryDelay_RateLimit_UsesHintPlusJitter(t *testing.T) {
	// Provider hint of 60s -> 60s + 10% = 66s.
	errStr := "openai rate limit (HTTP 429, retry after 1m0s): foo"
	got := RetryDelay(errStr, 5, 1)
	want := 60*time.Second + 6*time.Second
	if got != want {
		t.Errorf("RetryDelay(rate-limit, hint=60s) = %v, want %v", got, want)
	}
}

func TestRetryDelay_RateLimit_NoHintExponentialFloor(t *testing.T) {
	// No hint in the error string -> 15m floor, 10% jitter on first
	// retry; double per subsequent retry up to 2h cap.
	errStr := "rate limit hit"

	r1 := RetryDelay(errStr, 5, 1)
	want1 := 15*time.Minute + 90*time.Second
	if r1 != want1 {
		t.Errorf("rate-limit no-hint retry=1 = %v, want %v", r1, want1)
	}

	// retry=2 -> 30m + jitter (the loop doubles ONCE because i starts
	// at 1 and runs while i < retryCount).
	r2 := RetryDelay(errStr, 5, 2)
	want2 := 30*time.Minute + 3*time.Minute
	if r2 != want2 {
		t.Errorf("rate-limit no-hint retry=2 = %v, want %v", r2, want2)
	}

	// High retry count should cap at 2h + 10% jitter.
	rBig := RetryDelay(errStr, 5, 10)
	// Cap is 2h, jitter +10% -> 2h12m.
	wantCap := 2*time.Hour + 12*time.Minute
	if rBig != wantCap {
		t.Errorf("rate-limit no-hint retry=10 = %v, want cap %v", rBig, wantCap)
	}
}

func TestRetryDelay_RateLimit_HintOverridesBaseBackoff(t *testing.T) {
	// Even with a generous base backoff, rate-limit hint wins.
	errStr := "rate limit (HTTP 429, retry after 5s): slow down"
	got := RetryDelay(errStr, 60, 1) // user said 60min base — ignored
	want := 5*time.Second + 500*time.Millisecond
	if got != want {
		t.Errorf("RetryDelay with hint=5s, base=60m = %v, want %v", got, want)
	}
}
