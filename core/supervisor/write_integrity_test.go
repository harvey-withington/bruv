package supervisor

// Read-modify-writes of one card / the card types store must not lose
// concurrent changes (pre-release sweep 2026-09-29, §3.1).

import (
	"sync"
	"testing"
)

func runConcurrently(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func TestAppendDeckSlideConcurrent(t *testing.T) {
	rt := newTestRuntime(t)
	deck := newDeckCard(t, rt)
	const n = 20
	runConcurrently(n, func(int) {
		if _, err := rt.AppendDeckSlide(deck.ID, "d1", map[string]any{"contentTypeId": "title"}); err != nil {
			t.Error(err)
		}
	})
	if got := len(deckSlides(t, rt, deck.ID)); got != n {
		t.Errorf("slides = %d, want %d", got, n)
	}
}

func TestSocialPostTypeMatchesExistingTypeLoosely(t *testing.T) {
	rt := newTestRuntime(t)
	existing, err := rt.CreateUserCardType("social post", "#000000", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.ensureSocialPostType(); got != existing.ID {
		t.Errorf("type = %q, want the existing %q", got, existing.ID)
	}
}

func TestSocialPostTypeProvisionedOnce(t *testing.T) {
	rt := newTestRuntime(t)
	runConcurrently(10, func(int) { rt.ensureSocialPostType() })
	n := 0
	for _, ct := range rt.ListCardTypes() {
		if ct.Label == socialPostTypeLabel {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d Social Post types, want 1", n)
	}
}
