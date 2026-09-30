package card

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bruv/internal/index"
	"bruv/internal/repo"
)

// testDeps satisfies Deps over a real temp repo with no-op instrumentation.
type testDeps struct{ r *repo.Repository }

func (d testDeps) Repo() *repo.Repository                                     { return d.r }
func (d testDeps) Index() *index.Index                                        { return nil }
func (d testDeps) ApplyTypeBlocks(_, _ string) error                          { return nil }
func (d testDeps) CardTypeExists(string) bool                                 { return true }
func (d testDeps) LogActivity(_, _, _ string)                                 {}
func (d testDeps) LogActivityWithContext(_, _, _, _ string, _ []CategoryPath) {}
func (d testDeps) Publish(string, any)                                        {}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Init(dir, "Card Test")
	if err != nil {
		t.Fatal(err)
	}
	return New(testDeps{r: r})
}

func TestParseDueDate(t *testing.T) {
	local := func(y int, m time.Month, d, h, min int) time.Time {
		return time.Date(y, m, d, h, min, 0, 0, time.Local)
	}
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-10-04", time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		{"2026-10-04T14:35:00+10:00", time.Date(2026, 10, 4, 4, 35, 0, 0, time.UTC)},
		// Zone-less date-times are wall-clock times: local, never UTC.
		{"2026-10-04T14:35", local(2026, 10, 4, 14, 35)},
		{"2026-10-04T14:35:00", local(2026, 10, 4, 14, 35)},
		{"2026-10-04 14:35", local(2026, 10, 4, 14, 35)},
	}
	for _, c := range cases {
		got, err := ParseDueDate(c.in)
		if err != nil || !got.Equal(c.want) {
			t.Errorf("ParseDueDate(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"next friday", "04/10/2026", "soon"} {
		if _, err := ParseDueDate(bad); err == nil {
			t.Errorf("ParseDueDate(%q) must fail, not yield a zero date", bad)
		}
	}
}

// An unparseable due date used to be stored as year 0001 and reported as
// success (reachable from every LLM tool: "next friday").
func TestUpdateDueDateRejectsUnparseable(t *testing.T) {
	s := newTestService(t)
	c, err := s.Create("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDueDate(c.ID, "2026-10-04"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateDueDate(c.ID, "next friday"); err == nil {
		t.Fatal("an unparseable due date must be an error")
	}
	got, _ := s.Get(c.ID)
	if got.DueDate == nil || got.DueDate.Year() != 2026 {
		t.Errorf("due date = %v, want the earlier 2026-10-04 kept", got.DueDate)
	}
	if got, err := s.UpdateDueDate(c.ID, ""); err != nil || got.DueDate != nil {
		t.Errorf("empty must clear: %v, %v", got.DueDate, err)
	}
}
