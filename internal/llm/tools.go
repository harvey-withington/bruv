package llm

// CardTools returns the tool definitions available for card classification.
func CardTools(cardTypes []string, categories []map[string]string) []ToolDef {
	// Build enum for card types
	typeEnum := make([]any, len(cardTypes))
	for i, t := range cardTypes {
		typeEnum[i] = t
	}

	// Build enum for category IDs + descriptions for the LLM
	catIDs := make([]any, len(categories))
	for i, c := range categories {
		catIDs[i] = c["id"]
	}

	tools := []ToolDef{
		{
			Name:        "set_title",
			Description: "Set or update the card's title.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "The new title for the card",
					},
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "set_due_date",
			Description: "Set or clear the card's due date. Use ISO 8601 format (YYYY-MM-DD). Pass an empty string to clear the due date.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"due_date": map[string]any{
						"type":        "string",
						"description": "Due date in YYYY-MM-DD format, or empty string to clear",
					},
				},
				"required": []string{"due_date"},
			},
		},
		{
			Name:        "set_card_type",
			Description: "Set the card's type. This creates empty fields for that type which you MUST then fill using set_fields.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_type": map[string]any{
						"type":        "string",
						"enum":        typeEnum,
						"description": "The card type to set",
					},
				},
				"required": []string{"card_type"},
			},
		},
		{
			Name:        "set_fields",
			Description: "Fill in one or more field values on the card. Each entry maps a field key (like 'description', 'priority', 'notes') to its new value. ALWAYS call this after set_card_type to populate the fields with real content.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fields": map[string]any{
						"type":        "object",
						"description": "Map of field key to new value. Use strings for text/select fields, numbers for number fields, booleans for checkbox fields.",
					},
				},
				"required": []string{"fields"},
			},
		},
		{
			Name:        "add_tags",
			Description: "Add tags to the card. Prefer existing project tags when available, but create short descriptive tags if needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of tags to add to the card",
					},
				},
				"required": []string{"tags"},
			},
		},
	}

	// add_field: lets the LLM append new blocks to a card beyond its schema
	tools = append(tools, ToolDef{
		Name:        "add_field",
		Description: "Add a new field to the card. Use this when the user asks for a field that does not already exist (e.g. a checklist, extra notes, a checkbox). The field is appended after existing fields. After adding, use set_fields to populate it.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Machine-friendly key for the field, e.g. 'characters', 'todo', 'links'. Must be lowercase with underscores, no spaces.",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Human-readable label, e.g. 'Characters', 'To-Do List', 'Reference Links'.",
				},
				"field_type": map[string]any{
					"type":        "string",
					"enum":        []any{"text", "checklist", "checkbox", "number", "date", "url"},
					"description": "The type of field to add. Use 'text' for freeform text, 'checklist' for a list of items with checkboxes, 'checkbox' for a boolean toggle, 'number' for numeric values, 'date' for dates, 'url' for links.",
				},
				"value": map[string]any{
					"description": "Initial value for the field. For text: a string. For checklist: an array of strings. For checkbox: a boolean. For number: a number. May be omitted to leave empty.",
				},
			},
			"required": []string{"key", "label", "field_type"},
		},
	})

	// configure_agent: set up or modify the card's autonomous agent
	tools = append(tools, ToolDef{
		Name:        "configure_agent",
		Description: "Configure the card's autonomous agent. The agent runs on a schedule and can perform tasks like fetching web pages, searching the web, sending notifications, and updating this card. Set enabled to true and provide a goal and schedule to activate it.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled": map[string]any{
					"type":        "boolean",
					"description": "Whether the agent is active",
				},
				"goal": map[string]any{
					"type":        "string",
					"description": "What the agent should do each run. Be specific — this is the agent's instruction.",
				},
				"schedule": map[string]any{
					"type":        "string",
					"description": "How often to run. Use: '@hourly', '@daily', '@weekly', '30m', '1h', or a cron expression like '0 9 * * *' (daily at 9am).",
				},
				"allowed_tools": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "enum": []string{"web_fetch", "web_search", "notify", "update_self", "create_card", "read_card", "http_request"}},
					"description": "Which tools the agent can use. Common sets: ['web_fetch', 'web_search', 'notify', 'update_self'] for monitoring tasks.",
				},
				"notify_on": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": []string{"success", "failure"}},
					"description": "When to send notifications. Use ['success', 'failure'] for most agents.",
				},
				"notify_channel": map[string]any{
					"type":        "string",
					"description": "Notification channels as comma-separated string. Options: 'system', 'email', 'webhook'. In-app is always included automatically.",
				},
				"next_run_at": map[string]any{
					"type":        "string",
					"description": "ISO 8601 datetime to schedule the next run at a specific time. Use this to dynamically reschedule the agent.",
				},
				"new_schedule": map[string]any{
					"type":        "string",
					"description": "Cron expression or interval to change the agent's schedule (e.g. '@daily', '30m', '0 9 * * *').",
				},
			},
			"required": []string{"enabled", "goal", "schedule", "allowed_tools"},
		},
	})

	// suggest_pin: always available — can pin to existing category or create new hierarchy
	pinProps := map[string]any{
		"reason": map[string]any{
			"type":        "string",
			"description": "Brief explanation of why this location is a good fit",
		},
	}
	pinRequired := []string{"reason"}

	if len(categories) > 0 {
		pinProps["category_id"] = map[string]any{
			"type":        "string",
			"enum":        catIDs,
			"description": "ID of an existing category to pin to. Use this when a suitable category already exists.",
		}
	}

	// Hierarchy fields for creating new locations
	pinProps["brand"] = map[string]any{
		"type":        "string",
		"description": "Brand name. Uses existing brand if name matches, otherwise creates a new one. Only provide if category_id is not set.",
	}
	pinProps["stream"] = map[string]any{
		"type":        "string",
		"description": "Stream name within the brand. Uses existing if name matches, otherwise creates new. Only provide if category_id is not set.",
	}
	pinProps["project"] = map[string]any{
		"type":        "string",
		"description": "Project name within the stream. Uses existing if name matches, otherwise creates new. Only provide if category_id is not set.",
	}
	pinProps["category"] = map[string]any{
		"type":        "string",
		"description": "Category name within the project. Uses existing if name matches, otherwise creates new. Only provide if category_id is not set.",
	}

	tools = append(tools, ToolDef{
		Name:        "suggest_pin",
		Description: "Pin the card to a location. STRONGLY prefer using category_id from the existing categories list. Only provide brand/stream/project/category names to create a new location if no existing category is appropriate.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": pinProps,
			"required":   pinRequired,
		},
	})

	return tools
}

// ProjectTools returns the tool definitions for project-level AI chat.
// The LLM can create cards, bulk-tag, move cards between categories, etc.
func ProjectTools(cardTypes []string, categories []map[string]string) []ToolDef {
	typeEnum := make([]any, len(cardTypes))
	for i, t := range cardTypes {
		typeEnum[i] = t
	}

	catIDs := make([]any, len(categories))
	for i, c := range categories {
		catIDs[i] = c["id"]
	}

	tools := []ToolDef{
		{
			Name:        "create_card",
			Description: "Create a new card and optionally pin it to a category in this project. Returns the card ID. After creation, the user can open it to continue editing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "Card title",
					},
					"card_type": map[string]any{
						"type":        "string",
						"enum":        typeEnum,
						"description": "Card type (optional)",
					},
					"category_id": map[string]any{
						"type":        "string",
						"enum":        catIDs,
						"description": "Category to pin the card to (optional, from categories in this project)",
					},
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Tags to add to the card (optional)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Initial description text for the card's first text block (optional)",
					},
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "add_tags_to_cards",
			Description: "Add tags to one or more cards by their ID. Use this for bulk-tagging based on criteria.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_ids": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of card IDs to add tags to",
					},
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Tags to add",
					},
				},
				"required": []string{"card_ids", "tags"},
			},
		},
		{
			Name:        "move_card",
			Description: "Move a card from one category to another within this project.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_id": map[string]any{
						"type":        "string",
						"description": "ID of the card to move",
					},
					"from_category_id": map[string]any{
						"type":        "string",
						"enum":        catIDs,
						"description": "Current category",
					},
					"to_category_id": map[string]any{
						"type":        "string",
						"enum":        catIDs,
						"description": "Destination category",
					},
				},
				"required": []string{"card_id", "from_category_id", "to_category_id"},
			},
		},
		{
			Name:        "update_card",
			Description: "Update a card's title, type, or tags.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_id": map[string]any{
						"type":        "string",
						"description": "ID of the card to update",
					},
					"title": map[string]any{
						"type":        "string",
						"description": "New title (optional)",
					},
					"card_type": map[string]any{
						"type":        "string",
						"enum":        typeEnum,
						"description": "New card type (optional)",
					},
					"tags": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Tags to add (appended to existing, optional)",
					},
				},
				"required": []string{"card_id"},
			},
		},
	}

	return tools
}
