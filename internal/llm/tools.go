package llm

import "strings"

// builtinAgentToolNames are the agent-only built-ins (see AgentBuiltinTools).
var builtinAgentToolNames = []string{"web_fetch", "web_search", "notify", "update_self", "http_request"}

// BuiltinAgentToolNames returns a copy of the built-in tool names an
// agent's allowed_tools list may contain.
func BuiltinAgentToolNames() []string {
	return append([]string(nil), builtinAgentToolNames...)
}

// AddFieldTypes is the block-type vocabulary the add_field tool accepts.
// Mirrors the model.Block* constants for the types an LLM can sensibly
// create from a chat request; richer blocks (media, slide decks, surveys)
// need configuration the tool doesn't carry.
var AddFieldTypes = []string{"text", "list", "checklist", "checkbox", "number", "date", "url"}

// addFieldTypeEnum renders AddFieldTypes as the []any a JSON-schema enum
// expects.
func addFieldTypeEnum() []any {
	out := make([]any, len(AddFieldTypes))
	for i, t := range AddFieldTypes {
		out[i] = t
	}
	return out
}

// CardTools returns card chat's own tool definitions: the ones that act on
// the open card through its field schema (set_fields, add_field) and
// suggest_pin. The native board tools are added alongside by the chat runtime.
func CardTools(categories []map[string]string) []ToolDef {
	// Build enum for card types
	// Build enum for category IDs + descriptions for the LLM
	catIDs := make([]any, len(categories))
	for i, c := range categories {
		catIDs[i] = c["id"]
	}

	// Card chat's own tools act on THIS card with its field schema. Every
	// other board tool (title, description, type, due date, tags, agent…)
	// comes from the native registry, appended by the chat runtime.
	tools := []ToolDef{
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
	}

	// add_field: lets the LLM append new blocks to a card beyond its schema.
	// The allowed types live in AddFieldTypes so the dispatcher validates
	// against the same set the schema advertises.
	tools = append(tools, ToolDef{
		Name:        "add_field",
		Description: "Add a new field to the card. Use this when the user asks for a field that does not already exist (e.g. a checklist, extra notes, a checkbox). If the user described what should go in the field, ALWAYS pass the `value` parameter in this same call — never split into add_field followed by set_fields to fill it in, that leaves the field empty if you forget the follow-up. Only omit `value` if the user explicitly wants an empty field to fill in themselves.",
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
					"enum":        addFieldTypeEnum(),
					"description": "The type of field to add. Use 'text' for freeform text, 'list' for plain bullet points (dot points, no checkboxes), 'checklist' for a list of items with checkboxes, 'checkbox' for a boolean toggle, 'number' for numeric values, 'date' for dates, 'url' for links.",
				},
				"value": map[string]any{
					"description": "Initial value for the field. REQUIRED when the user described what should go in the field — do not defer it to a follow-up call. For text: a string. For list and checklist: an array of strings. For checkbox: a boolean. For number: a number. For date: a YYYY-MM-DD string. Only omit if the user explicitly wants an empty field.",
				},
			},
			"required": []string{"key", "label", "field_type"},
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
		Description: "File this Inbox card on a board (it is offered only while the card has no location). STRONGLY prefer category_id from the existing categories list. A category listed with [accepts: …] takes only those card types: if it is the best location, call set_card_type with the accepted type that best describes the card first (same response), then pin here; only pick another category if none of its accepted types fits the card. Only provide brand/stream/project/category names to create a new location if no existing category is appropriate.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": pinProps,
			"required":   pinRequired,
		},
	})

	// Web browsing — shared with agents. The handlers in app.go
	// delegate to agent.WebFetch / agent.WebSearch.
	tools = append(tools, WebTools()...)

	return tools
}

// cardTypeDesc describes a card_type parameter. Deliberately NOT an
// enum: an unknown name is CREATED as a new user card type by the
// dispatcher (ruling 2026-08-14), and a hard enum would forbid exactly
// that. Existing ids are listed so the model matches before inventing.
func cardTypeDesc(cardTypes []string) string {
	if len(cardTypes) == 0 {
		return "The card type: an existing type id, or a new short descriptive name to create one."
	}
	return "The card type. Existing type ids: " + strings.Join(cardTypes, ", ") +
		". Pass one of these (labels are matched too), or a new short descriptive name to create a new type."
}

// ProjectTools returns the tool definitions for project-level AI chat.
// The LLM can create cards, bulk-tag, move cards between categories, etc.
func ProjectTools(cardTypes []string, categories []map[string]string) []ToolDef {
	// accepted_types keeps a hard enum — restricting a category only makes
	// sense against EXISTING types. card_type params deliberately don't:
	// an unknown type name is CREATED by the dispatcher (ruling 2026-08-14),
	// and an enum would forbid exactly that.
	typeIDs := make([]any, len(cardTypes))
	for i, t := range cardTypes {
		typeIDs[i] = t
	}

	catIDs := make([]any, len(categories))
	for i, c := range categories {
		catIDs[i] = c["id"]
	}

	tools := []ToolDef{
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
			Name:        "update_cards",
			Description: "Update many cards in a single call. Each entry is a partial update for one card. All fields per entry are optional except `card_id`. Prefer this over many `update_card` calls when editing several cards at once.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"updates": map[string]any{
						"type":        "array",
						"description": "List of per-card updates",
						"items":       cardUpdateParameters(cardTypes, true),
					},
				},
				"required": []string{"updates"},
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
						"items":       map[string]any{"type": "string", "enum": typeIDs},
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

	// Web browsing — shared with card chat and agents. The handlers
	// in app.go delegate to agent.WebFetch / agent.WebSearch.
	tools = append(tools, WebTools()...)

	return tools
}

// cardUpdateParameters returns the JSON-schema parameter shape for a card
// update operation. Used by both `update_card` (single) and `update_cards`
// (plural) so the field set stays in sync.
//
// When `forArrayItem` is true, the schema doesn't carry the outer "type:object"
// wrapper at the top level — the caller embeds this inside an `items` field.
func cardUpdateParameters(cardTypes []string, forArrayItem bool) map[string]any {
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
			"description": "New card type (optional). " + cardTypeDesc(cardTypes),
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

// WithoutTool returns defs minus the named tool — how card chat drops
// suggest_pin for a card that is already filed. Order is preserved.
func WithoutTool(defs []ToolDef, name string) []ToolDef {
	out := make([]ToolDef, 0, len(defs))
	for _, d := range defs {
		if d.Name != name {
			out = append(out, d)
		}
	}
	return out
}
