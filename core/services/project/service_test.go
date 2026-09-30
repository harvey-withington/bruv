package project

import (
	"path/filepath"
	"testing"

	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
)

type testDeps struct{ r *repo.Repository }

func (d testDeps) Repo() *repo.Repository { return d.r }
func (d testDeps) Index() *index.Index    { return nil }
func (d testDeps) Publish(string, any)    {}

// categoryFixture is a project with two categories: "from" holding two
// cards (a task and a brainstorm) and an empty "to".
func categoryFixture(t *testing.T) (*Service, *repo.Repository, [3]string, *model.Category, *model.Category) {
	t.Helper()
	r, err := repo.InitAt(filepath.Join(t.TempDir(), "vault"), "Vault")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r.CreateBrand("B")
	st, _ := r.CreateStream(b.Slug, "S")
	p, _ := r.CreateProject(b.Slug, st.Slug, "P")
	from, err := r.CreateCategory(b.Slug, st.Slug, p.Slug, "From", 0)
	if err != nil {
		t.Fatal(err)
	}
	to, err := r.CreateCategory(b.Slug, st.Slug, p.Slug, "To", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"task", "brainstorm"} {
		c, _ := r.CreateCard(typ, typ)
		if err := r.PinCard(c.ID, from.ID); err != nil {
			t.Fatal(err)
		}
	}
	return New(testDeps{r: r}), r, [3]string{b.Slug, st.Slug, p.Slug}, from, to
}

// Cards are found from the pin files, so moving works with no (or a
// stale) search index (pre-release sweep 2026-09-29, §3.2).
func TestMoveCategoryCardsReadsPins(t *testing.T) {
	s, r, path, from, to := categoryFixture(t)
	if err := s.MoveCategoryCards(path[0], path[1], path[2], from.ID, to.ID); err != nil {
		t.Fatal(err)
	}
	left, _ := r.ListCardsInCategory(from.ID)
	moved, _ := r.ListCardsInCategory(to.ID)
	if len(left) != 0 || len(moved) != 2 {
		t.Errorf("left %d moved %d, want 0 and 2", len(left), len(moved))
	}
}

func TestMoveCategoryCardsHonoursAcceptedTypes(t *testing.T) {
	s, r, path, from, to := categoryFixture(t)
	if _, err := r.UpdateCategory(path[0], path[1], path[2], to.Slug, func(c *model.Category) {
		c.AcceptedTypes = []string{"task"}
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveCategoryCards(path[0], path[1], path[2], from.ID, to.ID); err == nil {
		t.Fatal("a brainstorm card was moved into a task-only category")
	}
	if moved, _ := r.ListCardsInCategory(to.ID); len(moved) != 0 {
		t.Errorf("%d card(s) moved; a refusal must move nothing", len(moved))
	}
}

func TestCopyProjectCopiesCardsWithoutIndex(t *testing.T) {
	s, r, path, _, _ := categoryFixture(t)
	copied, err := s.CopyProject(path[0], path[1], path[2], path[0], path[1], 1)
	if err != nil {
		t.Fatal(err)
	}
	cats, _ := r.ListCategories(path[0], path[1], copied.Slug)
	total := 0
	for _, c := range cats {
		pins, _ := r.ListCardsInCategory(c.ID)
		total += len(pins)
	}
	if total != 2 {
		t.Errorf("copied project holds %d cards, want 2", total)
	}
}
