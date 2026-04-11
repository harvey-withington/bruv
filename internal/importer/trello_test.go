package importer

import (
	"bruv/internal/model"
	"bruv/internal/repo"
	"testing"
	"time"
)

// A hand-written mini Trello export covering the features the importer needs to
// handle: two active lists, one closed list, labels, a checklist, a closed card,
// a due date, an image attachment, a URL attachment, and a comment action.
const fixtureBoard = `{
  "id": "board1",
  "name": "Harvey's Demo Board",
  "desc": "A test board for BRUV import",
  "closed": false,
  "labels": [
    {"id": "lbl-red", "name": "Bug", "color": "red"},
    {"id": "lbl-blue", "name": "", "color": "blue"}
  ],
  "lists": [
    {"id": "list-todo", "name": "To Do", "closed": false, "pos": 1000},
    {"id": "list-doing", "name": "In Progress", "closed": false, "pos": 2000},
    {"id": "list-done-closed", "name": "Done", "closed": true, "pos": 3000}
  ],
  "cards": [
    {
      "id": "card-1",
      "name": "First card",
      "desc": "This is the description.",
      "closed": false,
      "idList": "list-todo",
      "idLabels": ["lbl-red"],
      "idChecklists": ["chk-1"],
      "due": "2026-05-01T09:00:00.000Z",
      "pos": 1000,
      "attachments": [
        {"id": "att-1", "name": "diagram.png", "url": "https://example.com/diagram.png", "mimeType": "image/png"},
        {"id": "att-2", "name": "spec", "url": "https://example.com/spec.pdf", "mimeType": "application/pdf"}
      ]
    },
    {
      "id": "card-2",
      "name": "Second card",
      "desc": "",
      "closed": false,
      "idList": "list-doing",
      "idLabels": ["lbl-blue"],
      "idChecklists": [],
      "due": null,
      "pos": 1000,
      "attachments": []
    },
    {
      "id": "card-3",
      "name": "Archived card",
      "desc": "This was done.",
      "closed": true,
      "idList": "list-todo",
      "idLabels": [],
      "idChecklists": [],
      "due": null,
      "pos": 2000,
      "attachments": []
    }
  ],
  "checklists": [
    {
      "id": "chk-1",
      "name": "Subtasks",
      "idCard": "card-1",
      "checkItems": [
        {"id": "ci-1", "name": "Sketch it", "state": "complete", "pos": 1000},
        {"id": "ci-2", "name": "Build it",  "state": "incomplete", "pos": 2000}
      ]
    }
  ],
  "actions": [
    {
      "id": "act-1",
      "type": "commentCard",
      "date": "2026-04-10T12:34:56.000Z",
      "memberCreator": {"fullName": "Harvey", "username": "harvey"},
      "data": {"text": "This needs clarifying.", "card": {"id": "card-1"}}
    }
  ],
  "members": [{"id": "m-1", "fullName": "Harvey", "username": "harvey"}]
}`

// setupImporterRepo creates a fresh repo with a brand + stream ready for imports.
func setupImporterRepo(t *testing.T) *repo.Repository {
	t.Helper()
	dir := t.TempDir()
	r, err := repo.Init(dir, "Test Repository")
	if err != nil {
		t.Fatalf("repo.Init: %v", err)
	}
	if _, err := r.CreateBrand("Test Brand"); err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}
	if _, err := r.CreateStream("test-brand", "Main"); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	return r
}

func TestParseTrelloJSON_Valid(t *testing.T) {
	parsed, err := ParseTrelloJSON([]byte(fixtureBoard))
	if err != nil {
		t.Fatalf("ParseTrelloJSON: %v", err)
	}
	if parsed.Board.Name != "Harvey's Demo Board" {
		t.Errorf("board name = %q, want Harvey's Demo Board", parsed.Board.Name)
	}
	if len(parsed.Board.Lists) != 3 {
		t.Errorf("list count = %d, want 3", len(parsed.Board.Lists))
	}
	if len(parsed.Checklists) != 1 {
		t.Errorf("checklist count = %d, want 1", len(parsed.Checklists))
	}
	if _, ok := parsed.Checklists["chk-1"]; !ok {
		t.Error("checklist chk-1 not indexed")
	}
}

func TestParseTrelloJSON_Invalid(t *testing.T) {
	if _, err := ParseTrelloJSON([]byte("{}")); err == nil {
		t.Error("expected error for empty JSON, got nil")
	}
	if _, err := ParseTrelloJSON([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestImportTrello_ArchiveSkip(t *testing.T) {
	r := setupImporterRepo(t)
	parsed, err := ParseTrelloJSON([]byte(fixtureBoard))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	result, err := ImportTrello(r, "test-brand", "main", parsed, Options{Archive: ArchiveSkip})
	if err != nil {
		t.Fatalf("ImportTrello: %v", err)
	}

	// Only two active lists -> two categories; no archive.
	if result.Categories != 2 {
		t.Errorf("Categories = %d, want 2", result.Categories)
	}
	// Two active cards; card-3 is closed and list-todo is active, so it's skipped.
	if result.Cards != 2 {
		t.Errorf("Cards = %d, want 2", result.Cards)
	}
	if result.Archived != 0 {
		t.Errorf("Archived = %d, want 0", result.Archived)
	}
	if result.SkippedClosed != 1 {
		t.Errorf("SkippedClosed = %d, want 1", result.SkippedClosed)
	}
	if result.Comments != 1 {
		t.Errorf("Comments = %d, want 1", result.Comments)
	}
	if result.Labels != 2 {
		t.Errorf("Labels = %d, want 2", result.Labels)
	}

	// Verify categories exist in the repo and are in correct order.
	cats, err := r.ListCategories("test-brand", "main", result.ProjectSlug)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("cats = %d, want 2", len(cats))
	}
	if cats[0].Name != "To Do" {
		t.Errorf("first category = %q, want To Do", cats[0].Name)
	}
	if cats[1].Name != "In Progress" {
		t.Errorf("second category = %q, want In Progress", cats[1].Name)
	}

	// Verify card 1 has description + checklist + image block + url block.
	pins, _ := r.ListCardsInCategory(cats[0].ID, cats[0].ID)
	if len(pins) != 1 {
		t.Fatalf("to-do pins = %d, want 1", len(pins))
	}
	card, err := r.GetCard(pins[0].CardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Title != "First card" {
		t.Errorf("title = %q, want First card", card.Title)
	}
	if card.Type != "" {
		t.Errorf("card type = %q, want empty (no type)", card.Type)
	}
	if card.DueDate == nil {
		t.Error("expected non-nil DueDate")
	}
	if len(card.Labels) != 1 {
		t.Errorf("labels = %d, want 1", len(card.Labels))
	}
	// Trello labels become both internal per-project labels AND user-facing Tags.
	if len(card.Tags) != 1 || card.Tags[0] != "Bug" {
		t.Errorf("tags = %v, want [Bug]", card.Tags)
	}
	// The intrinsic Description field reads from Fields["description"], not the
	// block value, so the importer has to mirror the text into both places.
	if desc, _ := card.Fields["description"].(string); desc != "This is the description." {
		t.Errorf("Fields[description] = %q, want %q", desc, "This is the description.")
	}
	// Expect 4 blocks: description, checklist, image, url
	wantTypes := []string{model.BlockText, model.BlockChecklist, model.BlockImage, model.BlockURL}
	if len(card.Blocks) != len(wantTypes) {
		t.Fatalf("blocks = %d (%v), want %d", len(card.Blocks), blockTypes(card.Blocks), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if card.Blocks[i].Type != wt {
			t.Errorf("block[%d].Type = %q, want %q", i, card.Blocks[i].Type, wt)
		}
	}

	// Verify the comment was imported with the original timestamp + author.
	cf, err := r.LoadComments(card.ID)
	if err != nil {
		t.Fatalf("LoadComments: %v", err)
	}
	if len(cf.Comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(cf.Comments))
	}
	if cf.Comments[0].Author != "Harvey" {
		t.Errorf("comment author = %q, want Harvey", cf.Comments[0].Author)
	}
	want, _ := time.Parse(time.RFC3339, "2026-04-10T12:34:56Z")
	if !cf.Comments[0].CreatedAt.Equal(want) {
		t.Errorf("comment createdAt = %v, want %v", cf.Comments[0].CreatedAt, want)
	}
}

func TestImportTrello_ArchiveSeparate(t *testing.T) {
	r := setupImporterRepo(t)
	parsed, _ := ParseTrelloJSON([]byte(fixtureBoard))

	result, err := ImportTrello(r, "test-brand", "main", parsed, Options{Archive: ArchiveSeparate})
	if err != nil {
		t.Fatalf("ImportTrello: %v", err)
	}

	// Active lists (2) + closed list "Done" → "Archive: Done" (1) + catch-all Archive (1, for card-3)
	if result.Categories != 4 {
		t.Errorf("Categories = %d, want 4", result.Categories)
	}
	if result.Cards != 3 {
		t.Errorf("Cards = %d, want 3", result.Cards)
	}
	if result.Archived != 1 {
		t.Errorf("Archived = %d, want 1", result.Archived)
	}
	if result.SkippedClosed != 0 {
		t.Errorf("SkippedClosed = %d, want 0", result.SkippedClosed)
	}

	cats, _ := r.ListCategories("test-brand", "main", result.ProjectSlug)
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = c.Name
	}
	if len(names) != 4 {
		t.Fatalf("category names = %v, want 4", names)
	}
	// Expect: To Do, In Progress, Archive: Done, Archive
	if names[0] != "To Do" || names[1] != "In Progress" {
		t.Errorf("active category order wrong: %v", names)
	}
	hasArchiveDone := false
	hasArchive := false
	for _, n := range names {
		if n == "Archive: Done" {
			hasArchiveDone = true
		}
		if n == "Archive" {
			hasArchive = true
		}
	}
	if !hasArchiveDone {
		t.Errorf("missing 'Archive: Done' category, got %v", names)
	}
	if !hasArchive {
		t.Errorf("missing 'Archive' catch-all category, got %v", names)
	}
}

func TestImportTrello_ArchiveInline(t *testing.T) {
	r := setupImporterRepo(t)
	parsed, _ := ParseTrelloJSON([]byte(fixtureBoard))

	result, err := ImportTrello(r, "test-brand", "main", parsed, Options{Archive: ArchiveInline})
	if err != nil {
		t.Fatalf("ImportTrello: %v", err)
	}

	// Three categories: active "To Do" and "In Progress", plus the formerly
	// closed "Done" as a normal category.
	if result.Categories != 3 {
		t.Errorf("Categories = %d, want 3", result.Categories)
	}
	if result.Cards != 3 {
		t.Errorf("Cards = %d, want 3", result.Cards)
	}
	if result.Archived != 1 {
		t.Errorf("Archived = %d, want 1", result.Archived)
	}
}

func TestExportProject_Roundtrip(t *testing.T) {
	r := setupImporterRepo(t)
	parsed, _ := ParseTrelloJSON([]byte(fixtureBoard))
	result, err := ImportTrello(r, "test-brand", "main", parsed, Options{Archive: ArchiveInline})
	if err != nil {
		t.Fatalf("ImportTrello: %v", err)
	}

	data, err := ExportProject(r, "test-brand", "main", result.ProjectSlug)
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty export")
	}

	// Quick sanity-check: the export should include the project name, all
	// category names, all card titles, and the imported comment.
	s := string(data)
	wantStrings := []string{
		"Harvey's Demo Board",
		"To Do",
		"In Progress",
		"First card",
		"Second card",
		"Archived card",
		"This needs clarifying.",
	}
	for _, w := range wantStrings {
		if !contains(s, w) {
			t.Errorf("export missing %q", w)
		}
	}
}

func blockTypes(blocks []model.Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Type
	}
	return out
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	// Avoid importing strings just for tests; manual substring search.
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}
