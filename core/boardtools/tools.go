package boardtools

import (
	"strings"

	"bruv/core/services/catalog"
	"bruv/internal/llm"
	"bruv/internal/mcp"
)

// toolFunc is one tool implementation. It returns the text to surface to
// the model and whether that text represents an error (mapped to the
// MCP result's isError flag). Tool-level failures flow back to the model
// as text rather than JSON-RPC errors so it can recover.
type toolFunc func(rt Board, args map[string]any) (string, bool)

// toolHandlers maps tool name → implementation. Definitions advertised
// to the client live in Defs; the two must stay in sync.
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
	"create_brand":     hCreateBrand,
	"create_stream":    hCreateStream,
	"create_project":   hCreateProject,
	"create_category":  hCreateCategory,
	"create_card":      hCreateCard,
	"create_card_type": hCreateCardType,
	// Populate existing cards
	"add_card_blocks":  hAddCardBlocks,
	"set_card_fields":  hSetCardFields,
	"add_card_tags":    hAddCardTags,
	"remove_card_tags": hRemoveCardTags,
	// Intrinsic card properties
	"set_card_title":       hSetCardTitle,
	"set_card_description": hSetCardDescription,
	"set_card_type":        hSetCardType,
	"set_card_due_date":    hSetCardDueDate,
	"update_card":          hUpdateCard,
	// Attachments + comments
	"add_card_attachment": hAddCardAttachment,
	"add_card_comment":    hAddCardComment,
	"list_card_comments":  hListCardComments,
	// Filing + browsing
	"pin_card":     hPinCard,
	"unpin_card":   hUnpinCard,
	"list_cards":   hListCards,
	"recent_cards": hRecentCards,
	// Card agents
	"get_card_agent":       hGetCardAgent,
	"configure_card_agent": hConfigureCardAgent,
	"run_card_agent":       hRunCardAgent,
}

// richToolFunc is a tool whose result is more than one text block — an
// embedded file, an image. It builds the CallToolResult itself. Kept as
// a separate table so the common text-only handlers stay trivial.
type richToolFunc func(rt Board, args map[string]any) mcp.CallToolResult

// richToolHandlers maps tool name → rich implementation. Advertised in
// Defs alongside the text tools; the sync test covers both tables.
var richToolHandlers = map[string]richToolFunc{
	"get_card_attachment": hGetCardAttachment,
}

// Call runs one tool and wraps its result as an MCP CallToolResult. Tool
// failures and unknown tools surface as isError text so the model can
// adjust rather than seeing a transport error.
func Call(rt Board, name string, args map[string]any) mcp.CallToolResult {
	if args == nil {
		args = map[string]any{}
	}
	if rich, ok := richToolHandlers[name]; ok {
		return rich(rt, args)
	}
	fn, ok := toolHandlers[name]
	if !ok {
		return textResult("unknown tool: "+name, true)
	}
	text, isErr := fn(rt, args)
	return textResult(text, isErr)
}

// Has reports whether name is a native board tool.
func Has(name string) bool {
	_, text := toolHandlers[name]
	_, rich := richToolHandlers[name]
	return text || rich
}

// Names lists every registered tool (both handler tables).
func Names() []string {
	out := make([]string, 0, len(toolHandlers)+len(richToolHandlers))
	for n := range toolHandlers {
		out = append(out, n)
	}
	for n := range richToolHandlers {
		out = append(out, n)
	}
	return out
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
					"'list' (array of strings), 'url', 'number', 'date' (YYYY-MM-DD; set format 'date-time' to keep a time), 'checkbox' (boolean).",
			},
			"label":  strProp("Human-readable label for the block, e.g. 'Notes', 'To-Do'."),
			"format": map[string]any{"type": "string", "enum": []any{"date", "date-time"}, "description": llm.BlockFormatDesc},
			"value":  map[string]any{"description": "The block's content. String for text/url/date; array of strings for checklist/list; boolean for checkbox; number for number."},
			"key":    strProp("Optional machine key (lowercase_with_underscores). Omit for freeform blocks."),
		}, "type", "value"),
	}
}

// cardTypeRoster renders the board's live card types (built-in + user,
// from the catalog) as a comma-separated list of labels for tool
// descriptions, so clients pick a real type instead of guessing one.
func cardTypeRoster(rt Board) string {
	types := rt.ListCardTypes()
	labels := make([]string, 0, len(types))
	for _, t := range types {
		label := t.Label
		if label == "" {
			label = t.ID
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

// Defs returns the tool list, templating the repo name and the live
// card-type roster into the descriptions so a multi-connector user sees
// which board each tool writes to and which types it actually has.
func Defs(rt Board, repoName string) []mcp.Tool {
	board := "the \"" + repoName + "\" BRUV board"
	// The live roster rides in the description rather than an enum, so a
	// type made by create_card_type earlier in the same session is still
	// accepted; the handlers refuse an unknown name (ruling 2026-09-30).
	typeDesc := "Card type — an existing type, matched case-insensitively by id or label (see list_card_types). Unknown types are refused."
	if roster := cardTypeRoster(rt); roster != "" {
		typeDesc = "Card type — one of: " + roster + " (matched case-insensitively by id or label; unknown types are refused)."
	}
	cardTypeDesc := typeDesc + " Omit for the built-in default '" + catalog.DefaultCardType + "'."

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
			Description: "List the available card types in " + board + " with their descriptions and colours. Use one of these as `card_type` when creating a card.",
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
				"card_type":   strProp(cardTypeDesc),
				"brand":       strProp("Brand to file under (created if missing). Provide all four hierarchy fields or none."),
				"stream":      strProp("Stream to file under (created if missing)."),
				"project":     strProp("Project to file under (created if missing)."),
				"category":    strProp("Category to file the card into (created if missing)."),
				"tags":        strArr("Tags to add to the card."),
				"description": strProp("Freeform description text for the card (Markdown)." + mentionNote),
				"due_date":    strProp("Optional due date, YYYY-MM-DD."),
				"blocks":      blockArrayProp("Structured content blocks to add to the card."),
			}, "title"),
		},
		{
			Name: "create_card_type",
			Description: "Add a new card type to " + board + ". Card types are the user's own vocabulary, so use this SPARINGLY: " +
				"only when the user explicitly asks for a new type, or when no existing type fits at all — never just to label one card. " +
				"Check list_card_types first; a label that already exists is refused. Returns the new type's id.",
			InputSchema: obj(map[string]any{
				"label":       strProp("Display name of the new type, e.g. 'Recipe'."),
				"color":       strProp("Optional hex colour, e.g. '#6366f1' (one is picked when omitted)."),
				"description": strProp("Optional one-line description of what cards of this type are for."),
				"ai_hint":     strProp("Optional guidance for AI assistants on when to use this type."),
			}, "label"),
		},

		// --- Populate existing cards ---
		{
			Name:        "add_card_blocks",
			Description: "Append structured content blocks to an existing card in " + board + "." + mentionNote,
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"blocks":  blockArrayProp("Blocks to append."),
			}, "card_id", "blocks"),
		},
		{
			Name: "set_card_fields",
			Description: "Set values on a card's existing typed fields in " + board + ", matched by field key. " +
				"Use get_card first to see the available field keys." + mentionNote,
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
		{
			Name:        "remove_card_tags",
			Description: "Remove tags from a card in " + board + " (matched case-insensitively). Pass all=true to clear every tag — only when the user asks for that.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"tags":    strArr("Tags to remove."),
				"all":     map[string]any{"type": "boolean", "description": "Remove every tag on the card instead of listing them."},
			}, "card_id"),
		},

		// --- Intrinsic card properties ---
		{
			Name:        "set_card_title",
			Description: "Rename a card in " + board + ".",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"title":   strProp("New title."),
			}, "card_id", "title"),
		},
		{
			Name: "set_card_description",
			Description: "Replace the description of a card in " + board + " — the free-text summary under the title, " +
				"distinct from its blocks. Call this when the user asks to describe, summarise or explain a card. Markdown is rendered.",
			InputSchema: obj(map[string]any{
				"card_id":     strProp("The card's id."),
				"description": strProp("New description (Markdown). Empty string clears it." + mentionNote),
			}, "card_id", "description"),
		},
		{
			Name:        "set_card_type",
			Description: "Change a card's type in " + board + ". See list_card_types for the available ids.",
			InputSchema: obj(map[string]any{
				"card_id":   strProp("The card's id."),
				"card_type": strProp(typeDesc),
			}, "card_id", "card_type"),
		},
		{
			Name:        "set_card_due_date",
			Description: "Set or clear a card's due date in " + board + ".",
			InputSchema: obj(map[string]any{
				"card_id":  strProp("The card's id."),
				"due_date": strProp("YYYY-MM-DD (or an ISO 8601 date-time; one without an offset is local time), or an empty string to clear the due date."),
			}, "card_id", "due_date"),
		},
		{
			Name: "update_card",
			Description: "Update a card in " + board + " in one call: title, due date, tags, and block values matched by key or label " +
				"(a new key adds a text block). Values are shaped to each block's type — arrays for list/checklist, ISO 8601 for dates " +
				"(a date-time block keeps the time and offset). Read the card first with get_card to see its blocks.",
			// Same schema as the agent's update_self, plus card_id.
			InputSchema: llm.CardUpdateParams(true),
		},

		// --- Attachments + comments ---
		{
			Name: "add_card_attachment",
			Description: "Attach a file to a card in " + board + ". Pass `text` for a UTF-8 file (Markdown, notes, CSV) " +
				"or `content_base64` for binary content — exactly one of the two. Files up to 3 MB.",
			InputSchema: obj(map[string]any{
				"card_id":        strProp("The card's id."),
				"name":           strProp("File name including extension, e.g. 'design.md'. No directories."),
				"text":           strProp("UTF-8 file content. Use for text files instead of encoding them yourself."),
				"content_base64": strProp("Base64-encoded file content. Use for binary files."),
			}, "card_id", "name"),
		},
		{
			Name: "get_card_attachment",
			Description: "Download a file attached to a card in " + board + ". Identify it by attachment_id or by name " +
				"(get_card lists both). Text files come back as text; binary files as an embedded base64 resource. " +
				"Files over 4 MB return metadata plus a short-lived download URL instead of the bytes.",
			InputSchema: obj(map[string]any{
				"card_id":       strProp("The card's id."),
				"attachment_id": strProp("The attachment's id, from get_card."),
				"name":          strProp("Alternatively, the attachment's file name (case-insensitive; the newest match wins)."),
			}, "card_id"),
		},
		{
			Name: "add_card_comment",
			Description: "Post a comment on a card in " + board + ". Use this to record an outcome, a note or a status " +
				"update without altering the card's content.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"text":    strProp("Comment text (Markdown)."),
				"author":  strProp("Optional author name shown on the comment (default 'MCP')."),
			}, "card_id", "text"),
		},
		{
			Name:        "list_card_comments",
			Description: "List the comments on a card in " + board + ", oldest first.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
			}, "card_id"),
		},

		// --- Filing + browsing ---
		{
			Name: "pin_card",
			Description: "File an existing card into a category in " + board + " (a card can be pinned in several places). " +
				"Missing Brand/Stream/Project/Category are created automatically. Use this to move an inbox card onto a board.",
			InputSchema: obj(map[string]any{
				"card_id":  strProp("The card's id."),
				"brand":    strProp("Brand name or slug (created if missing)."),
				"stream":   strProp("Stream name or slug (created if missing)."),
				"project":  strProp("Project name or slug (created if missing)."),
				"category": strProp("Category name or slug (created if missing)."),
			}, "card_id", "brand", "stream", "project", "category"),
		},
		{
			Name:        "unpin_card",
			Description: "Remove a card from one category in " + board + ". Nothing is created; the location must exist. The card itself is kept.",
			InputSchema: obj(map[string]any{
				"card_id":  strProp("The card's id."),
				"brand":    strProp("Brand name or slug."),
				"stream":   strProp("Stream name or slug."),
				"project":  strProp("Project name or slug."),
				"category": strProp("Category name or slug."),
			}, "card_id", "brand", "stream", "project", "category"),
		},
		{
			Name: "list_cards",
			Description: "List the cards on a project board in " + board + ", grouped by category in board order. " +
				"Returns compact summaries (id, title, type, position, due date, tags); use get_card for a card's content.",
			InputSchema: obj(map[string]any{
				"brand":    strProp("Brand name or slug."),
				"stream":   strProp("Stream name or slug."),
				"project":  strProp("Project name or slug."),
				"category": strProp("Optional: only this category."),
			}, "brand", "stream", "project"),
		},
		{
			Name:        "recent_cards",
			Description: "The most recently updated cards in " + board + " — useful to find something the user just created or edited.",
			InputSchema: obj(map[string]any{
				"limit": intProp("Max results (default 20)."),
			}),
		},

		// --- Card agents ---
		{
			Name:        "get_card_agent",
			Description: llm.AgentReadDescription + " Board: " + board + ".",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
			}, "card_id"),
		},
		{
			// Same schema as card/project chat's configure_agent (llm.AgentConfigParams).
			Name:        "configure_card_agent",
			Description: llm.AgentConfigDescription("get_card_agent") + " Board: " + board + ". Any card can carry an agent; for a dedicated agent card, create it with card_type 'agent'.",
			InputSchema: llm.AgentConfigParams(true, "get_card_agent"),
		},
		{
			Name: "run_card_agent",
			Description: "Run a card's agent in " + board + " now, ignoring its schedule. The card must have a goal. " +
				"Runs are asynchronous: call get_card_agent afterwards to see the result.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
			}, "card_id"),
		},
	}
}

// mentionNote teaches assistants the one markup for cross-card links, so
// they stop pasting raw ids into item text (field report 2026-09-15).
const mentionNote = " To link another card inside any text (description, block text, list or checklist items, captions, comments), write a mention as Markdown: [Card title](bruv:card:<card id>) - ids come from search_cards / list_cards / get_card. It renders as a clickable link on every surface."
