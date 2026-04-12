package llm

// allAgentTools defines the full set of agent-specific tools.
var allAgentTools = []ToolDef{
	{
		Name:        "web_fetch",
		Description: "Fetch a web page and return its text content. Use this to read specific URLs.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
			},
			"required": []string{"url"},
		},
	},
	{
		Name:        "web_search",
		Description: "Search the web and return results. Returns titles, URLs, and snippets for the top results.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
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
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
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
			},
		},
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
		Name:        "create_card",
		Description: "Create a new card in the repository.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "The card title",
				},
				"card_type": map[string]any{
					"type":        "string",
					"description": "The card type (e.g. 'brainstorm', 'task'). Optional.",
				},
			},
			"required": []string{"title"},
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
