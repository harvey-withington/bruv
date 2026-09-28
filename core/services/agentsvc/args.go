package agentsvc

// PatchFromArgs decodes the JSON arguments of an LLM tool call (MCP
// configure_card_agent, chat configure_agent) into a ConfigPatch. Keys
// match the AgentConfig JSON tags; a key that is present is applied, an
// absent one is left unchanged. One parser keeps every tool surface
// agreeing on names, types and date formats.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// localTimeLayouts are the zone-less forms timeArg accepts, most
// specific first.
var localTimeLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"}

// PatchFromArgs builds a ConfigPatch from tool-call arguments. Type
// mismatches are reported together rather than silently dropped.
func PatchFromArgs(a map[string]any) (ConfigPatch, error) {
	var p ConfigPatch
	var errs []error
	fail := func(key, want string) { errs = append(errs, fmt.Errorf("%s must be %s", key, want)) }

	boolArg := func(key string) *bool {
		v, ok := a[key]
		if !ok {
			return nil
		}
		b, ok := v.(bool)
		if !ok {
			fail(key, "a boolean")
			return nil
		}
		return &b
	}
	strArg := func(key string) *string {
		v, ok := a[key]
		if !ok {
			return nil
		}
		s, ok := v.(string)
		if !ok {
			fail(key, "a string")
			return nil
		}
		s = strings.TrimSpace(s)
		return &s
	}
	numArg := func(key string) *float64 {
		v, ok := a[key]
		if !ok {
			return nil
		}
		n, ok := v.(float64)
		if !ok {
			fail(key, "a number")
			return nil
		}
		return &n
	}
	intArg := func(key string) *int {
		n := numArg(key)
		if n == nil {
			return nil
		}
		i := int(*n)
		if float64(i) != *n {
			fail(key, "a whole number")
			return nil
		}
		return &i
	}
	// listArg accepts a JSON array of strings or, for callers that send
	// one, a comma-separated string.
	listArg := func(key string) *[]string {
		v, ok := a[key]
		if !ok {
			return nil
		}
		out := []string{}
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				s, ok := item.(string)
				if !ok {
					fail(key, "an array of strings")
					return nil
				}
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		case string:
			for _, s := range strings.Split(x, ",") {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		default:
			fail(key, "an array of strings")
			return nil
		}
		return &out
	}
	// timeArg accepts RFC 3339, or a zone-less date-time / date read as
	// the server's local wall-clock time (the shapes a datetime-local or
	// date input produces). An empty string yields the zero time, which
	// clears the field.
	timeArg := func(key string) *time.Time {
		s := strArg(key)
		if s == nil {
			return nil
		}
		if *s == "" {
			return &time.Time{}
		}
		if t, err := time.Parse(time.RFC3339, *s); err == nil {
			return &t
		}
		for _, layout := range localTimeLayouts {
			if t, err := time.ParseInLocation(layout, *s, time.Local); err == nil {
				return &t
			}
		}
		fail(key, "an RFC 3339 timestamp, YYYY-MM-DDTHH:MM (server local time) or YYYY-MM-DD")
		return nil
	}

	p.Enabled = boolArg("enabled")
	p.Goal = strArg("goal")
	p.Schedule = strArg("schedule")
	if s := strArg("new_schedule"); s != nil && *s != "" { // legacy chat alias
		p.Schedule = s
	}
	p.AllowedTools = listArg("allowed_tools")
	p.NotifyOn = listArg("notify_on")
	p.NotifyChannels = listArg("notify_channel")
	p.LLMAccountID = strArg("llm_account_id")
	p.LLMModel = strArg("llm_model")
	p.MaxTokensBudget = intArg("max_tokens_budget")
	p.MinIntervalMins = intArg("min_interval_minutes")
	p.MaxRetries = intArg("max_retries")
	p.RetryBackoffMins = intArg("retry_backoff_minutes")
	p.CostBudgetUSD = numArg("cost_budget_usd")
	p.StartDate = timeArg("start_date")
	p.EndDate = timeArg("end_date")
	p.ActiveWindowStart = strArg("active_window_start")
	p.ActiveWindowEnd = strArg("active_window_end")
	p.OneShot = boolArg("one_shot")
	p.Timezone = strArg("timezone")
	p.NextRunAt = timeArg("next_run_at")
	return p, errors.Join(errs...)
}
