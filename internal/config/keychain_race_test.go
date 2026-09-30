package config

import (
	"sync"
	"testing"
)

// Regression: the first card open fires several RPCs at once, each
// reaching KeychainAvailable. Unguarded, two probes raced — one deleted
// the shared probe entry mid-read (keychain wrongly judged unusable for
// the session) and both closed the same channel (panic → "Could not load
// agent configuration"). Every concurrent caller must get one answer and
// nothing may panic.
func TestKeychainAvailableConcurrentFirstUse(t *testing.T) {
	const callers = 16
	var wg sync.WaitGroup
	results := make([]bool, callers)
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = KeychainAvailable()
		}()
	}
	close(start)
	wg.Wait()
	for i, r := range results {
		if r != results[0] {
			t.Fatalf("caller %d saw %v, caller 0 saw %v — probes raced", i, r, results[0])
		}
	}
}
