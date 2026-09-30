package card

import (
	"fmt"
	"sync"
	"testing"

	"bruv/internal/model"
	"bruv/internal/repo"
)

// Concurrent Edits of one card each land: every one runs on a fresh read
// under the card's lock (pre-release sweep 2026-09-29, §3.1).
func TestEditConcurrentKeepsEveryChange(t *testing.T) {
	s := newTestService(t)
	c, err := s.Create("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := s.Edit(c.ID, func(card *model.Card) error {
				card.Tags = append(card.Tags, fmt.Sprintf("t%d", i))
				card.Blocks = append(card.Blocks, model.Block{ID: fmt.Sprintf("b%d", i), Type: model.BlockText})
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, _ := s.Get(c.ID)
	if len(got.Tags) != n || len(got.Blocks) != n {
		t.Errorf("tags %d blocks %d, want %d each", len(got.Tags), len(got.Blocks), n)
	}
}

func TestEditReportsChangedParts(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")

	_, changed, err := s.Edit(c.ID, func(card *model.Card) error {
		card.Title = "New"
		card.Tags = []string{"a"}
		return nil
	})
	if err != nil || fmt.Sprint(changed) != "[title tags]" {
		t.Errorf("changed %v err %v", changed, err)
	}

	_, changed, err = s.Edit(c.ID, func(*model.Card) error { return repo.ErrNoChange })
	if err != nil || changed != nil {
		t.Errorf("no-op edit: changed %v err %v", changed, err)
	}

	if _, _, err := s.Edit(c.ID, func(card *model.Card) error { card.Type = "task"; return nil }); err == nil {
		t.Error("a type change through Edit must be refused")
	}
}
