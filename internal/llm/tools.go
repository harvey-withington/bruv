package llm

// builtinAgentToolNames is the static list of tool names an agent can be
// granted via configure_agent. MCP tools are appended dynamically at call
// time by buildAllowedToolsEnum.
var builtinAgentToolNames = []string{
	"web_fetch", "web_search", "notify", "update_self",
	"create_card", "read_card", "http_request",
}

// buildAllowedToolsEnum merges the static built-in tool names with any
// MCP tool IDs the repo currently has, producing the enum array for the
// configure_agent tool's allowed_tools parameter. This lets the LLM
// include MCP tools when configuring an agent from the card chat.
func buildAllowedToolsEnum(mcpToolIDs []string) []string {
	out := make([]string, len(builtinAgentToolNames), len(builtinAgentToolNames)+len(mcpToolIDs))
	copy(out, builtinAgentToolNames)
	out = append(out, mcpToolIDs...)
	return out
}

// CardTools returns the tool definitions available for card-level AI chat.
// mcpToolIDs is the list of namespaced MCP tool IDs (e.g. "filesystem__read_text_file")
// currently available via the repo's MCP registry. These are appended to the
// configure_agent tool's allowed_tools enum so the LLM can include them when
// setting up or modifying an agent's tool permissions. Pass nil if no MCP
// registry is active.
func CardTools(cardTypes []string, categories []map[string]string, mcpToolIDs []string) []ToolDef {
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
					"items": map[string]any{"type": "string", "enum": buildAllowedToolsEnum(mcpToolIDs)},
					"description": "Which tools the agent can use. Built-in tools: web_fetch, web_search, notify, update_self, create_card, read_card, http_request. MCP tools use a server__tool prefix (e.g. filesystem__read_text_file). Include MCP tools when the goal requires external capabilities like filesystem access.",
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
						"description": "ID of the category to pin the card to (optional). Use the IDs listed in the system prompt.",
					},
					"category_name": map[string]any{
						"type":        "string",
						"description": "Name of the category to pin to (optional). Use this instead of `category_id` when referring to a category you've just created in the same conversation — its ID won't be known yet.",
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
			Description: "Move a card to a different category within this project. Identify the destination by `to_category_id` (preferred) or `to_category_name` (use this when moving into a category you just created in the same conversation — its ID won't be known yet). The source is auto-detected from the card's current pin and does not need to be supplied.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_id": map[string]any{
						"type":        "string",
						"description": "ID of the card to move",
					},
					"to_category_id": map[string]any{
						"type":        "string",
						"description": "ID of the destination category (preferred)",
					},
					"to_category_name": map[string]any{
						"type":        "string",
						"description": "Name of the destination category — fallback for when `to_category_id` is unknown (e.g. you just staged its creation)",
					},
					"from_category_id": map[string]any{
						"type":        "string",
						"description": "Optional. Source category ID. If omitted, the card's current category in this project is used.",
					},
				},
				"required": []string{"card_id"},
			},
		},
		{
			Name:        "update_card",
			Description: "Update one card's title, type, tags, due date, description, or blocks. All fields are optional — only the supplied ones change. Use `update_cards` instead when changing multiple cards.",
			Parameters:  cardUpdateParameters(typeEnum, false),
		},
		{
			Name:        "update_cards",
			Description: "Update many cards in a single call. Each entry is a partial update for one card. All fields per entry are optional except `card_id`. Prefer this over many `update_card` calls when editing several cards at once.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"updates": map[string]any{
						"type":        "array",
						"description": "List of per-card updates",
						"items":       cardUpdateParameters(typeEnum, true),
					},
				},
				"required": []string{"updates"},
			},
		},
		{
			Name:        "configure_agent",
			Description: "Configure or update the agent attached to a card. Sets schedule (cron), goal, enabled state, or tool whitelist. Omit any field to leave it unchanged.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"card_id": map[string]any{
						"type":        "string",
						"description": "ID of the card whose agent to configure",
					},
					"enabled": map[string]any{
						"type":        "boolean",
						"description": "Enable or disable the agent (optional)",
					},
					"schedule": map[string]any{
						"type":        "string",
						"description": "Cron expression or shorthand (`@hourly`, `@daily`, `@weekly`). Empty string clears the schedule. (optional)",
					},
					"goal": map[string]any{
						"type":        "string",
						"description": "Plain-language description of what the agent should do on each run (optional)",
					},
					"allowed_tools": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Whitelist of tool names the agent may use (optional)",
					},
				},
				"required": []string{"card_id"},
			},
		},

		// --- Project metadata ---
		{
			Name:        "update_project",
			Description: "Update the current project's name, description, or icon. All fields optional — only the supplied ones change. Cannot change which brand/stream the project belongs to.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "New project name (optional)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "New project description (optional)",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "Icon identifier — Lucide icon name, or a `data:image/...;base64,...` data URL for a custom image, optionally prefixed with `c:#rrggbb:` to colorize. Empty string clears the icon. (optional)",
					},
				},
			},
		},

		// --- Project tags (the project's tag vocabulary) ---
		{
			Name:        "create_project_tag",
			Description: "Define a new tag in this project's tag vocabulary. The name is also the string used on cards. Color is auto-assigned from the palette unless specified.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Tag name (also the string used on cards)",
					},
					"color": map[string]any{
						"type":        "string",
						"description": "Hex color (e.g. `#61bd4f`). Optional — auto-assigned from palette if omitted.",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "Icon identifier (Lucide name or data URL). Optional.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "update_project_tag",
			Description: "Rename a tag, change its color, or set its icon. Identify the tag by either `tag_id` (preferred) or `tag_name`. All update fields optional.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tag_id": map[string]any{
						"type":        "string",
						"description": "ID of the tag to update (preferred)",
					},
					"tag_name": map[string]any{
						"type":        "string",
						"description": "Name of the tag to update — used as a fallback when `tag_id` is unknown",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "New name (optional). Renaming a tag here does NOT rename the tag string on existing cards.",
					},
					"color": map[string]any{
						"type":        "string",
						"description": "New hex color (optional)",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "New icon, or empty string to clear (optional)",
					},
				},
			},
		},
		{
			Name:        "delete_project_tag",
			Description: "Delete a tag from this project's tag vocabulary. Use this for removing unused tags. Identify by `tag_id` (preferred) or `tag_name`. This does NOT remove the tag string from any cards still using it — use `update_cards` with `tags_to_remove` first if needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tag_id": map[string]any{
						"type":        "string",
						"description": "ID of the tag to delete (preferred)",
					},
					"tag_name": map[string]any{
						"type":        "string",
						"description": "Name of the tag to delete — used as a fallback when `tag_id` is unknown",
					},
				},
			},
		},

		// --- Categories ---
		{
			Name:        "create_category",
			Description: "Create a new category (column) in the project. Position defaults to the end. After creation, you can immediately use `update_category` to set description, icon, or accepted_types.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Category name",
					},
					"position": map[string]any{
						"type":        "integer",
						"description": "Zero-based position (optional, defaults to end)",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "update_category",
			Description: "Update a category's name, description, icon, or accepted card types. Identify by `category_id` (preferred) or `category_name`. All update fields optional.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category_id": map[string]any{
						"type":        "string",
						"description": "ID of the category to update (preferred)",
					},
					"category_name": map[string]any{
						"type":        "string",
						"description": "Name of the category — fallback for when `category_id` is unknown (e.g. you just staged its creation)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "New name (optional)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "New description (optional)",
					},
					"icon": map[string]any{
						"type":        "string",
						"description": "New icon, or empty string to clear (optional)",
					},
					"accepted_types": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": typeEnum},
						"description": "Restrict the category to these card types. Empty array means accept all types. (optional)",
					},
				},
			},
		},
		{
			Name:        "delete_category",
			Description: "Delete a category. Cards pinned to this category will be unpinned (moved to the inbox). The project must have at least one category remaining. Identify by `category_id` (preferred) or `category_name`.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category_id": map[string]any{
						"type":        "string",
						"description": "ID of the category to delete (preferred)",
					},
					"category_name": map[string]any{
						"type":        "string",
						"description": "Name of the category — fallback for when `category_id` is unknown",
					},
				},
			},
		},
	}

	return tools
}

// cardUpdateParameters returns the JSON-schema parameter shape for a card
// update operation. Used by both `update_card` (single) and `update_cards`
// (plural) so the field set stays in sync.
//
// When `forArrayItem` is true, the schema doesn't carry the outer "type:object"
// wrapper at the top level — the caller embeds this inside an `items` field.
func cardUpdateParameters(typeEnum []any, forArrayItem bool) map[string]any {
	props := map[string]any{
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
			"description": "Replace the card's tags with this list (optional). Use `tags_to_add` instead to append.",
		},
		"tags_to_add": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Tags to append to the card's existing tags (optional)",
		},
		"tags_to_remove": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Tags to remove from the card's existing tags (optional). Use this when the user asks to remove specific tags — do NOT clear all tags by passing an empty `tags` array unless they explicitly ask for that.",
		},
		"due_date": map[string]any{
			"type":        "string",
			"description": "ISO 8601 date or datetime (e.g. `2026-04-15` or `2026-04-15T18:00:00Z`). Empty string clears the due date. (optional)",
		},
		"description": map[string]any{
			"type":        "string",
			"description": "Replace the card's description (the first text block, or the `description` field). (optional)",
		},
		"blocks": map[string]any{
			"type":        "array",
			"description": "Replace the card's blocks entirely. Each block is `{type, label, value, key?}`. Use only when restructuring the card; for simple text edits use `description`. (optional)",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":  map[string]any{"type": "string"},
					"label": map[string]any{"type": "string"},
					"value": map[string]any{},
					"key":   map[string]any{"type": "string"},
				},
				"required": []string{"type", "value"},
			},
		},
	}

	schema := map[string]any{
		"type":       "object",
		"properties": props,
		"required":   []string{"card_id"},
	}
	_ = forArrayItem // both call sites use the same shape; param kept for future divergence
	return schema
}
