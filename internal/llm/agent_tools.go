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
		Parameters: cardUpdateParams(false),
	},
	{
		Name: "update_card",
		Description: "Update ANOTHER card by id — e.g. one you created earlier, found with search_cards or list_cards — " +
			"when its details change. Same fields and value formats as update_self (read it first with read_card to see its blocks). " +
			"Use update_self for this agent's own card.",
		Parameters: cardUpdateParams(true),
	},
	{
		Name:        "read_card",
		Description: "Read another card's content. Returns the card's title, type, tags, and all block content.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"card_id": map[string]any{
					"type":        "string",
					"description": "The ID of the card to read",
				},
			},
			"required": []string{"card_id"},
		},
	},
	{
		Name: "create_card",
		Description: "Create and populate a new card. Call search_cards first so you don't create a duplicate. " +
			"To file it on a board give ALL of brand, stream, project and category (missing levels are created); " +
			"omit all four to leave it unfiled in the inbox. Use list_cards to see a board's existing categories.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string", "description": "The card title"},
				"card_type":   map[string]any{"type": "string", "description": "Card type id or label (e.g. 'task', 'reference'), matched case-insensitively; an unrecognised name creates a new type. Default 'brainstorm'."},
				"description": map[string]any{"type": "string", "description": "Free-text summary under the title (Markdown)."},
				"due_date":    map[string]any{"type": "string", "description": "Due date, YYYY-MM-DD."},
				"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for the card."},
				"brand":       map[string]any{"type": "string", "description": "Brand to file under (name or slug)."},
				"stream":      map[string]any{"type": "string", "description": "Stream to file under."},
				"project":     map[string]any{"type": "string", "description": "Project to file under."},
				"category":    map[string]any{"type": "string", "description": "Category (board column) to file into."},
				"blocks": map[string]any{
					"type":        "array",
					"description": "Content blocks to add to the card.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":   map[string]any{"type": "string", "description": "'text', 'list', 'checklist', 'url', 'number', 'date' or 'checkbox'."},
							"label":  map[string]any{"type": "string", "description": "Block label, e.g. 'Notes'."},
							"value":  map[string]any{"description": "String for text/url/date; array of strings for list/checklist; number; boolean."},
							"format": map[string]any{"type": "string", "enum": []any{"date", "date-time"}, "description": BlockFormatDesc},
						},
						"required": []string{"type", "value"},
					},
				},
			},
			"required": []string{"title"},
		},
	},
	{
		Name:        "search_cards",
		Description: "Full-text search every card's title and content. Returns id, title, type and board location for each match. Use it to check whether a card already exists before creating one, or to find cards to read.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search words; each matches as a prefix."},
				"limit": map[string]any{"type": "integer", "description": "Max results (default 20)."},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "list_cards",
		Description: "List the cards on a project board, grouped by category in board order (id, title, type, due date, tags). Pass category to list one column only. Use read_card for a card's content.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"brand":    map[string]any{"type": "string", "description": "Brand name or slug."},
				"stream":   map[string]any{"type": "string", "description": "Stream name or slug."},
				"project":  map[string]any{"type": "string", "description": "Project name or slug."},
				"category": map[string]any{"type": "string", "description": "Optional: only this category."},
			},
			"required": []string{"brand", "stream", "project"},
		},
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

// cardUpdateParams is the schema shared by update_self and update_card;
// withCardID adds the target card for update_card.
func cardUpdateParams(withCardID bool) map[string]any {
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

// AgentTools returns the tool definitions for an agent, filtered by the allowed list.
// If allowedTools is empty, all tools are returned.
func AgentTools(allowedTools []string) []ToolDef {
	if len(allowedTools) == 0 {
		return allAgentTools
	}

	allowed := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		allowed[t] = true
	}

	var filtered []ToolDef
	for _, tool := range allAgentTools {
		if allowed[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}
