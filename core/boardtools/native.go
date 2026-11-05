package boardtools

// In-process use of the registry by BRUV's own LLM surfaces (card chat,
// project chat, agents). Same tools, names and handlers as the MCP
// server; what differs per surface is the scope a call may touch:
//
//   - Unscoped (MCP, agents): the whole board.
//   - Scoped (project chat, card chat): one project. Card ids must be in
//     that project, hierarchy arguments default to it and may not point
//     elsewhere, search results are filtered to it, and tools that only
//     make sense board-wide (creating brands/streams/projects) are not
//     offered at all.
//
// The per-tool classification behind both lives in meta.go.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/mcp"
)

// LLMDefs returns the tool definitions for an in-process surface. scoped
// drops the board-only tools; include (optional) filters by name.
func LLMDefs(b Board, repoName string, scoped bool, include func(string) bool) []llm.ToolDef {
	var out []llm.ToolDef
	for _, t := range Defs(b, repoName) {
		if scoped && meta[t.Name].scope == scopeBoardOnly {
			continue
		}
		if include != nil && !include(t.Name) {
			continue
		}
		out = append(out, llm.ToolDef{Name: t.Name, Description: t.Description, Parameters: t.InputSchema})
	}
	return out
}

// CallNative runs a tool in-process and flattens the result to text for
// a chat or agent transcript. scope nil = the whole board. A card the
// tool creates inside a scope is added to it, so later calls in the same
// session can reach it.
func CallNative(b Board, scope *cardtools.ProjectChatScope, name string, args map[string]any) (string, bool) {
	if args == nil {
		args = map[string]any{}
	}
	if scope != nil {
		if err := checkScope(b, scope, name, args); err != nil {
			return "error: " + err.Error(), true
		}
	}
	res := Call(b, name, args)
	text := flatten(res)
	if scope != nil && !res.IsError {
		text = scopeResult(scope, name, text)
	}
	return text, res.IsError
}

// CheckScope validates a scoped call without running it (Suggest mode
// stages only calls that would be allowed). Like the call itself, it
// fills omitted brand/stream/project with the scope's.
func CheckScope(b Board, s *cardtools.ProjectChatScope, name string, args map[string]any) error {
	if s == nil {
		return nil
	}
	return checkScope(b, s, name, args)
}

// checkScope validates (and, for locations, defaults) a scoped call's
// arguments. It mutates args to fill the scope's brand/stream/project.
func checkScope(b Board, s *cardtools.ProjectChatScope, name string, args map[string]any) error {
	rule, ok := meta[name]
	if !ok {
		return fmt.Errorf("unknown tool %q", name)
	}
	switch rule.scope {
	case scopeBoardOnly:
		return fmt.Errorf("%s isn't available here; this chat can only work inside its project", name)
	case scopeCard:
		return checkCard(s, args)
	case scopeLocation:
		return checkLocation(b, s, name, args)
	case scopeCardAndLocation:
		if err := checkCard(s, args); err != nil {
			return err
		}
		return checkLocation(b, s, name, args)
	}
	return nil
}

func checkCard(s *cardtools.ProjectChatScope, args map[string]any) error {
	id := argStr(args, "card_id")
	if id != "" && s.CardIDs != nil && !s.CardIDs[id] {
		return fmt.Errorf("card %s is not in this project", id)
	}
	return nil
}

// checkLocation fills omitted brand/stream/project with the scope's and
// refuses any that name a different project. A category is required only
// where the tool needs one (the handler reports that itself).
func checkLocation(b Board, s *cardtools.ProjectChatScope, name string, args map[string]any) error {
	loc := cardtools.LocationArgs(args)
	if loc.Brand == "" && loc.Stream == "" && loc.Project == "" {
		// create_card with no location at all would land in the inbox,
		// outside the project — file it here instead.
		args["brand"], args["stream"], args["project"] = s.BrandSlug, s.StreamSlug, s.ProjectSlug
		if name == "create_card" && loc.Category == "" {
			return fmt.Errorf("create_card needs a category of this project (see list_categories)")
		}
		return nil
	}
	ps := b.ProjectService()
	brand, _, okB := cardtools.FindBrand(ps, loc.Brand)
	stream, _, okS := cardtools.FindStream(ps, brand, loc.Stream)
	project, _, okP := cardtools.FindProject(ps, brand, stream, loc.Project)
	if !okB || !okS || !okP || brand != s.BrandSlug || stream != s.StreamSlug || project != s.ProjectSlug {
		return fmt.Errorf("%s can only use this chat's own project", name)
	}
	return nil
}

// scopeResult filters search results to in-scope cards and records a
// card created in scope.
func scopeResult(s *cardtools.ProjectChatScope, name, text string) string {
	switch meta[name].scope {
	case scopeResults:
		var results []index.SearchResult
		if json.Unmarshal([]byte(text), &results) != nil || s.CardIDs == nil {
			return text
		}
		kept := results[:0]
		for _, r := range results {
			if s.CardIDs[r.CardID] {
				kept = append(kept, r)
			}
		}
		if out, err := json.MarshalIndent(kept, "", "  "); err == nil {
			return string(out)
		}
	}
	if name == "create_card" && s.CardIDs != nil {
		var created struct {
			CardID string `json:"card_id"`
		}
		if json.Unmarshal([]byte(text), &created) == nil && created.CardID != "" {
			s.CardIDs[created.CardID] = true
		}
	}
	return text
}

// flatten turns a CallToolResult into transcript text. Binary content
// (an attachment) is summarised; its metadata text block carries the id.
func flatten(res mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		switch {
		case c.Type == "text":
			parts = append(parts, c.Text)
		case c.Resource != nil:
			parts = append(parts, fmt.Sprintf("[binary attachment %s, %s — not shown in chat]", c.Resource.URI, c.Resource.MimeType))
		}
	}
	return strings.Join(parts, "\n")
}

// Summary is a one-line, human-readable outcome of a native tool call
// for the chat's action list and a pending edit's label. For create_card
// it includes "(ID: …)" so the UI can link to the new card.
func Summary(name string, args map[string]any, result string) string {
	var out struct {
		CardID string `json:"card_id"`
		Title  string `json:"title"`
	}
	_ = json.Unmarshal([]byte(result), &out)
	title := argStr(args, "title")
	switch name {
	case "create_card":
		if out.CardID != "" {
			return fmt.Sprintf("Created card '%s' (ID: %s)", firstNonEmpty(out.Title, title), out.CardID)
		}
		return "Create card: " + title
	case "set_card_title":
		return "Title → " + title
	case "set_card_description":
		if argStr(args, "description") == "" {
			return "Cleared the description"
		}
		return "Set the description"
	case "set_card_type":
		return "Type → " + argStr(args, "card_type")
	case "set_card_due_date":
		if d := argStr(args, "due_date"); d != "" {
			return "Due date → " + d
		}
		return "Cleared the due date"
	case "add_card_tags":
		return "Added tags: " + strings.Join(argStrSlice(args, "tags"), ", ")
	case "remove_card_tags":
		if all, _ := args["all"].(bool); all {
			return "Removed all tags"
		}
		return "Removed tags: " + strings.Join(argStrSlice(args, "tags"), ", ")
	case "create_card_type":
		return "Created card type: " + argStr(args, "label")
	case "configure_card_agent":
		return "Configured the agent"
	case "update_card":
		return updateCardSummary(args)
	case "set_card_fields":
		var r struct {
			Updated []string `json:"updated_fields"`
		}
		_ = json.Unmarshal([]byte(result), &r)
		if len(r.Updated) > 0 {
			return "Updated fields: " + strings.Join(r.Updated, ", ")
		}
		fields, _ := args["fields"].(map[string]any)
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "Set fields: " + strings.Join(keys, ", ")
	case "add_card_blocks":
		blocks, _ := args["blocks"].([]any)
		labels := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok {
				labels = append(labels, firstNonEmpty(argStr(m, "label"), argStr(m, "key")))
			}
		}
		return "Added fields: " + strings.Join(labels, ", ")
	case "add_checklist_items":
		items, _ := args["items"].([]any)
		return fmt.Sprintf("Added %d item(s) to %s", len(items), argStr(args, "checklist"))
	}
	return strings.ReplaceAll(name, "_", " ")
}

// updateCardSummary names what an update_card call changes, block by
// block (by the key or label it names), so a review or approval reads
// "Updated Preparation to Leave" rather than "update card".
func updateCardSummary(args map[string]any) string {
	var parts []string
	if title := argStr(args, "title"); title != "" {
		parts = append(parts, "title → "+title)
	}
	if due, ok := args["due_date"].(string); ok {
		if due != "" {
			parts = append(parts, "due date → "+due)
		} else {
			parts = append(parts, "cleared the due date")
		}
	}
	if _, ok := args["tags"]; ok {
		parts = append(parts, "tags → "+strings.Join(argStrSlice(args, "tags"), ", "))
	}
	updates, _ := args["updates"].([]any)
	fields := make([]string, 0, len(updates))
	for _, u := range updates {
		if m, ok := u.(map[string]any); ok {
			if key := argStr(m, "key"); key != "" {
				fields = append(fields, key)
			}
		}
	}
	if len(fields) > 0 {
		parts = append(parts, strings.Join(fields, ", "))
	}
	if len(parts) == 0 {
		return "Updated the card"
	}
	return "Updated " + strings.Join(parts, "; ")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
