package config

import (
	"testing"
	"time"
)

func TestUncliSessions(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })

	if id, err := CardUncliSession("r1", "c1"); err != nil || id != "" {
		t.Fatalf("fresh card: %q, %v", id, err)
	}
	if err := SetCardUncliSession("r1", "c1", "s1"); err != nil {
		t.Fatal(err)
	}
	// A new session replaces the card's old one.
	if err := SetCardUncliSession("r1", "c1", "s2"); err != nil {
		t.Fatal(err)
	}
	if id, _ := CardUncliSession("r1", "c1"); id != "s2" {
		t.Errorf("card session = %q, want s2", id)
	}
	if _, ok := UncliSessionCard("s1", 0); ok {
		t.Error("the replaced session still names the card")
	}
	if s, ok := UncliSessionCard("s2", 0); !ok || s != (UncliSession{RepoID: "r1", CardID: "c1"}) {
		t.Errorf("session card = %+v, %v", s, ok)
	}
	if err := SetCardUncliSession("r1", "../c", "s3"); err == nil {
		t.Error("accepted a path-like card id")
	}
}

// A session asked about before its card is recorded is answered once it is.
func TestUncliSessionCardWaits(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = SetCardUncliSession("r1", "c1", "s1")
	}()
	if s, ok := UncliSessionCard("s1", 5*time.Second); !ok || s.CardID != "c1" {
		t.Fatalf("got %+v, %v", s, ok)
	}
	if _, ok := UncliSessionCard("nope", 20*time.Millisecond); ok {
		t.Error("an unknown session was answered")
	}
}
