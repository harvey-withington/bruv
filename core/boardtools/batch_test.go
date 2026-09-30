package boardtools_test

// Project chat's batch edits (update_cards, add_tags_to_cards) run as
// loops over the native registry. The Suggest-mode test is the release
// gate: every staged row, once accepted, must really change the card —
// before, most staged fields were accepted with a green tick and did
// nothing (pre-release sweep 2026-09-29, 4.1).

import (
	"slices"
	"strings"
	"testing"

	cardtools "bruv/core/runtime/tools"
	"bruv/core/supervisor"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// projectCard files a card into Home / Trips / Winter / Leads and gives
// it a select field with configured options (block Meta that a
// wholesale block rewrite used to drop).
func projectCard(t *testing.T, rt *supervisor.Runtime, title string) (string, cardtools.ProjectChatScope) {
	t.Helper()
	created, text, isErr := call(t, rt, nil, "create_card", map[string]any{
		"title": title, "brand": "Home", "stream": "Trips", "project": "Winter", "category": "Leads",
		"tags": []any{"keep", "drop"}, "due_date": "2026-01-01",
	})
	if isErr {
		t.Fatalf("create_card: %s", text)
	}
	id := created["card_id"].(string)
	card, _ := rt.GetCard(id)
	blocks := append(card.Blocks, model.Block{
		ID: "b-status", Type: model.BlockSelect, Label: "Status", Key: "status", Value: "todo",
		Meta: map[string]any{"options": []any{"todo", "done"}},
	})
	if _, err := rt.UpdateCardBlocks(id, blocks); err != nil {
		t.Fatal(err)
	}
	return id, cardtools.ScopeForProject(rt.Repo(), "home", "trips", "winter")
}

func TestSuggestModeUpdateCardsAppliesEveryField(t *testing.T) {
	rt := newBoard(t)
	id, scope := projectCard(t, rt, "Old title")

	_, edits := rt.Tools().StageProject(llm.ToolCall{Name: "update_cards", Arguments: map[string]any{
		"updates": []any{map[string]any{
			"card_id":        id,
			"title":          "New title",
			"card_type":      "Task",
			"tags_to_add":    []any{"added"},
			"tags_to_remove": []any{"DROP"},
			"due_date":       "",
			"description":    "A description",
			"blocks": []any{
				map[string]any{"type": "select", "label": "Status", "key": "status", "value": "done"},
				map[string]any{"type": "text", "label": "Venue", "key": "venue", "value": "fresh"},
			},
		}},
	}}, scope)
	if len(edits) < 8 {
		t.Fatalf("staged %d edits, want one per field: %+v", len(edits), edits)
	}
	// Accept every row: each replays through the project executor exactly
	// as ApplyProjectPendingEdits does.
	for _, e := range edits {
		result, _ := rt.Tools().ExecuteProject(llm.ToolCall{ID: e.ID, Name: e.Tool, Arguments: e.Input}, scope)
		if strings.HasPrefix(strings.ToLower(result), "error") {
			t.Errorf("accepting %q (%s) failed: %s", e.Label, e.Tool, result)
		}
	}

	card, err := rt.GetCard(id)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "New title" {
		t.Errorf("title = %q", card.Title)
	}
	if card.Type != "task" {
		t.Errorf("type = %q, want task", card.Type)
	}
	if !slices.Equal(card.Tags, []string{"keep", "added"}) {
		t.Errorf("tags = %v, want [keep added]", card.Tags)
	}
	if card.DueDate != nil {
		t.Errorf("due date = %v, want cleared", card.DueDate)
	}
	if card.Description != "A description" {
		t.Errorf("description = %q", card.Description)
	}
	status := blockByKey(card, "status")
	if status == nil || status.Value != "done" || status.ID != "b-status" {
		t.Fatalf("status block = %+v, want the same block set to done", status)
	}
	if opts, _ := status.Meta["options"].([]any); len(opts) != 2 {
		t.Errorf("status options lost: %v", status.Meta)
	}
	if venue := blockByKey(card, "venue"); venue == nil || venue.Value != "fresh" {
		t.Errorf("venue block = %+v, want it added", venue)
	}

	// tags: [] asks for no tags at all; accepting it must clear them.
	_, edits = rt.Tools().StageProject(llm.ToolCall{Name: "update_cards", Arguments: map[string]any{
		"updates": []any{map[string]any{"card_id": id, "tags": []any{}}},
	}}, scope)
	for _, e := range edits {
		if result, _ := rt.Tools().ExecuteProject(llm.ToolCall{Name: e.Tool, Arguments: e.Input}, scope); strings.HasPrefix(result, "error") {
			t.Fatalf("clear tags: %s", result)
		}
	}
	if card, _ = rt.GetCard(id); len(card.Tags) != 0 {
		t.Errorf("tags = %v, want none", card.Tags)
	}
}

// Edit mode reports what each card really got: a refused field is an
// error in the result (not "Updated N cards"), the other fields still
// land, and a bad select value never replaces the field or its options.
func TestEditModeUpdateCardsReportsPerCardErrors(t *testing.T) {
	rt := newBoard(t)
	good, scope := projectCard(t, rt, "Good")
	bad, _ := projectCard(t, rt, "Bad")
	scope = cardtools.ScopeForProject(rt.Repo(), "home", "trips", "winter")

	result, action := rt.Tools().ExecuteProject(llm.ToolCall{Name: "update_cards", Arguments: map[string]any{
		"updates": []any{
			map[string]any{"card_id": good, "title": "Good 2", "blocks": []any{map[string]any{"key": "status", "value": "done"}}},
			map[string]any{"card_id": bad, "title": "Bad 2", "card_type": "Nonexistent", "blocks": []any{map[string]any{"key": "status", "value": "maybe"}}},
		},
	}}, scope)
	if action == nil || !strings.Contains(result, "FAILED") || !strings.Contains(result, "unknown card type") {
		t.Fatalf("result should report the failed card and why:\n%s", result)
	}
	if strings.HasPrefix(result, "error") {
		t.Errorf("a partly successful batch must not read as a total failure:\n%s", result)
	}

	g, _ := rt.GetCard(good)
	if g.Title != "Good 2" || blockByKey(g, "status").Value != "done" {
		t.Errorf("good card not updated: %q %v", g.Title, blockByKey(g, "status").Value)
	}
	b, _ := rt.GetCard(bad)
	if b.Title != "Bad 2" {
		t.Errorf("valid fields of a partly failed card should still land, title = %q", b.Title)
	}
	if b.Type == "nonexistent" || rt.Catalog.CardTypeExists("nonexistent") {
		t.Error("an unknown type was applied or created")
	}
	if st := blockByKey(b, "status"); st.Value != "todo" || st.Meta == nil {
		t.Errorf("rejected select value must leave the field untouched: %+v", st)
	}

	// add_tags_to_cards: per-card native add_card_tags, errors reported.
	result, _ = rt.Tools().ExecuteProject(llm.ToolCall{Name: "add_tags_to_cards", Arguments: map[string]any{
		"card_ids": []any{good}, "tags": []any{"x"},
	}}, scope)
	if g, _ = rt.GetCard(good); !slices.Contains(g.Tags, "x") || strings.HasPrefix(result, "error") {
		t.Errorf("add_tags_to_cards: tags %v, result %s", g.Tags, result)
	}
}

// update_card with none of its own fields used to "succeed" with
// {"updated":[]} — a staged edit then showed a green tick for nothing.
func TestUpdateCardWithNothingToDoIsAnError(t *testing.T) {
	rt := newBoard(t)
	id, _ := projectCard(t, rt, "Card")
	if _, text, isErr := call(t, rt, nil, "update_card", map[string]any{"card_id": id, "card_type": "task"}); !isErr {
		t.Errorf("update_card with no fields it sets must be an error, got %s", text)
	}
	if _, text, isErr := call(t, rt, nil, "update_card", map[string]any{"card_id": id, "due_date": "next friday"}); !isErr {
		t.Errorf("an unparseable due date must be an error, got %s", text)
	}
}

func TestRemoveCardTags(t *testing.T) {
	rt := newBoard(t)
	id, _ := projectCard(t, rt, "Card")
	out, text, isErr := call(t, rt, nil, "remove_card_tags", map[string]any{"card_id": id, "tags": []any{"DROP", "absent"}})
	if isErr {
		t.Fatalf("remove_card_tags: %s", text)
	}
	if card, _ := rt.GetCard(id); !slices.Equal(card.Tags, []string{"keep"}) {
		t.Errorf("tags = %v, want [keep] (removed %v)", card.Tags, out["tags_removed"])
	}
	if _, text, isErr := call(t, rt, nil, "remove_card_tags", map[string]any{"card_id": id}); !isErr {
		t.Errorf("no tags and no all=true must be refused, got %s", text)
	}
	if _, _, isErr := call(t, rt, nil, "remove_card_tags", map[string]any{"card_id": id, "all": true}); isErr {
		t.Fatal("all=true failed")
	}
	if card, _ := rt.GetCard(id); len(card.Tags) != 0 {
		t.Errorf("tags = %v, want none", card.Tags)
	}
}

// create_card refuses up front whatever would fail after the card exists:
// a category whose accepted types exclude the card, an unknown block type,
// two blocks with one key. Each refusal used to leave an orphan card in
// the Inbox, and each model retry added another.
func TestCreateCardLeavesNoOrphanOnRefusal(t *testing.T) {
	rt := newBoard(t)
	projectCard(t, rt, "Seed") // creates Home / Trips / Winter / Leads
	if _, err := rt.UpdateCategoryAcceptedTypes("home", "trips", "winter", "leads", []string{"task"}); err != nil {
		t.Fatal(err)
	}
	before, _ := rt.ListCards()

	loc := func(args map[string]any) map[string]any {
		args["brand"], args["stream"], args["project"], args["category"] = "Home", "Trips", "Winter", "Leads"
		return args
	}
	refused := []map[string]any{
		loc(map[string]any{"title": "Wrong type", "card_type": "brainstorm"}),
		loc(map[string]any{"title": "Bad block", "card_type": "task", "blocks": []any{map[string]any{"type": "sparkles", "value": "x"}}}),
		loc(map[string]any{"title": "Dup keys", "card_type": "task", "blocks": []any{
			map[string]any{"type": "text", "key": "k", "value": "a"}, map[string]any{"type": "text", "key": "k", "value": "b"},
		}}),
		{"title": "Bad due", "due_date": "next friday"},
	}
	for _, args := range refused {
		if _, text, isErr := call(t, rt, nil, "create_card", args); !isErr {
			t.Errorf("create_card %q should be refused, got %s", args["title"], text)
		}
	}
	if after, _ := rt.ListCards(); len(after) != len(before) {
		t.Errorf("refused creates left %d orphan card(s)", len(after)-len(before))
	}

	// The accepted type files fine.
	if _, text, isErr := call(t, rt, nil, "create_card", loc(map[string]any{"title": "Right type", "card_type": "task"})); isErr {
		t.Errorf("an accepted type should file: %s", text)
	}
}
