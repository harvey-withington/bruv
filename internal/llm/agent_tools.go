package llm

// allAgentTools defines the full set of agent-specific tools.
var allAgentTools = []ToolDef{
	{
		Name:        "web_fetch",
		Description: "Fetch a web page and return its text content. CALL THIS whenever the user shares a URL, or when you need to read the full contents of a page you've already found via web_search. Do not paraphrase or guess at content — actually fetch it. Returns plain text extracted from the HTML (up to 4000 chars).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch (must include http:// or https://)",
				},
			},
			"required": []string{"url"},
		},
	},
	{
		Name:        "web_search",
		Description: "Search the public web via DuckDuckGo. CALL THIS — do NOT tell the user to search themselves — whenever the user asks about anything you don't already know: current events, recent news, prices, stock/crypto quotes, sports scores, weather, product info, documentation, 'what's happening with X', 'latest on Y', etc. Returns titles, URLs, and short snippets for up to 10 results. For 'why did X happen' questions, one search is often not enough: if the first results cover the effect (price moved, outage happened) but not the cause (news, geopolitics, macro, regulation), run a SECOND search targeting the likely cause before answering. Follow up with web_fetch on the most relevant result if snippets aren't enough.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query — a short natural-language phrase works best",
				},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "notify",
		Description: "Send a notification to the user. Use this to report results or alert the user about something important.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Notification title (short)",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Notification body (details)",
				},
			},
			"required": []string{"title", "body"},
		},
	},
	{
		Name: "update_self",
		Description: "Update this card's intrinsic fields (title, due date, tags) and/or content blocks. The 'Current Card State' section of the system prompt lists each block's type (text, list, checklist, number, etc.) — match the value format to that type:\n" +
			"  - text / description / findings: send a plain string.\n" +
			"  - list: send an ARRAY of strings, one per list item, e.g. [\"Phnom Penh 20 May $60\", \"Bali 12 Jun $85\"].\n" +
			"  - checklist: send an ARRAY of strings (each becomes an unchecked item) OR an array of {text, done} objects to set done state.\n" +
			"  - number: send a number (or numeric string).\n" +
			"  - date: send an ISO-8601 date/time string.\n" +
			"  - select / radio: send the chosen option as a string.\n" +
			"If you send a plain string to a list or checklist block it will be split by newlines as a fallback, but sending an array is strongly preferred. Use existing block keys to update them; use a new key to create a new text block.\n" +
			"To change intrinsic card fields, use the top-level 'title', 'due_date', or 'tags' parameters.",
		Parameters: CardUpdateParams(false),
	},
	{
		Name:        "http_request",
		Description: "Make an HTTP request to an external API. Returns the response body.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method (GET, POST, PUT, DELETE)",
					"enum":        []any{"GET", "POST", "PUT", "DELETE"},
				},
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to request",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Request body (for POST/PUT). Optional.",
				},
			},
			"required": []string{"method", "url"},
		},
	},
}

// BlockFormatDesc documents a new date block's optional format, shared by
// the agent's and the MCP server's create_card block schemas.
const BlockFormatDesc = "Date blocks only: 'date' (YYYY-MM-DD, the default) or 'date-time', which keeps the time " +
	"and the UTC offset of an ISO 8601 value such as '2026-10-04T14:35:00+10:00'."

// CardUpdateParams is the schema shared by update_self and the native update_card tool;
// withCardID adds the target card for update_card.
func CardUpdateParams(withCardID bool) map[string]any {
	props := map[string]any{
		"title": map[string]any{
			"type":        "string",
			"description": "New card title. Omit to leave unchanged.",
		},
		"due_date": map[string]any{
			"type":        "string",
			"description": "Due date in YYYY-MM-DD or ISO-8601 format. Omit to leave unchanged.",
		},
		"tags": map[string]any{
			"type":        "array",
			"description": "Set the card's tags. Omit to leave unchanged.",
			"items":       map[string]any{"type": "string"},
		},
		"updates": map[string]any{
			"type":        "array",
			"description": "List of block updates",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "The block key or label to update (e.g. 'description', 'Flight Options')",
					},
					"value": map[string]any{
						// Deliberately NOT typed — different block types accept
						// different value shapes (string, array, number, object).
						// The Go handler parses based on the target block's type.
						"description": "The new value. See the tool description for format requirements per block type.",
					},
				},
				"required": []string{"key", "value"},
			},
		},
	}
	required := []string{}
	if withCardID {
		props["card_id"] = map[string]any{"type": "string", "description": "The id of the card to update."}
		required = []string{"card_id"}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// WebTools returns just the web-browsing tool definitions (web_fetch
// and web_search). Exposed so card and project chat can offer them
// too — the underlying Go handlers (agent.WebFetch / agent.WebSearch)
// are shared. http_request is intentionally excluded from chat: it's
// arbitrary HTTP with any method, which is a bigger surface than
// casual chat should default to granting.
func WebTools() []ToolDef {
	out := make([]ToolDef, 0, 2)
	for _, t := range allAgentTools {
		if t.Name == "web_fetch" || t.Name == "web_search" {
			out = append(out, t)
		}
	}
	return out
}

// AgentBuiltinTools returns the agent-only built-ins: tools about the
// agent's own run (update_self, notify) and the open web. Every board
// tool (get_card, create_card, update_card, search_cards, …) is BRUV's
// native tool set, added by the agent runtime — the same tools the MCP
// server and chat use.
func AgentBuiltinTools() []ToolDef {
	return append([]ToolDef(nil), allAgentTools...)
}

// legacyAgentToolIDs maps allowed_tools ids from before agents used the
// native tool set onto the native names, so saved agents keep working.
var legacyAgentToolIDs = map[string]string{"read_card": "get_card"}

// CanonicalAgentToolID returns the current id for a (possibly legacy)
// allowed_tools entry.
func CanonicalAgentToolID(id string) string {
	if current, ok := legacyAgentToolIDs[id]; ok {
		return current
	}
	return id
}

// agentExcludedTools are native tools an agent may never be granted:
// letting a run start or reconfigure agents (including itself) would let
// one agent spend without limit. They stay available to chat and MCP,
// where a person is driving.
var agentExcludedTools = map[string]bool{"configure_card_agent": true, "run_card_agent": true}

// AgentMayUse reports whether an agent can be granted tool name at all.
func AgentMayUse(name string) bool { return !agentExcludedTools[CanonicalAgentToolID(name)] }

// AgentTools returns the agent-only built-ins allowed by allowedTools
// (legacy ids accepted). An empty list allows none: an agent gets only
// the tools it was explicitly granted, which is what the Agent tab shows.
func AgentTools(allowedTools []string) []ToolDef {
	if len(allowedTools) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowed[CanonicalAgentToolID(t)] = true
	}
	var filtered []ToolDef
	for _, tool := range allAgentTools {
		if allowed[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}
