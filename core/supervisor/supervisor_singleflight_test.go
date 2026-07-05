package supervisor

import (
	"path/filepath"
	"sync"
	"testing"

	"bruv/internal/config"
	"bruv/internal/repo"
)

// Reproduces the repo-switch stampede: on switching repos the frontend
// fires a burst of parallel RPCs, each lazily calling Load(id). Before
// single-flighting, several buildRuntimes raced on the same repo — the
// one whose index.Open lost the SQLite lock finished first (it skipped
// the refresh work), won the cache slot with a nil index, and the board
// rendered cardless until app restart.
func TestLoadSingleFlight(t *testing.T) {
	r, err := repo.InitAt(filepath.Join(t.TempDir(), "vault"), "Race Repo")
	if err != nil {
		t.Fatal(err)
	}
	sup, err := New([]config.RepoEntry{{ID: "race", Name: "Race Repo", Path: r.Root}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sup.Close)

	const callers = 16
	var wg sync.WaitGroup
	results := make([]*Runtime, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = sup.Load("race")
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("Load[%d]: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Fatalf("Load[%d] returned nil runtime", i)
		}
		if results[i] != results[0] {
			t.Fatalf("Load[%d] returned a different runtime — build ran more than once", i)
		}
	}
	// The cached runtime must have a live index: a nil index here is
	// exactly the silent degradation that rendered boards cardless.
	if results[0].Index() == nil {
		t.Fatal("runtime cached with nil index — index.Open lost a lock race")
	}
	if got := sup.Resolve("race"); got != results[0] {
		t.Fatal("Resolve disagrees with Load result")
	}
}
