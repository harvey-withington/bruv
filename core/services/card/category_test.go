package card

import (
	"testing"

	"bruv/internal/model"
)

// categoryWithTask is a project category holding one task card.
func categoryWithTask(t *testing.T, s *Service) (brand, stream, project string, cat *model.Category, cardID string) {
	t.Helper()
	r := s.deps.Repo()
	b, _ := r.CreateBrand("B")
	st, _ := r.CreateStream(b.Slug, "S")
	p, _ := r.CreateProject(b.Slug, st.Slug, "P")
	cat, err := r.CreateCategory(b.Slug, st.Slug, p.Slug, "Tasks", 0)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := r.CreateCard("task", "A task")
	if err := r.PinCard(c.ID, cat.ID); err != nil {
		t.Fatal(err)
	}
	return b.Slug, st.Slug, p.Slug, cat, c.ID
}

// CopyCategory reads the source's cards from the pin files, so it copies
// them with no (or a stale) search index (pre-release sweep 2026-09-29, §3.2).
func TestCopyCategoryCopiesCardsWithoutIndex(t *testing.T) {
	s := newTestService(t)
	b, st, p, cat, _ := categoryWithTask(t, s)
	copied, err := s.CopyCategory(b, st, p, cat.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if pins, _ := s.deps.Repo().ListCardsInCategory(copied.ID); len(pins) != 1 {
		t.Errorf("copy holds %d cards, want 1", len(pins))
	}
}

func TestDuplicateHonoursAcceptedTypes(t *testing.T) {
	s := newTestService(t)
	b, st, p, _, cardID := categoryWithTask(t, s)
	r := s.deps.Repo()
	notes, _ := r.CreateCategory(b, st, p, "Notes", 1)
	if _, err := r.UpdateCategory(b, st, p, notes.Slug, func(c *model.Category) {
		c.AcceptedTypes = []string{"brainstorm"}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Duplicate(cardID, notes.ID); err == nil {
		t.Error("a task was duplicated into a brainstorm-only category")
	}
}
