package boardtools_test

// The project tools, once project chat's own and now the registry's (so
// UNCLI's chats have them too): in a project chat they need no location,
// staged in Suggest they apply when accepted, and over MCP they take one.

import (
	"slices"
	"strings"
	"testing"

	"bruv/core/supervisor"
	"bruv/internal/llm"
	"bruv/internal/model"
)

func TestProjectToolsInAProjectChat(t *testing.T) {
	rt := newBoard(t)
	id, scope := projectCard(t, rt, "Trip")
	must := func(name string, args map[string]any) map[string]any {
		t.Helper()
		out, text, isErr := call(t, rt, &scope, name, args)
		if isErr {
			t.Fatalf("%s: %s", name, text)
		}
		return out
	}

	// A column made this turn, moved into by name.
	must("create_category", map[string]any{"name": "Booked"})
	if out := must("move_card", map[string]any{"card_id": id, "to_category_name": "booked"}); out["moved_to"] != "Booked" {
		t.Errorf("move_card = %v", out)
	}
	if category(t, rt, id) != "Booked" {
		t.Errorf("card is in %q", category(t, rt, id))
	}
	if _, text, isErr := call(t, rt, &scope, "move_card", map[string]any{"card_id": id, "to_category_name": "Booked"}); !isErr || !strings.Contains(text, "already in Booked") {
		t.Errorf("a move to where it is: %s", text)
	}

	must("update_project", map[string]any{"description": "Snow"})
	if p, _ := rt.Repo().GetProject("home", "trips", "winter"); p == nil || p.Description != "Snow" {
		t.Errorf("project = %+v", p)
	}

	tag := func(name string) *model.Label {
		labels, _ := rt.GetProjectLabels("home", "trips", "winter")
		for i := range labels {
			if labels[i].Name == name {
				return &labels[i]
			}
		}
		return nil
	}
	must("create_project_tag", map[string]any{"name": "Urgent", "color": "#ff0000"})
	must("update_project_tag", map[string]any{"tag_name": "urgent", "name": "Soon"})
	if l := tag("Soon"); l == nil || l.Color != "#ff0000" || tag("Urgent") != nil {
		t.Errorf("renamed tag = %+v", l)
	}
	must("delete_project_tag", map[string]any{"tag_name": "Soon"})
	if tag("Soon") != nil {
		t.Error("the tag is still there")
	}

	if _, text, isErr := call(t, rt, &scope, "update_category", map[string]any{"category_name": "Booked", "accepted_types": []any{"Spaceship"}}); !isErr || !strings.Contains(text, "create_card_type") {
		t.Errorf("an unknown accepted type: %s", text)
	}
	names := func() []string {
		cats, _ := rt.ListCategories("home", "trips", "winter")
		out := make([]string, len(cats))
		for i, c := range cats {
			out[i] = c.Name
		}
		return out
	}
	before := len(names())
	must("update_category", map[string]any{"category_name": "Leads", "name": "Shortlist", "description": "Maybe"})
	must("delete_category", map[string]any{"category_name": "Shortlist"})
	if got := names(); len(got) != before-1 || slices.Contains(got, "Shortlist") || slices.Contains(got, "Leads") {
		t.Errorf("categories = %v", got)
	}

	// A name two columns share is refused, not guessed.
	must("create_category", map[string]any{"name": "Twin"})
	must("update_category", map[string]any{"category_name": "Booked", "name": "Twin"})
	if _, text, isErr := call(t, rt, &scope, "delete_category", map[string]any{"category_name": "Twin"}); !isErr || !strings.Contains(text, "2 categories") {
		t.Errorf("an ambiguous name: %s", text)
	}

	// Another project's column is out of reach.
	call(t, rt, nil, "create_category", map[string]any{"brand": "Home", "stream": "Trips", "project": "Summer", "name": "Leads"})
	if _, text, isErr := call(t, rt, &scope, "delete_category", map[string]any{"project": "Summer", "brand": "Home", "stream": "Trips", "category_name": "Leads"}); !isErr || !strings.Contains(text, "own project") {
		t.Errorf("another project's category: %s", text)
	}
}

func TestSuggestedMoveApplies(t *testing.T) {
	rt := newBoard(t)
	id, scope := projectCard(t, rt, "Trip")
	call(t, rt, nil, "create_category", map[string]any{"brand": "Home", "stream": "Trips", "project": "Winter", "name": "Booked"})

	result, edits := rt.Tools().StageProject(llm.ToolCall{Name: "move_card", Arguments: map[string]any{"card_id": id, "to_category_name": "Booked"}}, scope)
	if len(edits) != 1 || edits[0].Label != "Moved to Booked" {
		t.Fatalf("staged %v: %s", edits, result)
	}
	if category(t, rt, id) != "Leads" {
		t.Fatal("staging moved the card")
	}
	if result, _ := rt.Tools().ExecuteProject(llm.ToolCall{Name: edits[0].Tool, Arguments: edits[0].Input}, scope); strings.HasPrefix(result, "error") {
		t.Fatalf("applying: %s", result)
	}
	if category(t, rt, id) != "Booked" {
		t.Errorf("after applying, the card is in %q", category(t, rt, id))
	}
}

// A card reads only the files its Workspace Files block names.
func TestReadCardFileOnlyTheCardsFiles(t *testing.T) {
	rt := newBoard(t)
	id, _ := projectCard(t, rt, "Notes")
	if _, text, isErr := call(t, rt, nil, "read_card_file", map[string]any{"card_id": id, "path": "secrets.md"}); !isErr || !strings.Contains(text, "names no workspace files") {
		t.Errorf("a card naming no files: %s", text)
	}
}

// Over MCP (no scope) the project is named.
func TestProjectToolsNeedAProjectUnscoped(t *testing.T) {
	rt := newBoard(t)
	if _, text, isErr := call(t, rt, nil, "update_project", map[string]any{"name": "x"}); !isErr || !strings.Contains(text, "brand, stream and project are required") {
		t.Errorf("no project: %s", text)
	}
}

func category(t *testing.T, rt *supervisor.Runtime, cardID string) string {
	t.Helper()
	crumbs, err := rt.GetCardPinBreadcrumbs(cardID)
	if err != nil || len(crumbs) == 0 {
		t.Fatalf("breadcrumbs: %v %v", crumbs, err)
	}
	return crumbs[0].CategoryName
}
