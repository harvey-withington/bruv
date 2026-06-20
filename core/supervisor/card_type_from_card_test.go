package supervisor

import (
	"testing"

	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	cfgDir := t.TempDir()
	r, err := repo.InitAt(t.TempDir(), "Test Repo")
	if err != nil {
		t.Fatalf("repo.InitAt: %v", err)
	}
	sup, err := New([]config.RepoEntry{{ID: "r1", Name: "Test Repo", Path: r.Root}}, cfgDir)
	if err != nil {
		t.Fatalf("supervisor.New: %v", err)
	}
	rt, err := sup.Load("r1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(sup.Close)
	return rt
}

func TestCreateCardTypeFromCard(t *testing.T) {
	rt := newTestRuntime(t)

	card, err := rt.CreateCard("", "Episode 12")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	// Three freeform blocks (empty keys), two of which we'll templatise.
	blocks := []model.Block{
		{ID: "b1", Type: model.BlockText, Label: "Notes", Value: "draft notes"},
		{ID: "b2", Type: model.BlockSelect, Label: "Priority", Value: "High",
			Meta: map[string]any{"options": []any{"Low", "High"}}},
		{ID: "b3", Type: model.BlockChecklist, Label: "Shots", Value: []any{"intro"}},
	}
	if _, err := rt.UpdateCardBlocks(card.ID, blocks); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}

	info, err := rt.CreateCardTypeFromCard(card.ID, "Episode", "calendar", "#ec4899", []string{"b1", "b2"})
	if err != nil {
		t.Fatalf("CreateCardTypeFromCard: %v", err)
	}

	// --- The new type ---
	if info.ID == "" || info.Label != "Episode" || info.Color != "#ec4899" {
		t.Fatalf("type info = %+v, want Episode/#ec4899 with an id", info)
	}
	if info.Icon != "calendar" {
		t.Errorf("icon = %q, want calendar", info.Icon)
	}
	if info.TemplateID == "" {
		t.Fatal("type has no template id")
	}
	var found bool
	for _, ct := range rt.ListCardTypes() {
		if ct.ID == info.ID {
			found = true
			if ct.Icon != "calendar" {
				t.Errorf("listed type icon = %q, want calendar", ct.Icon)
			}
		}
	}
	if !found {
		t.Error("new type not in ListCardTypes")
	}

	// --- The template: selected blocks, keys derived, values stripped ---
	templates, _ := rt.ListCardTemplates()
	var tmpl *config.CardTemplate
	for i := range templates {
		if templates[i].ID == info.TemplateID {
			tmpl = &templates[i]
		}
	}
	if tmpl == nil {
		t.Fatal("template not found")
	}
	if len(tmpl.Blocks) != 2 {
		t.Fatalf("template has %d blocks, want 2 (only selected)", len(tmpl.Blocks))
	}
	byKey := map[string]model.Block{}
	for _, b := range tmpl.Blocks {
		byKey[b.Key] = b
	}
	notes, ok := byKey["notes"]
	if !ok {
		t.Fatalf("template missing 'notes' field; keys = %v", keysOf(tmpl.Blocks))
	}
	if notes.Value != "" {
		t.Errorf("template 'notes' value = %#v, want empty (stripped)", notes.Value)
	}
	priority, ok := byKey["priority"]
	if !ok {
		t.Fatalf("template missing 'priority' field; keys = %v", keysOf(tmpl.Blocks))
	}
	if priority.Meta["options"] == nil {
		t.Errorf("template 'priority' lost its options meta: %+v", priority.Meta)
	}

	// --- The card: switched type, keys assigned, values kept, NO duplicates ---
	updated, err := rt.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if updated.Type != info.ID {
		t.Errorf("card type = %q, want %q", updated.Type, info.ID)
	}
	if len(updated.Blocks) != 3 {
		t.Fatalf("card has %d blocks, want 3 (no duplicate template fields appended); blocks = %+v", len(updated.Blocks), updated.Blocks)
	}
	for _, b := range updated.Blocks {
		switch b.ID {
		case "b1":
			if b.Key != "notes" || b.Value != "draft notes" {
				t.Errorf("b1 = key %q value %#v, want notes/'draft notes' (key assigned, value kept)", b.Key, b.Value)
			}
		case "b2":
			if b.Key != "priority" || b.Value != "High" {
				t.Errorf("b2 = key %q value %#v, want priority/'High'", b.Key, b.Value)
			}
		case "b3":
			if b.Key != "" {
				t.Errorf("b3 (unselected) key = %q, want empty (untouched)", b.Key)
			}
		}
	}
}

func TestCreateCardTypeFromCard_RequiresName(t *testing.T) {
	rt := newTestRuntime(t)
	card, _ := rt.CreateCard("", "x")
	if _, err := rt.CreateCardTypeFromCard(card.ID, "   ", "", "#fff", nil); err == nil {
		t.Error("expected error for blank name")
	}
}

func keysOf(blocks []model.Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Key
	}
	return out
}
