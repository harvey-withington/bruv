package mcpserver

// tools/list carries MCP tool annotations (spec 2025-03-26) so clients can
// gate BRUV's tools: run the reads freely, prompt for or refuse the writes.
// The classification is pinned here on purpose — a new tool fails this
// test until someone decides what it does to the board.

import (
	"encoding/json"
	"testing"
)

// hints is one tool's expected annotations: read-only, destructive,
// idempotent, open-world.
type hints struct{ readOnly, destructive, idempotent, openWorld bool }

var (
	readHints    = hints{readOnly: true, idempotent: true}
	addHints     = hints{}
	addOnceHints = hints{idempotent: true}
	replaceHints = hints{destructive: true, idempotent: true}
)

var wantHints = map[string]hints{
	"list_brands":         readHints,
	"list_streams":        readHints,
	"list_projects":       readHints,
	"list_categories":     readHints,
	"list_cards":          readHints,
	"list_card_types":     readHints,
	"list_card_comments":  readHints,
	"get_card":            readHints,
	"get_card_attachment": readHints,
	"get_card_agent":      readHints,
	"search_cards":        readHints,
	"recent_cards":        readHints,

	"create_brand":        addHints,
	"create_stream":       addHints,
	"create_project":      addHints,
	"create_category":     addHints,
	"create_card":         addHints,
	"create_card_type":    addHints,
	"add_card_attachment": addHints,
	"add_card_blocks":     addHints,
	"add_checklist_items": addHints,
	"add_card_comment":    addHints,
	"add_card_tags":       addOnceHints,
	"pin_card":            addOnceHints,

	// Overwriting a value counts as destructive (ruling 2026-11-02).
	"update_card":          replaceHints,
	"set_card_title":       replaceHints,
	"set_card_description": replaceHints,
	"set_card_fields":      replaceHints,
	"set_card_type":        replaceHints,
	"set_card_due_date":    replaceHints,
	"configure_card_agent": replaceHints,
	"remove_card_tags":     replaceHints,
	"unpin_card":           replaceHints,
	"move_card":            replaceHints,
	"update_project":       replaceHints,
	"update_project_tag":   replaceHints,
	"delete_project_tag":   replaceHints,
	"update_category":      replaceHints,
	"delete_category":      replaceHints,
	"create_project_tag":   addHints,

	// The agent's own tools may reach outside the board.
	"run_card_agent": {destructive: true, openWorld: true},
}

func TestToolsListCarriesAnnotations(t *testing.T) {
	h, _ := newTestHandler(t)
	_, resp := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	raw, _ := json.Marshal(resp.Result)
	// Pointers, so a missing hint (which a client would read as the
	// spec's default) is told apart from an explicit false.
	var out struct {
		Tools []struct {
			Name        string `json:"name"`
			Annotations *struct {
				ReadOnly    *bool `json:"readOnlyHint"`
				Destructive *bool `json:"destructiveHint"`
				Idempotent  *bool `json:"idempotentHint"`
				OpenWorld   *bool `json:"openWorldHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range out.Tools {
		seen[tool.Name] = true
		want, ok := wantHints[tool.Name]
		if !ok {
			t.Errorf("%s has no expected classification — add it to wantHints (and meta.go)", tool.Name)
			continue
		}
		a := tool.Annotations
		if a == nil || a.ReadOnly == nil || a.Destructive == nil || a.Idempotent == nil || a.OpenWorld == nil {
			t.Errorf("%s must send all four hints explicitly", tool.Name)
			continue
		}
		got := hints{*a.ReadOnly, *a.Destructive, *a.Idempotent, *a.OpenWorld}
		if got != want {
			t.Errorf("%s annotations = %+v, want %+v", tool.Name, got, want)
		}
	}
	for name := range wantHints {
		if !seen[name] {
			t.Errorf("%s is pinned here but not advertised by tools/list", name)
		}
	}
}
