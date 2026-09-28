package boardtools_test

// The native registry is the ONE tool set every BRUV LLM surface uses.
// These tests pin that contract: every tool is classified for scoping,
// no surface redefines a native tool under its own schema, and a scoped
// (chat) call can't reach outside its project.

import (
	"encoding/json"
	"strings"
	"testing"

	"bruv/core/boardtools"
	cardtools "bruv/core/runtime/tools"
	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/repo"
)

func newBoard(t *testing.T) *supervisor.Runtime {
	t.Helper()
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	r, err := repo.InitAt(t.TempDir(), "Board")
	if err != nil {
		t.Fatal(err)
	}
	sup, err := supervisor.New([]config.RepoEntry{{ID: "r1", Name: "Board", Path: r.Root}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rt, err := sup.Load("r1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sup.Close)
	return rt
}

func call(t *testing.T, b boardtools.Board, scope *cardtools.ProjectChatScope, name string, args map[string]any) (map[string]any, string, bool) {
	t.Helper()
	text, isErr := boardtools.CallNative(b, scope, name, args)
	var out map[string]any
	_ = json.Unmarshal([]byte(text), &out)
	return out, text, isErr
}

// Every registered tool must say whether it writes and how a project
// scope applies — an unclassified tool would bypass the chat's limit.
func TestEveryToolIsClassified(t *testing.T) {
	rt := newBoard(t)
	defs := map[string]bool{}
	for _, d := range boardtools.LLMDefs(rt, "Board", false, nil) {
		defs[d.Name] = true
	}
	for _, name := range boardtools.Names() {
		if !defs[name] {
			t.Errorf("%q is registered but has no definition", name)
		}
		if err := boardtools.CheckScope(rt, &cardtools.ProjectChatScope{CardIDs: map[string]bool{}}, name, map[string]any{}); err != nil && strings.Contains(err.Error(), "unknown tool") {
			t.Errorf("%q has no scope classification", name)
		}
	}
}

// "The internal tools should be the same" (2026-09-29): no chat or agent
// tool set may define a tool under a native name with its own schema.
func TestNoSurfaceRedefinesANativeTool(t *testing.T) {
	native := map[string]bool{}
	for _, n := range boardtools.Names() {
		native[n] = true
	}
	for surface, defs := range map[string][]llm.ToolDef{
		"card chat":    llm.CardTools(nil),
		"project chat": llm.ProjectTools(nil, nil),
		"agent":        llm.AgentBuiltinTools(),
	} {
		for _, d := range defs {
			if native[d.Name] {
				t.Errorf("%s defines %q, which is a native tool — use the native one", surface, d.Name)
			}
		}
	}
}

// REGRESSION (2026-08-14, moved from card chat): card_type must not be a
// hard enum — an unknown name is created as a new type, which an enum
// forbids. The roster rides in the description instead.
func TestCardTypeParamsHaveNoEnum(t *testing.T) {
	rt := newBoard(t)
	for _, d := range boardtools.LLMDefs(rt, "Board", false, nil) {
		if d.Name != "set_card_type" && d.Name != "create_card" {
			continue
		}
		prop := d.Parameters["properties"].(map[string]any)["card_type"].(map[string]any)
		if _, hasEnum := prop["enum"]; hasEnum {
			t.Errorf("%s card_type carries an enum", d.Name)
		}
		if desc, _ := prop["description"].(string); !strings.Contains(desc, "Brainstorm") {
			t.Errorf("%s card_type description should list the roster: %q", d.Name, desc)
		}
	}
}

func TestScopedCallsStayInTheProject(t *testing.T) {
	rt := newBoard(t)
	// Two projects: Home/Trips/Winter (the chat's) and Work/Ops/Q4.
	mine, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Mine", "brand": "Home", "stream": "Trips", "project": "Winter", "category": "Leads"})
	other, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Other", "brand": "Work", "stream": "Ops", "project": "Q4", "category": "Todo"})
	scope := cardtools.ScopeForProject(rt.Repo(), "home", "trips", "winter")
	s := &scope

	if _, text, isErr := call(t, rt, s, "get_card", map[string]any{"card_id": other["card_id"]}); !isErr || !strings.Contains(text, "not in this project") {
		t.Errorf("reading another project's card should be refused, got %s", text)
	}
	if _, _, isErr := call(t, rt, s, "get_card", map[string]any{"card_id": mine["card_id"]}); isErr {
		t.Error("reading an in-project card failed")
	}
	if _, text, isErr := call(t, rt, s, "create_brand", map[string]any{"name": "X"}); !isErr {
		t.Errorf("board-level tools must be refused in a scoped chat, got %s", text)
	}
	if _, text, isErr := call(t, rt, s, "create_card", map[string]any{"title": "Elsewhere", "brand": "Work", "stream": "Ops", "project": "Q4", "category": "Todo"}); !isErr {
		t.Errorf("filing into another project should be refused, got %s", text)
	}

	// No location: filed into the chat's own project, and reachable after.
	created, text, isErr := call(t, rt, s, "create_card", map[string]any{"title": "New lead", "category": "Leads"})
	if isErr || !strings.Contains(created["pinned_to"].(string), "Winter") {
		t.Fatalf("create_card should default to the chat's project: %s", text)
	}
	if !s.CardIDs[created["card_id"].(string)] {
		t.Error("a card created in scope must join the scope")
	}

	// Search results are limited to the project.
	_, text, _ = call(t, rt, s, "search_cards", map[string]any{"query": "Other"})
	if strings.Contains(text, other["card_id"].(string)) {
		t.Errorf("search leaked another project's card: %s", text)
	}

	// Scoped definitions leave the board-level tools out.
	for _, d := range boardtools.LLMDefs(rt, "Board", true, nil) {
		if d.Name == "create_brand" || d.Name == "list_brands" {
			t.Errorf("scoped defs should not offer %s", d.Name)
		}
	}
}

func TestUpdateCardSavesEachChangedPart(t *testing.T) {
	rt := newBoard(t)
	created, _, _ := call(t, rt, nil, "create_card", map[string]any{"title": "Race 5", "card_type": "task",
		"blocks": []any{map[string]any{"type": "date", "key": "race_start", "label": "Race start", "format": "date-time", "value": "2026-10-04T14:35:00+10:00"}}})
	id := created["card_id"].(string)

	out, text, isErr := call(t, rt, nil, "update_card", map[string]any{
		"card_id": id, "title": "Race 5 — 3:10pm", "tags": []any{"racing"},
		"updates": []any{map[string]any{"key": "race_start", "value": "2026-10-04T15:10:00+10:00"}},
	})
	if isErr {
		t.Fatalf("update_card: %s", text)
	}
	if got, _ := json.Marshal(out["updated"]); string(got) != `["title","tags","blocks"]` {
		t.Errorf("updated = %s", got)
	}
	card, _ := rt.GetCard(id)
	if card.Title != "Race 5 — 3:10pm" || len(card.Tags) != 1 {
		t.Errorf("card not updated: %+v", card)
	}
	for _, b := range card.Blocks {
		if b.Key == "race_start" && b.Value != "2026-10-04T15:10:00+10:00" {
			t.Errorf("race_start = %v, want the offset kept", b.Value)
		}
	}
}
