package catalog

import (
	"fmt"
	"sync"
	"testing"

	"bruv/internal/config"
)

// Concurrent card-type writes each land: the store is read-modified-
// written under its lock (pre-release sweep 2026-09-29, §3.1).
func TestCardTypeWritesConcurrent(t *testing.T) {
	s, _ := newMergeService(t)
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, err := s.CreateUserCardType(fmt.Sprintf("Type %d", i), "#000000", "", "", "")
				if err != nil {
					t.Error(err)
				}
				return
			}
			if _, err := s.CreateCardTemplate(fmt.Sprintf("Tpl %d", i), nil); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	tmpls, _ := s.ListCardTemplates()
	types := s.ListCardTypes()
	if len(tmpls) != n/2 || len(types) != len(BuiltinTypes)+len(seedTypes)+n/2 {
		t.Errorf("templates %d types %d; a concurrent write was lost", len(tmpls), len(types))
	}
}

func TestFindOrCreateTypeMatchesLoosely(t *testing.T) {
	s, _ := newMergeService(t)
	first, err := s.FindOrCreateType(config.UserCardType{Label: "Social Post"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.FindOrCreateType(config.UserCardType{Label: "  social POST "}, nil)
	if err != nil || again != first {
		t.Errorf("second lookup = %q (%v), want %q", again, err, first)
	}
	if id, _ := s.FindOrCreateType(config.UserCardType{Label: "task"}, nil); id != "task" {
		t.Errorf("built-in lookup = %q, want task", id)
	}
}
