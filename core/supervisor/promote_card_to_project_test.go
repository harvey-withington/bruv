package supervisor

import (
	"strings"
	"testing"
)

// promoteFixture creates a runtime with one brand/stream and one card
// carrying tags and a description.
func promoteFixture(t *testing.T) (*Runtime, string) {
	t.Helper()
	rt := newTestRuntime(t)
	if _, err := rt.CreateBrand("Acme"); err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}
	if _, err := rt.CreateStream("acme", "Content"); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	card, err := rt.CreateCard("", "Big idea")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := rt.UpdateCardDescription(card.ID, "A promising brainstorm."); err != nil {
		t.Fatalf("UpdateCardDescription: %v", err)
	}
	if _, err := rt.UpdateCardTags(card.ID, []string{"urgent", "video"}); err != nil {
		t.Fatalf("UpdateCardTags: %v", err)
	}
	return rt, card.ID
}

func TestPromoteCardToProject(t *testing.T) {
	rt, cardID := promoteFixture(t)

	res, err := rt.PromoteCardToProject(cardID, "acme", "content", "Big Idea Project", false)
	if err != nil {
		t.Fatalf("PromoteCardToProject: %v", err)
	}
	if res.Project == nil || res.Project.Name != "Big Idea Project" {
		t.Fatalf("project = %+v, want name 'Big Idea Project'", res.Project)
	}
	if res.Category == nil || res.Category.Name != "Ideas" {
		t.Fatalf("category = %+v, want default 'Ideas'", res.Category)
	}
	if res.Category.ProjectID != res.Project.ID {
		t.Errorf("category.ProjectID = %q, want %q", res.Category.ProjectID, res.Project.ID)
	}

	// The card is pinned into the default category.
	pins, err := rt.GetCardPins(cardID)
	if err != nil {
		t.Fatalf("GetCardPins: %v", err)
	}
	var pinned bool
	for _, p := range pins {
		if p.CategoryID == res.Category.ID {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("card not pinned in new category; pins = %+v", pins)
	}

	// The card's tags were seeded into the new project's palette.
	labels, err := rt.GetProjectLabels("acme", "content", res.Project.Slug)
	if err != nil {
		t.Fatalf("GetProjectLabels: %v", err)
	}
	have := map[string]bool{}
	for _, l := range labels {
		have[strings.ToLower(l.Name)] = true
	}
	if !have["urgent"] || !have["video"] {
		t.Errorf("project labels = %+v, want urgent + video seeded", labels)
	}

	// Description NOT copied when copyDescription is false.
	if res.Project.Description != "" {
		t.Errorf("project description = %q, want empty", res.Project.Description)
	}
}

func TestPromoteCardToProject_CopiesDescription(t *testing.T) {
	rt, cardID := promoteFixture(t)

	res, err := rt.PromoteCardToProject(cardID, "acme", "content", "With Description", true)
	if err != nil {
		t.Fatalf("PromoteCardToProject: %v", err)
	}
	if res.Project.Description != "A promising brainstorm." {
		t.Errorf("project description = %q, want the card's description", res.Project.Description)
	}
}

func TestPromoteCardToProject_KeepsExistingPins(t *testing.T) {
	rt, cardID := promoteFixture(t)

	// Pin the card somewhere first: a project created the ordinary way.
	origin, originCat, err := rt.Project.CreateProjectWithDefaultCategory("acme", "content", "Origin")
	if err != nil || originCat == nil {
		t.Fatalf("CreateProjectWithDefaultCategory: %v (cat=%v)", err, originCat)
	}
	if err := rt.PinCard(cardID, originCat.ID); err != nil {
		t.Fatalf("PinCard: %v", err)
	}
	_ = origin

	res, err := rt.PromoteCardToProject(cardID, "acme", "content", "Promoted", false)
	if err != nil {
		t.Fatalf("PromoteCardToProject: %v", err)
	}

	pins, _ := rt.GetCardPins(cardID)
	if len(pins) != 2 {
		t.Fatalf("pins = %+v, want 2 (original kept + new)", pins)
	}
	have := map[string]bool{}
	for _, p := range pins {
		have[p.CategoryID] = true
	}
	if !have[originCat.ID] || !have[res.Category.ID] {
		t.Errorf("pins missing a category: %+v", pins)
	}
}

func TestPromoteCardToProject_RequiresName(t *testing.T) {
	rt, cardID := promoteFixture(t)
	if _, err := rt.PromoteCardToProject(cardID, "acme", "content", "   ", false); err == nil {
		t.Error("expected error for blank name")
	}
}

func TestPromoteCardToProject_UnknownCard(t *testing.T) {
	rt, _ := promoteFixture(t)
	if _, err := rt.PromoteCardToProject("no-such-card", "acme", "content", "X", false); err == nil {
		t.Error("expected error for unknown card")
	}
	// No project should have been created.
	projects, _ := rt.ListProjects("acme", "content")
	if len(projects) != 0 {
		t.Errorf("projects = %+v, want none (validation before create)", projects)
	}
}

func TestPromoteCardToProject_UnknownStream(t *testing.T) {
	rt, cardID := promoteFixture(t)
	if _, err := rt.PromoteCardToProject(cardID, "acme", "no-such-stream", "X", false); err == nil {
		t.Error("expected error for unknown stream")
	}
}
