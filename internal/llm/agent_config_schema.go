package llm

// The one JSON schema for configuring a card's agent. MCP
// configure_card_agent, card-chat configure_agent and project-chat
// configure_agent all use it, and agentsvc.Configure accepts exactly
// these keys (the AgentConfig JSON tags), so the surfaces can't drift.

// AgentScheduleSyntax documents the schedule field.
const AgentScheduleSyntax = "Interval ('30m', '2h', '1d'; minimum 1m), cron shortcut ('@hourly', '@daily', '@weekly', '@every 45m') " +
	"or 5-field cron ('0 9 * * 1-5' = weekdays 09:00, in `timezone`). Empty = runs only when triggered."

// AgentConfigDescription is the shared tool description; readTool names
// the surface's read tool (get_card_agent / get_agent).
func AgentConfigDescription(readTool string) string {
	return "Set up or change a card's autonomous agent. Only the fields you pass change — call " + readTool +
		" first to read the full current config (including the complete goal) and the valid option values, and again " +
		"afterwards to check the result. A working agent needs enabled=true, a specific goal, and a schedule (or a manual run). " +
		"The result lists warnings for anything that would stop it running."
}

// AgentConfigParams is the configure schema. withCardID adds the target
// card (MCP, project chat); readTool names where the option lists live.
func AgentConfigParams(withCardID bool, readTool string) map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	integer := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	props := map[string]any{
		"enabled":  map[string]any{"type": "boolean", "description": "Whether the agent runs. Enabling requires a goal."},
		"goal":     str("The agent's full instruction for every run — this REPLACES the whole goal, so include everything to keep. Be specific about what to check, what to write, and when to notify."),
		"schedule": str(AgentScheduleSyntax),
		"allowed_tools": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"},
			"description": "Tool ids the agent may call (" + readTool + " options.tools lists them; MCP tools are server__tool). " +
				"Grant every tool the goal needs — an agent can use ONLY these, and an empty array means no tools (it can then only reply in text).",
		},
		"notify_on": map[string]any{
			"type": "array", "items": map[string]any{"type": "string", "enum": []any{"success", "failure"}},
			"description": "When to notify the user after a run.",
		},
		"notify_channel": map[string]any{
			"type": "array", "items": map[string]any{"type": "string", "enum": []any{"system", "email", "webhook"}},
			"description": "Extra notification channels; in-app is always on.",
		},
		"llm":                   str("Model or router to run on: a `ref` from " + readTool + " options.llm ('model:<id>' or 'router:<id>'). Empty string = whatever the user assigned to agent runs."),
		"timezone":              str("IANA timezone for cron schedules and the active window, e.g. 'Australia/Brisbane'. Empty = server local time."),
		"start_date":            str("Don't run before this time (RFC 3339; zone-less times and dates are the BRUV server's local time). Empty string clears it."),
		"end_date":              str("Disable the agent after this time (same formats as start_date). Empty string clears it."),
		"active_window_start":   str("Only run from this time of day, HH:MM 24-hour. Set with active_window_end; empty strings for both clear it."),
		"active_window_end":     str("Only run until this time of day, HH:MM 24-hour."),
		"one_shot":              map[string]any{"type": "boolean", "description": "Run once at the next scheduled time, then stop."},
		"next_run_at":           str("Pin the next run to an exact time (RFC 3339, or zone-less in the server's local time) instead of the schedule's next slot."),
		"max_turns":             integer("Model turns per run (each = one response plus its tool calls), up to 100. 0 = default (25). A run that hits it is marked failed."),
		"max_tokens_budget":     integer("Token cap per run. 0 = default (50000). A run that hits it is marked failed."),
		"cost_budget_usd":       map[string]any{"type": "number", "description": "Total spend cap in USD; the agent is disabled when reached. 0 = no cap."},
		"min_interval_minutes":  integer("Minimum minutes between runs. 0 = default (5)."),
		"max_retries":           integer("Retries after a failed run, 0–10. 0 = no retry."),
		"retry_backoff_minutes": integer("Minutes to wait before a retry. 0 = default (5)."),
	}
	required := []string{}
	if withCardID {
		props["card_id"] = str("The id of the card whose agent to configure.")
		required = []string{"card_id"}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// AgentReadDescription is the shared description of the read tool.
const AgentReadDescription = "Read a card's autonomous agent: its full config (complete goal, schedule, tools, model, limits), " +
	"its last few runs (status, summary, error), and `options` — the valid tool ids, models/routers, notification values " +
	"and schedule syntax. Call before changing an agent and after running it."
