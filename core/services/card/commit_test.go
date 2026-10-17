package card

import (
	"encoding/base64"
	"sync"
	"testing"

	"bruv/internal/model"
)

func strPtr(s string) *string { return &s }

func TestEverySaveBumpsRev(t *testing.T) {
	s := newTestService(t)
	c, err := s.Create("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	start := c.Rev
	if _, err := s.UpdateTitle(c.ID, "Two"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(c.ID)
	if got.Rev != start+1 {
		t.Errorf("rev %d after one save, want %d", got.Rev, start+1)
	}
}

func TestCommitEditAppliesAtExpectedRev(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")
	res, err := s.CommitEdit(c.ID, c.Rev, model.CardEdit{
		Title:       strPtr("Offline title"),
		Description: strPtr("written on a plane"),
		DueDate:     strPtr("2026-11-01"),
		Tags:        &[]string{"blog"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict || res.Card.Title != "Offline title" || res.Card.Description != "written on a plane" || res.Card.DueDate == nil || len(res.Card.Tags) != 1 {
		t.Fatalf("commit not applied: %+v", res)
	}
	if res.Card.Rev != c.Rev+1 {
		t.Errorf("rev %d, want %d", res.Card.Rev, c.Rev+1)
	}

	// Clearing the due date: "" means clear, nil means leave alone.
	res, err = s.CommitEdit(c.ID, res.Card.Rev, model.CardEdit{DueDate: strPtr("")})
	if err != nil || res.Conflict || res.Card.DueDate != nil {
		t.Fatalf("clear due date: %+v %v", res, err)
	}
	if res.Card.Title != "Offline title" {
		t.Error("fields not in the edit must be left alone")
	}
}

func TestCommitEditRefusesAMovedRev(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")
	if _, err := s.UpdateDescription(c.ID, "edited at home"); err != nil {
		t.Fatal(err)
	}
	res, err := s.CommitEdit(c.ID, c.Rev, model.CardEdit{Description: strPtr("edited offline")})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Conflict {
		t.Fatal("a stale revision must be refused")
	}
	if res.Card.Description != "edited at home" || res.Card.Rev != c.Rev+1 {
		t.Errorf("conflict should return the current card, got %+v", res.Card)
	}
	got, _ := s.Get(c.ID)
	if got.Description != "edited at home" {
		t.Error("a refused commit must write nothing")
	}
}

func TestCommitEditRejectsBadInput(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")
	if _, err := s.CommitEdit(c.ID, c.Rev, model.CardEdit{DueDate: strPtr("next friday")}); err == nil {
		t.Error("an unparseable due date must be an error")
	}
	if _, err := s.CommitEdit(c.ID, c.Rev, model.CardEdit{Title: strPtr("  ")}); err == nil {
		t.Error("an empty title must be an error")
	}
}

// Only one of many commits racing from the same revision can win.
func TestCommitEditRaceHasOneWinner(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")
	const n = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := s.CommitEdit(c.ID, c.Rev, model.CardEdit{Description: strPtr(string(rune('a' + i)))})
			if err != nil {
				t.Error(err)
				return
			}
			if !res.Conflict {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("%d commits won from one revision, want 1", wins)
	}
}

func TestSaveAttachmentTextGuardsUnderLock(t *testing.T) {
	s := newTestService(t)
	c, _ := s.Create("", "Card")
	card, err := s.AddAttachment(c.ID, "draft.md", base64.StdEncoding.EncodeToString([]byte("v1")))
	if err != nil {
		t.Fatal(err)
	}
	attID := card.FileAttachments[0].ID
	open, err := s.OpenAttachmentText(c.ID, attID)
	if err != nil {
		t.Fatal(err)
	}
	const n = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	saved := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := s.SaveAttachmentText(c.ID, attID, "v2-"+string(rune('a'+i)), open.Stamp.Hash)
			if err != nil {
				t.Error(err)
				return
			}
			if !res.Diverged {
				mu.Lock()
				saved++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if saved != 1 {
		t.Errorf("%d guarded saves passed one stamp, want 1", saved)
	}
	if _, err := s.SaveAttachmentText(c.ID, "missing", "x", ""); err == nil {
		t.Error("saving an unknown attachment must be an error")
	}
}
