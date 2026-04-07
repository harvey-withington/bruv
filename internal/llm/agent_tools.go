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
		Name:        "update_self",
		Description: "Update this card's content blocks. Use this to record findings, update status, or modify the card's own data.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"updates": map[string]any{
					"type":        "array",
					"description": "List of block updates",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"key": map[string]any{
								"type":        "string",
								"description": "The block key to update (e.g. 'description', 'notes')",
							},
							"value": map[string]any{
								"type":        "string",
								"description": "The new value for the block",
							},
						},
						"required": []string{"key", "value"},
					},
				},
			},
			"required": []string{"updates"},
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
