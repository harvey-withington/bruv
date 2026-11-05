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

// cardTypeDesc describes a card_type parameter. Not an enum, so a type
// made with create_card_type earlier in the same turn is still accepted;
// an unknown name is refused (ruling 2026-09-30), never created.
func cardTypeDesc(cardTypes []string) string {
	if len(cardTypes) == 0 {
		return "The card type: an existing type id (see list_card_types). Unknown types are refused."
	}
	return "The card type. Existing type ids: " + strings.Join(cardTypes, ", ") +
		". Pass one of these (labels are matched too); unknown types are refused."
}

// ProjectTools returns project chat's own tool definitions: the batch
// edits. Everything else it does (moving cards, the project, its tags and
// categories) is a native board tool, added alongside by the chat runtime.
func ProjectTools(cardTypes []string) []ToolDef {
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
			Name:        "update_cards",
			Description: "Update many cards in a single call. Each entry is a partial update for one card. All fields per entry are optional except `card_id`. Prefer this over many `update_card` calls when editing several cards at once. The result lists what changed and what failed per card.",
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
			"description": "Set or add fields. Each block is `{type, label, value, key?}`: one matching an existing field by key (or label) sets that field's value, shaped to the field and keeping its settings; any other is added as a new field. Nothing is ever deleted. For the description use `description`. (optional)",
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
