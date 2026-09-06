package mcpserver

// End-to-end coverage for the card-property, attachment, comment, filing
// and browsing tools added 2026-09-06. Same harness as server_test.go: a
// real Supervisor over a fresh repo, driven through tools/call.

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustCallTool(t *testing.T, h *Handler, name string, args map[string]any) string {
	t.Helper()
	text, isErr := callToolRPC(t, h, name, args)
	if isErr {
		t.Fatalf("%s reported error: %s", name, text)
	}
	return text
}

func decodeJSON(t *testing.T, text string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(text), into); err != nil {
		t.Fatalf("decode %q: %v", text, err)
	}
}

func TestToolsListAdvertisesNewTools(t *testing.T) {
	h, _ := newTestHandler(t)
	_, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	raw, _ := json.Marshal(resp.Result)
	var out struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(raw, &out)
	advertised := map[string]bool{}
	for _, tool := range out.Tools {
		advertised[tool.Name] = true
	}
	// Every registered handler must be advertised and vice versa — the two
	// tables are maintained by hand.
	for name := range toolHandlers {
		if !advertised[name] {
			t.Errorf("handler %q has no tools/list definition", name)
		}
	}
	for name := range advertised {
		if _, ok := toolHandlers[name]; !ok {
			t.Errorf("tools/list advertises %q but no handler is registered", name)
		}
	}
}

func TestSetCardIntrinsics(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)
	created, err := rt.CreateCard("idea", "Draft")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	id := created.ID

	mustCallTool(t, h, "set_card_title", map[string]any{"card_id": id, "title": "Final title"})
	mustCallTool(t, h, "set_card_description", map[string]any{"card_id": id, "description": "A **summary**."})
	mustCallTool(t, h, "set_card_type", map[string]any{"card_id": id, "card_type": "task"})
	mustCallTool(t, h, "set_card_due_date", map[string]any{"card_id": id, "due_date": "2026-12-24"})

	card, _ := rt.GetCard(id)
	if card.Title != "Final title" {
		t.Errorf("title = %q", card.Title)
	}
	if card.Description != "A **summary**." {
		t.Errorf("description = %q", card.Description)
	}
	if card.Type != "task" {
		t.Errorf("type = %q", card.Type)
	}
	if card.DueDate == nil || card.DueDate.Format("2006-01-02") != "2026-12-24" {
		t.Errorf("due date = %v, want 2026-12-24", card.DueDate)
	}

	// Clearing the due date and description via empty strings.
	mustCallTool(t, h, "set_card_due_date", map[string]any{"card_id": id, "due_date": ""})
	mustCallTool(t, h, "set_card_description", map[string]any{"card_id": id, "description": ""})
	card, _ = rt.GetCard(id)
	if card.DueDate != nil {
		t.Errorf("due date not cleared: %v", card.DueDate)
	}
	if card.Description != "" {
		t.Errorf("description not cleared: %q", card.Description)
	}

	// Malformed dates are rejected before touching the card.
	if text, isErr := callToolRPC(t, h, "set_card_due_date", map[string]any{"card_id": id, "due_date": "24/12/2026"}); !isErr {
		t.Errorf("expected error for non-ISO date, got %s", text)
	}
	// A missing description argument is an error, not a silent clear.
	if _, isErr := callToolRPC(t, h, "set_card_description", map[string]any{"card_id": id}); !isErr {
		t.Error("expected error when description is omitted")
	}
}

func TestAddCardAttachment(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)
	created, _ := rt.CreateCard("idea", "With files")
	id := created.ID

	text := mustCallTool(t, h, "add_card_attachment", map[string]any{
		"card_id": id, "name": "notes.md", "text": "# Notes\n\nhello",
	})
	var out struct {
		AttachmentID string `json:"attachment_id"`
		Name         string `json:"name"`
		Size         int    `json:"size"`
	}
	decodeJSON(t, text, &out)
	if out.Name != "notes.md" || out.Size != len("# Notes\n\nhello") || out.AttachmentID == "" {
		t.Errorf("attachment result = %+v", out)
	}
	card, _ := rt.GetCard(id)
	if len(card.FileAttachments) != 1 || card.FileAttachments[0].Name != "notes.md" {
		t.Errorf("card attachments = %+v", card.FileAttachments)
	}

	// Binary path: base64 content.
	mustCallTool(t, h, "add_card_attachment", map[string]any{
		"card_id": id, "name": "blob.bin", "content_base64": "AAECAwQ=",
	})
	card, _ = rt.GetCard(id)
	if len(card.FileAttachments) != 2 || card.FileAttachments[1].Size != 5 {
		t.Errorf("binary attachment not recorded: %+v", card.FileAttachments)
	}

	// Exactly one content argument; no path separators in the name.
	for _, bad := range []map[string]any{
		{"card_id": id, "name": "x.txt"},
		{"card_id": id, "name": "x.txt", "text": "a", "content_base64": "YQ=="},
		{"card_id": id, "name": "../x.txt", "text": "a"},
	} {
		if _, isErr := callToolRPC(t, h, "add_card_attachment", bad); !isErr {
			t.Errorf("expected error for args %v", bad)
		}
	}
}

func TestCardComments(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)
	created, _ := rt.CreateCard("idea", "Discussed")
	id := created.ID

	empty := mustCallTool(t, h, "list_card_comments", map[string]any{"card_id": id})
	if strings.TrimSpace(empty) != "[]" {
		t.Errorf("expected empty JSON array for no comments, got %q", empty)
	}

	mustCallTool(t, h, "add_card_comment", map[string]any{"card_id": id, "text": "Deployed and verified."})
	mustCallTool(t, h, "add_card_comment", map[string]any{"card_id": id, "text": "Follow-up needed.", "author": "Claude Code"})

	var comments []struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	}
	decodeJSON(t, mustCallTool(t, h, "list_card_comments", map[string]any{"card_id": id}), &comments)
	if len(comments) != 2 {
		t.Fatalf("comments = %+v, want 2", comments)
	}
	if comments[0].Author != defaultCommentAuthor || comments[0].Text != "Deployed and verified." {
		t.Errorf("first comment = %+v", comments[0])
	}
	if comments[1].Author != "Claude Code" {
		t.Errorf("second comment author = %q", comments[1].Author)
	}
}

func TestPinUnpinAndListCards(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)
	a, _ := rt.CreateCard("idea", "Alpha")
	b, _ := rt.CreateCard("idea", "Beta")

	// pin_card creates the whole hierarchy on first use.
	text := mustCallTool(t, h, "pin_card", map[string]any{
		"card_id": a.ID, "brand": "Acme", "stream": "Apps", "project": "Widget", "category": "Ideas",
	})
	if !strings.Contains(text, "Acme / Apps / Widget / Ideas") {
		t.Errorf("pin breadcrumb missing: %s", text)
	}
	mustCallTool(t, h, "pin_card", map[string]any{
		"card_id": b.ID, "brand": "acme", "stream": "apps", "project": "widget", "category": "Ideas",
	})

	var listed []struct {
		Category string `json:"category"`
		Cards    []struct {
			CardID   string `json:"card_id"`
			Title    string `json:"title"`
			Position int    `json:"position"`
		} `json:"cards"`
	}
	decodeJSON(t, mustCallTool(t, h, "list_cards", map[string]any{
		"brand": "Acme", "stream": "Apps", "project": "Widget",
	}), &listed)
	if len(listed) != 1 || listed[0].Category != "Ideas" || len(listed[0].Cards) != 2 {
		t.Fatalf("list_cards = %+v", listed)
	}
	// Both cards were pinned fresh, so both sit at position 0 (Position is
	// per-card, not per-category — see hListCards). Assert membership, and
	// that the listing is stable across calls, rather than a specific order.
	titles := map[string]bool{listed[0].Cards[0].Title: true, listed[0].Cards[1].Title: true}
	if !titles["Alpha"] || !titles["Beta"] {
		t.Errorf("list_cards titles = %v, want Alpha and Beta", titles)
	}
	var again []struct {
		Cards []struct {
			CardID string `json:"card_id"`
		} `json:"cards"`
	}
	decodeJSON(t, mustCallTool(t, h, "list_cards", map[string]any{
		"brand": "Acme", "stream": "Apps", "project": "Widget",
	}), &again)
	for i := range again[0].Cards {
		if again[0].Cards[i].CardID != listed[0].Cards[i].CardID {
			t.Fatalf("list_cards order changed between identical calls")
		}
	}

	// Filtering to a category that doesn't exist is an error, not [].
	if _, isErr := callToolRPC(t, h, "list_cards", map[string]any{
		"brand": "Acme", "stream": "Apps", "project": "Widget", "category": "Nope",
	}); !isErr {
		t.Error("expected error for unknown category filter")
	}

	// unpin_card never creates: a typo fails loudly and leaves the board alone.
	if _, isErr := callToolRPC(t, h, "unpin_card", map[string]any{
		"card_id": a.ID, "brand": "Acme", "stream": "Apps", "project": "Widget", "category": "Typo",
	}); !isErr {
		t.Error("expected error unpinning from a non-existent category")
	}
	mustCallTool(t, h, "unpin_card", map[string]any{
		"card_id": a.ID, "brand": "Acme", "stream": "Apps", "project": "Widget", "category": "Ideas",
	})
	decodeJSON(t, mustCallTool(t, h, "list_cards", map[string]any{
		"brand": "Acme", "stream": "Apps", "project": "Widget", "category": "ideas",
	}), &listed)
	if len(listed) != 1 || len(listed[0].Cards) != 1 || listed[0].Cards[0].CardID != b.ID {
		t.Errorf("after unpin, list_cards = %+v", listed)
	}
	if _, err := rt.GetCard(a.ID); err != nil {
		t.Errorf("unpin must keep the card: %v", err)
	}
}

func TestRecentCards(t *testing.T) {
	h, sup := newTestHandler(t)
	rt := sup.Resolve(testRepoID)
	if _, err := rt.CreateCard("idea", "Newest"); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	text := mustCallTool(t, h, "recent_cards", map[string]any{"limit": 5})
	if !strings.Contains(text, "Newest") {
		t.Errorf("recent_cards did not include the new card: %s", text)
	}
}
