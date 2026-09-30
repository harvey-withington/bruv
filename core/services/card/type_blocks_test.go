package card

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/repo"
)

// failingTypeDeps fails every template-block merge, so the card service's
// handling of that failure can be observed.
type failingTypeDeps struct{ testDeps }

func (failingTypeDeps) ApplyTypeBlocks(_, _ string) error { return errors.New("disk full") }

func newFailingTypeService(t *testing.T) (*Service, *repo.Repository) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Init(dir, "Type Blocks Test")
	if err != nil {
		t.Fatal(err)
	}
	return New(failingTypeDeps{testDeps{r: r}}), r
}

// A typed card whose type fields can't be added must not be left behind
// half-made: Create fails as a whole and the card is gone, so a retry
// (by the user or a model) doesn't accumulate field-less duplicates.
func TestCreateRollsBackWhenTypeFieldsFail(t *testing.T) {
	s, r := newFailingTypeService(t)
	if _, err := s.Create("task", "Half made"); err == nil {
		t.Fatal("Create must report the failed type-field merge")
	}
	cards, err := r.ListCards()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 0 {
		t.Fatalf("half-created card left behind: %+v", cards)
	}
}

// An untyped create never merges template blocks, so it's unaffected.
func TestCreateUntypedIgnoresTypeFields(t *testing.T) {
	s, _ := newFailingTypeService(t)
	if _, err := s.Create("", "Plain"); err != nil {
		t.Fatalf("untyped Create: %v", err)
	}
}

// A type change keeps the new type on disk but reports the failed merge.
func TestUpdateTypeReportsFailedFields(t *testing.T) {
	s, r := newFailingTypeService(t)
	c, err := s.Create("", "Retype me")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateType(c.ID, "task"); err == nil {
		t.Fatal("UpdateType must report the failed type-field merge")
	}
	got, err := r.GetCard(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "task" {
		t.Errorf("type = %q, want the change to stand", got.Type)
	}
}
