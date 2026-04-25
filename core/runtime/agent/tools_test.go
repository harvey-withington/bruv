package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/prompts"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	llmsvc "bruv/core/services/llm"
	projectsvc "bruv/core/services/project"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// toolsTestDeps is the minimal Deps stub needed to drive
// executeAgentToolCall through its built-in-tool switch. The tool
// dispatch path only needs the repository — the LLM / scheduler /
// chat runtime are unused by the update_self / read_card / create_card
// branches that this test file exercises.
type toolsTestDeps struct {
	repo *repo.Repository
}

func (d *toolsTestDeps) Repo() *repo.Repository       { return d.repo }
func (d *toolsTestDeps) Index() *index.Index          { return nil }
func (d *toolsTestDeps) Registry() *schema.Registry   { return nil }
func (d *toolsTestDeps) Ctx() context.Context         { return context.Background() }
func (d *toolsTestDeps) Publish(string, any)          {}
func (d *toolsTestDeps) LLM() *llmsvc.Service         { return nil }
func (d *toolsTestDeps) Card() *card.Service          { return nil }
func (d *toolsTestDeps) Project() *projectsvc.Service { return nil }
func (d *toolsTestDeps) Catalog() *catalog.Service    { return nil }
func (d *toolsTestDeps) Prompts() *prompts.Builder    { return nil }
func (d *toolsTestDeps) ChatRT() *chatrt.Runtime      { return nil }
func (d *toolsTestDeps) MCPRegistry() *mcp.Registry   { return nil }
func (d *toolsTestDeps) LLMActors() *sync.Map         { return &sync.Map{} }

// testRuntime creates a minimal Runtime bound to a real on-disk repo
// for tool-dispatch tests.
func testRuntime(t *testing.T) (*Runtime, *repo.Repository) {
	t.Helper()
	dir := t.TempDir()
	r, err := repo.Init(dir, "test")
	if err != nil {
		t.Fatalf("repo.Init: %v", err)
	}
	rt := New(&toolsTestDeps{repo: r})
	return rt, r
}

// testCard creates a card with the given blocks and returns its ID.
func testCard(t *testing.T, r *repo.Repository, title string, blocks []model.Block) string {
	t.Helper()
	card, err := r.CreateCard("test", title)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	card.Blocks = blocks
	if err := r.UpdateCardDirect(card.ID, card); err != nil {
		t.Fatalf("UpdateCardDirect: %v", err)
	}
	return card.ID
}

func call(name string, args map[string]any) llm.ToolCall {
	return llm.ToolCall{ID: "test-call", Name: name, Arguments: args}
}

// blockItems extracts the value of a list/checklist block as []map[string]any.
// After JSON round-trip the Go decoder gives []any of map[string]any, so we
// handle both the direct (in-memory) and deserialized (from disk) shapes.
func blockItems(t *testing.T, val any) []map[string]any {
	t.Helper()
	switch v := val.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, len(v))
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("item %d is %T, not map[string]any", i, item)
			}
			out[i] = m
		}
		return out
	default:
		t.Fatalf("expected list/checklist value, got %T", val)
		return nil
	}
}

// ---------------------------------------------------------------------------
// update_self — title
// ---------------------------------------------------------------------------

func TestUpdateSelf_Title(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Old Title", nil)

	card, _ := r.GetCard(id)
	result, action := a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"title": "New Title",
	}))
	if result != "Card blocks updated successfully." {
		t.Fatalf("unexpected result: %s", result)
	}
	if action == nil || action.Tool != "update_self" {
		t.Fatal("expected action record")
	}

	updated, _ := r.GetCard(id)
	if updated.Title != "New Title" {
		t.Errorf("title = %q, want 'New Title'", updated.Title)
	}
}

func TestUpdateSelf_TitleEmpty_NoChange(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Keep This", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"title":   "",
		"updates": []any{},
	}))

	updated, _ := r.GetCard(id)
	if updated.Title != "Keep This" {
		t.Errorf("empty title should not change card, got %q", updated.Title)
	}
}

func TestUpdateSelf_TitleAndBlocks(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "description", Label: "Description", Value: "old"},
	}
	id := testCard(t, r, "Old", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"title": "New",
		"updates": []any{
			map[string]any{"key": "description", "value": "new text"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Title != "New" {
		t.Errorf("title = %q, want 'New'", updated.Title)
	}
	if updated.Blocks[0].Value != "new text" {
		t.Errorf("description = %v, want 'new text'", updated.Blocks[0].Value)
	}
}

func TestUpdateSelf_TitleViaUpdatesArray(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Old Title", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "title", "value": "Bitcoin Price: $71,005.87"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Title != "Bitcoin Price: $71,005.87" {
		t.Errorf("title = %q, want 'Bitcoin Price: $71,005.87'", updated.Title)
	}
	for _, b := range updated.Blocks {
		if strings.EqualFold(b.Key, "title") || strings.EqualFold(b.Label, "title") {
			t.Errorf("should not create a block for title, found block: %+v", b)
		}
	}
}

// ---------------------------------------------------------------------------
// update_self — due_date (top-level parameter)
// ---------------------------------------------------------------------------

func TestUpdateSelf_DueDate_TopLevel(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"due_date": "2026-05-01",
	}))

	updated, _ := r.GetCard(id)
	if updated.DueDate == nil {
		t.Fatal("due_date should be set")
	}
	if updated.DueDate.Format("2006-01-02") != "2026-05-01" {
		t.Errorf("due_date = %v, want 2026-05-01", updated.DueDate)
	}
}

func TestUpdateSelf_DueDate_RFC3339(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"due_date": "2026-05-01T10:00:00Z",
	}))

	updated, _ := r.GetCard(id)
	if updated.DueDate == nil {
		t.Fatal("due_date should be set")
	}
	if updated.DueDate.Format("2006-01-02") != "2026-05-01" {
		t.Errorf("due_date = %v, want 2026-05-01", updated.DueDate)
	}
}

func TestUpdateSelf_DueDateViaUpdatesArray(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "due_date", "value": "2026-06-15"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.DueDate == nil {
		t.Fatal("due_date should be set via updates array intercept")
	}
	if updated.DueDate.Format("2006-01-02") != "2026-06-15" {
		t.Errorf("due_date = %v, want 2026-06-15", updated.DueDate)
	}
	for _, b := range updated.Blocks {
		if strings.EqualFold(b.Key, "due_date") {
			t.Errorf("should not create a block for due_date, found: %+v", b)
		}
	}
}

// ---------------------------------------------------------------------------
// update_self — tags (top-level parameter)
// ---------------------------------------------------------------------------

func TestUpdateSelf_Tags_TopLevel(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"tags": []any{"bitcoin", "crypto", "alert"},
	}))

	updated, _ := r.GetCard(id)
	if len(updated.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(updated.Tags), updated.Tags)
	}
	if updated.Tags[0] != "bitcoin" {
		t.Errorf("tag 0 = %q, want 'bitcoin'", updated.Tags[0])
	}
}

func TestUpdateSelf_TagsViaUpdatesArray(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "tags", "value": []any{"news", "finance"}},
		},
	}))

	updated, _ := r.GetCard(id)
	if len(updated.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(updated.Tags), updated.Tags)
	}
	for _, b := range updated.Blocks {
		if strings.EqualFold(b.Key, "tags") {
			t.Errorf("should not create a block for tags, found: %+v", b)
		}
	}
}

func TestUpdateSelf_TagsViaUpdatesArray_String(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "tags", "value": "urgent"},
		},
	}))

	updated, _ := r.GetCard(id)
	if len(updated.Tags) != 1 || updated.Tags[0] != "urgent" {
		t.Errorf("expected ['urgent'], got %v", updated.Tags)
	}
}

func TestUpdateSelf_TagsBlockTakesPriority(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "tags", Label: "Tags", Value: ""},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "tags", "value": "custom block value"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "custom block value" {
		t.Errorf("block value = %v, want 'custom block value'", updated.Blocks[0].Value)
	}
	if len(updated.Tags) != 0 {
		t.Errorf("intrinsic tags should be empty, got %v", updated.Tags)
	}
}

// ---------------------------------------------------------------------------
// update_self — all intrinsic fields combined
// ---------------------------------------------------------------------------

func TestUpdateSelf_AllIntrinsicFields(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "notes", Label: "Notes", Value: ""},
	}
	id := testCard(t, r, "Old", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"title":    "New Title",
		"due_date": "2026-12-25",
		"tags":     []any{"holiday", "special"},
		"updates": []any{
			map[string]any{"key": "notes", "value": "updated"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Title != "New Title" {
		t.Errorf("title = %q", updated.Title)
	}
	if updated.DueDate == nil || updated.DueDate.Format("2006-01-02") != "2026-12-25" {
		t.Errorf("due_date = %v", updated.DueDate)
	}
	if len(updated.Tags) != 2 {
		t.Errorf("tags = %v", updated.Tags)
	}
	if updated.Blocks[0].Value != "updated" {
		t.Errorf("notes = %v", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — text block
// ---------------------------------------------------------------------------

func TestUpdateSelf_TextBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "notes", Label: "Notes", Value: ""},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "notes", "value": "updated notes"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "updated notes" {
		t.Errorf("got %v, want 'updated notes'", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — number block
// ---------------------------------------------------------------------------

func TestUpdateSelf_NumberBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockNumber, Key: "price", Label: "Price", Value: float64(0)},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "price", "value": float64(71034.02)},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(71034.02) {
		t.Errorf("got %v, want 71034.02", updated.Blocks[0].Value)
	}
}

func TestUpdateSelf_NumberBlock_StringCoercion(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockNumber, Key: "count", Label: "Count", Value: float64(0)},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "count", "value": "42"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(42) {
		t.Errorf("got %v, want 42 (coerced from string)", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — list block
// ---------------------------------------------------------------------------

func TestUpdateSelf_ListBlock_Array(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockList, Key: "items", Label: "Items", Value: nil},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "items", "value": []any{"apple", "banana", "cherry"}},
		},
	}))

	updated, _ := r.GetCard(id)
	items := blockItems(t, updated.Blocks[0].Value)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0]["text"] != "apple" {
		t.Errorf("item 0 = %v, want 'apple'", items[0]["text"])
	}
}

func TestUpdateSelf_ListBlock_String(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockList, Key: "items", Label: "Items", Value: nil},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "items", "value": "- first\n- second"},
		},
	}))

	updated, _ := r.GetCard(id)
	items := blockItems(t, updated.Blocks[0].Value)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// update_self — checklist block
// ---------------------------------------------------------------------------

func TestUpdateSelf_ChecklistBlock_StringArray(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockChecklist, Key: "tasks", Label: "Tasks", Value: nil},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "tasks", "value": []any{"task one", "task two"}},
		},
	}))

	updated, _ := r.GetCard(id)
	items := blockItems(t, updated.Blocks[0].Value)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["done"] != false {
		t.Error("new checklist items should default to not done")
	}
}

func TestUpdateSelf_ChecklistBlock_MapArray(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockChecklist, Key: "tasks", Label: "Tasks", Value: nil},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "tasks", "value": []any{
				map[string]any{"text": "done item", "done": true},
				map[string]any{"text": "pending item", "done": false},
			}},
		},
	}))

	updated, _ := r.GetCard(id)
	items := blockItems(t, updated.Blocks[0].Value)
	if items[0]["done"] != true {
		t.Error("first item should be done")
	}
	if items[1]["done"] != false {
		t.Error("second item should not be done")
	}
}

// ---------------------------------------------------------------------------
// update_self — checkbox block
// ---------------------------------------------------------------------------

func TestUpdateSelf_CheckboxBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockCheckbox, Key: "active", Label: "Active", Value: false},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "active", "value": "true"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != true {
		t.Errorf("got %v, want true (coerced from string)", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — date block
// ---------------------------------------------------------------------------

func TestUpdateSelf_DateBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockDate, Key: "deadline", Label: "Deadline", Value: ""},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "deadline", "value": "2026-04-12T10:30:00Z"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "2026-04-12" {
		t.Errorf("got %v, want '2026-04-12' (normalised)", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — select block
// ---------------------------------------------------------------------------

func TestUpdateSelf_SelectBlock_Valid(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockSelect, Key: "status", Label: "Status", Value: "",
			Meta: map[string]any{"options": []any{"open", "closed", "pending"}}},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	result, _ := a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "status", "value": "closed"},
		},
	}))
	if result != "Card blocks updated successfully." {
		t.Fatalf("expected success, got: %s", result)
	}

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "closed" {
		t.Errorf("got %v, want 'closed'", updated.Blocks[0].Value)
	}
}

func TestUpdateSelf_SelectBlock_Invalid(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockSelect, Key: "status", Label: "Status", Value: "",
			Meta: map[string]any{"options": []any{"open", "closed"}}},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	result, _ := a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "status", "value": "invalid_option"},
		},
	}))
	if result == "Card blocks updated successfully." {
		t.Fatal("expected error for invalid select option")
	}
}

// ---------------------------------------------------------------------------
// update_self — rating block
// ---------------------------------------------------------------------------

func TestUpdateSelf_RatingBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockRating, Key: "score", Label: "Score", Value: float64(0),
			Meta: map[string]any{"max": float64(5)}},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "score", "value": float64(4)},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(4) {
		t.Errorf("got %v, want 4", updated.Blocks[0].Value)
	}
}

func TestUpdateSelf_RatingBlock_Clamped(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockRating, Key: "score", Label: "Score", Value: float64(0),
			Meta: map[string]any{"max": float64(5)}},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "score", "value": float64(99)},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(5) {
		t.Errorf("got %v, want 5 (clamped)", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — progress block
// ---------------------------------------------------------------------------

func TestUpdateSelf_ProgressBlock(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockProgress, Key: "progress", Label: "Progress", Value: float64(0)},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "progress", "value": float64(75)},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(75) {
		t.Errorf("got %v, want 75", updated.Blocks[0].Value)
	}
}

func TestUpdateSelf_ProgressBlock_Clamped(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockProgress, Key: "progress", Label: "Progress", Value: float64(0)},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "progress", "value": float64(200)},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != float64(100) {
		t.Errorf("got %v, want 100 (clamped)", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — block matching by label (case-insensitive)
// ---------------------------------------------------------------------------

func TestUpdateSelf_MatchByLabel(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "my_notes", Label: "My Notes", Value: ""},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "my notes", "value": "found by label"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "found by label" {
		t.Errorf("got %v, want 'found by label'", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — new block creation
// ---------------------------------------------------------------------------

func TestUpdateSelf_CreateNewBlock(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "New Field", "value": "hello"},
		},
	}))

	updated, _ := r.GetCard(id)
	if len(updated.Blocks) != 1 {
		t.Fatalf("expected 1 new block, got %d", len(updated.Blocks))
	}
	if updated.Blocks[0].Type != model.BlockText {
		t.Errorf("new block type = %s, want text", updated.Blocks[0].Type)
	}
	if updated.Blocks[0].Label != "New Field" {
		t.Errorf("new block label = %q, want 'New Field'", updated.Blocks[0].Label)
	}
	if updated.Blocks[0].Key != "new_field" {
		t.Errorf("new block key = %q, want 'new_field'", updated.Blocks[0].Key)
	}
	if updated.Blocks[0].Value != "hello" {
		t.Errorf("new block value = %v, want 'hello'", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — error cases
// ---------------------------------------------------------------------------

func TestUpdateSelf_InvalidCardID(t *testing.T) {
	a, _ := testRuntime(t)
	result, action := a.executeAgentToolCall("nonexistent", nil, call("update_self", map[string]any{
		"title": "Won't work",
	}))
	if action == nil {
		t.Fatal("expected action record even on error")
	}
	if result == "Card blocks updated successfully." {
		t.Fatal("expected error result")
	}
}

func TestUpdateSelf_NoUpdatesNoTitle(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	result, _ := a.executeAgentToolCall(id, card, call("update_self", map[string]any{}))
	if result != "Card blocks updated successfully." {
		t.Fatalf("expected success for no-op, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// read_card
// ---------------------------------------------------------------------------

func TestReadCard(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "desc", Label: "Description", Value: "Some content"},
	}
	id := testCard(t, r, "Test Card", blocks)

	card, _ := r.GetCard(id)
	result, action := a.executeAgentToolCall("other-card", card, call("read_card", map[string]any{
		"card_id": id,
	}))
	if action == nil || action.Tool != "read_card" {
		t.Fatal("expected action record for read_card")
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestReadCard_NotFound(t *testing.T) {
	a, r := testRuntime(t)
	id := testCard(t, r, "Card", nil)

	card, _ := r.GetCard(id)
	result, _ := a.executeAgentToolCall(id, card, call("read_card", map[string]any{
		"card_id": "nonexistent",
	}))
	if result == "" {
		t.Fatal("expected error message")
	}
}

// ---------------------------------------------------------------------------
// create_card
// ---------------------------------------------------------------------------

func TestCreateCard(t *testing.T) {
	a, r := testRuntime(t)
	seedID := testCard(t, r, "Seed", nil)

	card, _ := r.GetCard(seedID)
	result, action := a.executeAgentToolCall(seedID, card, call("create_card", map[string]any{
		"title": "New Agent Card",
	}))
	if action == nil || action.Tool != "create_card" {
		t.Fatal("expected action record for create_card")
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// ---------------------------------------------------------------------------
// update_self — radio block
// ---------------------------------------------------------------------------

func TestUpdateSelf_RadioBlock_Valid(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockRadio, Key: "priority", Label: "Priority", Value: "",
			Meta: map[string]any{"options": []any{"low", "medium", "high"}}},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"updates": []any{
			map[string]any{"key": "priority", "value": "high"},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Blocks[0].Value != "high" {
		t.Errorf("got %v, want 'high'", updated.Blocks[0].Value)
	}
}

// ---------------------------------------------------------------------------
// update_self — multiple blocks in one call
// ---------------------------------------------------------------------------

func TestUpdateSelf_MultipleBlocks(t *testing.T) {
	a, r := testRuntime(t)
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Key: "summary", Label: "Summary", Value: ""},
		{ID: "b2", Type: model.BlockNumber, Key: "count", Label: "Count", Value: float64(0)},
		{ID: "b3", Type: model.BlockCheckbox, Key: "done", Label: "Done", Value: false},
	}
	id := testCard(t, r, "Card", blocks)

	card, _ := r.GetCard(id)
	a.executeAgentToolCall(id, card, call("update_self", map[string]any{
		"title": "Updated Card",
		"updates": []any{
			map[string]any{"key": "summary", "value": "new summary"},
			map[string]any{"key": "count", "value": float64(5)},
			map[string]any{"key": "done", "value": true},
		},
	}))

	updated, _ := r.GetCard(id)
	if updated.Title != "Updated Card" {
		t.Errorf("title = %q, want 'Updated Card'", updated.Title)
	}
	if updated.Blocks[0].Value != "new summary" {
		t.Errorf("summary = %v", updated.Blocks[0].Value)
	}
	if updated.Blocks[1].Value != float64(5) {
		t.Errorf("count = %v", updated.Blocks[1].Value)
	}
	if updated.Blocks[2].Value != true {
		t.Errorf("done = %v", updated.Blocks[2].Value)
	}
}
