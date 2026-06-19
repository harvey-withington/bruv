package mcpserver

import (
	"encoding/json"

	"bruv/core/supervisor"
	"bruv/internal/mcp"
)

// toolFunc is one tool implementation. It returns the text to surface to
// the model and whether that text represents an error (mapped to the
// MCP result's isError flag). Tool-level failures flow back to the model
// as text rather than JSON-RPC errors so it can recover.
type toolFunc func(rt *supervisor.Runtime, args map[string]any) (string, bool)

// toolHandlers maps tool name → implementation. Definitions advertised
// to the client live in toolDefs; the two must stay in sync.
var toolHandlers = map[string]toolFunc{
	// Discovery / read
	"list_brands":     hListBrands,
	"list_streams":    hListStreams,
	"list_projects":   hListProjects,
	"list_categories": hListCategories,
	"list_card_types": hListCardTypes,
	"get_card":        hGetCard,
	"search_cards":    hSearchCards,
	// Create / capture
	"create_brand":    hCreateBrand,
	"create_stream":   hCreateStream,
	"create_project":  hCreateProject,
	"create_category": hCreateCategory,
	"create_card":     hCreateCard,
	// Populate existing cards
	"add_card_blocks": hAddCardBlocks,
	"set_card_fields": hSetCardFields,
	"add_card_tags":   hAddCardTags,
}

// callTool executes a tools/call request and wraps the result in an MCP
// CallToolResult. Bad params or unknown tools surface as isError text so
// the model can adjust rather than seeing a transport error.
func callTool(rt *supervisor.Runtime, params json.RawMessage) mcp.CallToolResult {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return textResult("invalid tools/call params: "+err.Error(), true)
		}
	}
	fn, ok := toolHandlers[p.Name]
	if !ok {
		return textResult("unknown tool: "+p.Name, true)
	}
	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}
	text, isErr := fn(rt, p.Arguments)
	return textResult(text, isErr)
}

func textResult(text string, isErr bool) mcp.CallToolResult {
	return mcp.CallToolResult{
		Content: []mcp.Content{{Type: "text", Text: text}},
		IsError: isErr,
	}
}

// --- Tool definitions (advertised via tools/list) ---

// schema-builder shorthands keep the definitions readable.
func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if required == nil {
		required = []string{}
	}
	m["required"] = required
	return m
}
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func strArr(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// blockArrayProp is the shared schema for a list of card blocks. The
// shape matches BRUV's internal block model: {type, label, value, key?}.
func blockArrayProp(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items": obj(map[string]any{
			"type": map[string]any{
				"type": "string",
				"description": "Block type. Common: 'text' (freeform), 'checklist' (array of strings), " +
					"'list' (array of strings), 'url', 'number', 'date' (YYYY-MM-DD), 'checkbox' (boolean).",
			},
			"label": strProp("Human-readable label for the block, e.g. 'Notes', 'To-Do'."),
			"value": map[string]any{"description": "The block's content. String for text/url/date; array of strings for checklist/list; boolean for checkbox; number for number."},
			"key":   strProp("Optional machine key (lowercase_with_underscores). Omit for freeform blocks."),
		}, "type", "value"),
	}
}

// toolDefs returns the tool list, templating the repo name into the
// descriptions so a multi-connector user sees which board each tool
// writes to.
func toolDefs(repoName string) []mcp.Tool {
	board := "the \"" + repoName + "\" BRUV board"

	return []mcp.Tool{
		// --- Discovery / read ---
		{
			Name:        "list_brands",
			Description: "List the Brands in " + board + ". A Brand is the top-level container in the Brand → Stream → Project → Category → Card hierarchy.",
			InputSchema: obj(map[string]any{}),
		},
		{
			Name:        "list_streams",
			Description: "List the Streams under a Brand in " + board + ".",
			InputSchema: obj(map[string]any{
				"brand": strProp("Brand name or slug."),
			}, "brand"),
		},
		{
			Name:        "list_projects",
			Description: "List the Projects under a Stream in " + board + ".",
			InputSchema: obj(map[string]any{
				"brand":  strProp("Brand name or slug."),
				"stream": strProp("Stream name or slug."),
			}, "brand", "stream"),
		},
		{
			Name:        "list_categories",
			Description: "List the Categories (columns) in a Project in " + board + ". Cards are filed into Categories.",
			InputSchema: obj(map[string]any{
				"brand":   strProp("Brand name or slug."),
				"stream":  strProp("Stream name or slug."),
				"project": strProp("Project name or slug."),
			}, "brand", "stream", "project"),
		},
		{
			Name:        "list_card_types",
			Description: "List the available card types in " + board + " (e.g. idea, task, note). Use one of these as `card_type` when creating a card.",
			InputSchema: obj(map[string]any{}),
		},
		{
			Name:        "get_card",
			Description: "Fetch a single card from " + board + " by id, including its blocks, tags and type.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
			}, "card_id"),
		},
		{
			Name:        "search_cards",
			Description: "Full-text search cards in " + board + ". Use this to check whether an idea already exists before creating a duplicate.",
			InputSchema: obj(map[string]any{
				"query": strProp("Search query."),
				"limit": intProp("Max results (default 20)."),
			}, "query"),
		},

		// --- Create / capture ---
		{
			Name:        "create_brand",
			Description: "Create a new Brand in " + board + ". Returns the created brand.",
			InputSchema: obj(map[string]any{
				"name":        strProp("Brand name."),
				"description": strProp("Optional description."),
			}, "name"),
		},
		{
			Name:        "create_stream",
			Description: "Create a Stream under a Brand in " + board + ". The Brand is created automatically if it doesn't exist.",
			InputSchema: obj(map[string]any{
				"brand":       strProp("Brand name or slug (created if missing)."),
				"name":        strProp("Stream name."),
				"description": strProp("Optional description."),
			}, "brand", "name"),
		},
		{
			Name:        "create_project",
			Description: "Create a Project under a Stream in " + board + ". Missing Brand/Stream are created automatically.",
			InputSchema: obj(map[string]any{
				"brand":       strProp("Brand name or slug (created if missing)."),
				"stream":      strProp("Stream name or slug (created if missing)."),
				"name":        strProp("Project name."),
				"description": strProp("Optional description."),
			}, "brand", "stream", "name"),
		},
		{
			Name:        "create_category",
			Description: "Create a Category (column) in a Project in " + board + ". Missing Brand/Stream/Project are created automatically.",
			InputSchema: obj(map[string]any{
				"brand":    strProp("Brand name or slug (created if missing)."),
				"stream":   strProp("Stream name or slug (created if missing)."),
				"project":  strProp("Project name or slug (created if missing)."),
				"name":     strProp("Category name."),
				"position": intProp("Optional zero-based position (defaults to the end)."),
			}, "brand", "stream", "project", "name"),
		},
		{
			Name: "create_card",
			Description: "Create and populate a card in " + board + " — the main idea-capture tool. " +
				"To file the card, provide ALL of brand, stream, project and category (they're created if they don't " +
				"exist); omit all four to leave it unfiled in the inbox. Pass `description` and/or `blocks` to fill it in.",
			InputSchema: obj(map[string]any{
				"title":       strProp("Card title."),
				"card_type":   strProp("Card type (default 'idea'). See list_card_types."),
				"brand":       strProp("Brand to file under (created if missing). Provide all four hierarchy fields or none."),
				"stream":      strProp("Stream to file under (created if missing)."),
				"project":     strProp("Project to file under (created if missing)."),
				"category":    strProp("Category to file the card into (created if missing)."),
				"tags":        strArr("Tags to add to the card."),
				"description": strProp("Freeform description text for the card."),
				"blocks":      blockArrayProp("Structured content blocks to add to the card."),
			}, "title"),
		},

		// --- Populate existing cards ---
		{
			Name:        "add_card_blocks",
			Description: "Append structured content blocks to an existing card in " + board + ".",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"blocks":  blockArrayProp("Blocks to append."),
			}, "card_id", "blocks"),
		},
		{
			Name: "set_card_fields",
			Description: "Set values on a card's existing typed fields in " + board + ", matched by field key. " +
				"Use get_card first to see the available field keys.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"fields": map[string]any{
					"type":        "object",
					"description": "Map of field key → new value.",
				},
			}, "card_id", "fields"),
		},
		{
			Name:        "add_card_tags",
			Description: "Add tags to an existing card in " + board + " (existing tags are kept).",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"tags":    strArr("Tags to add."),
			}, "card_id", "tags"),
		},
	}
}
