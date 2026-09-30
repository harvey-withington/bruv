package fsutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Concurrent writers of one file each get their own temp file, so the
// result is always exactly one writer's complete payload — never a mix —
// even while readers keep the destination open (which on Windows makes
// the rename fail transiently until the retry succeeds).
func TestWriteFileAtomicConcurrentWritersAndReaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "card.json")
	if err := WriteFileAtomic(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := func(i int) []byte {
		return bytes.Repeat([]byte(fmt.Sprintf("%02d", i)), 4096)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 2; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if f, err := os.Open(path); err == nil {
					time.Sleep(time.Millisecond)
					f.Close()
				}
				time.Sleep(3 * time.Millisecond)
			}
		}()
	}

	const writers = 20
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := WriteFileAtomic(path, payload(i), 0o644); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	valid := false
	for i := 0; i < writers; i++ {
		if bytes.Equal(got, payload(i)) {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("final file is not any single writer's payload (%d bytes)", len(got))
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestWriteFileAtomicCreatesDirAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "a.json")
	for _, s := range []string{"one", "two"} {
		if err := WriteFileAtomic(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Errorf("content = %q, want two", got)
	}
}

func TestKeyedMutexSerializesPerKey(t *testing.T) {
	var km KeyedMutex
	counter := 0
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := km.Lock("k")
			v := counter
			time.Sleep(10 * time.Microsecond)
			counter = v + 1
			unlock()
		}()
	}
	wg.Wait()
	if counter != 50 {
		t.Errorf("counter = %d, want 50", counter)
	}

	// Different keys don't block each other, and unused entries are
	// dropped.
	unlockA := km.Lock("a")
	done := make(chan struct{})
	go func() { km.Lock("b")(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lock on key b blocked behind key a")
	}
	unlockA()
	if n := len(km.locks); n != 0 {
		t.Errorf("%d lock entries retained, want 0", n)
	}
}
