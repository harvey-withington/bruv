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
