package events

import (
	"sync"
	"testing"
	"time"
)

// TestPublishUnsubscribeStress hammers Publish from several goroutines
// while subscribers come and go. Before delivery moved under the lock,
// Publish snapshotted the subscriber list and sent after releasing it,
// so an unsubscribe in between closed a channel Publish then sent on:
// "send on closed channel" panicked and killed the process.
func TestPublishUnsubscribeStress(t *testing.T) {
	b := NewMemBus(4)
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.Publish("storm", nil)
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				_, unsub := b.Subscribe()
				unsub()
			}
		}()
	}

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestCloseEndsSubscriptions(t *testing.T) {
	b := NewMemBus(16)
	ch, unsub := b.Subscribe()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	b.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscriber range loop did not end after Close")
	}

	// Everything after Close is a safe no-op.
	unsub()
	b.Close()
	b.Publish("after-close", nil)
	late, lateUnsub := b.Subscribe()
	defer lateUnsub()
	if _, ok := <-late; ok {
		t.Fatal("Subscribe after Close should return a closed channel")
	}
}

// TestCloseDuringPublishStress races Close against publishers and
// subscribers; none of them may panic.
func TestCloseDuringPublishStress(t *testing.T) {
	for round := 0; round < 50; round++ {
		b := NewMemBus(2)
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				for j := 0; j < 200; j++ {
					b.Publish("t", j)
				}
			}()
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					_, unsub := b.Subscribe()
					unsub()
				}
			}()
		}
		b.Close()
		wg.Wait()
	}
}
