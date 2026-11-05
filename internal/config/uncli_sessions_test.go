package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUncliSessions(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })

	if id, err := ChatUncliSession("r1", "c1"); err != nil || id != "" {
		t.Fatalf("fresh chat: %q, %v", id, err)
	}
	if err := SetChatUncliSession("r1", "c1", "s1"); err != nil {
		t.Fatal(err)
	}
	// A new session replaces the chat's old one.
	if err := SetChatUncliSession("r1", "c1", "s2"); err != nil {
		t.Fatal(err)
	}
	if id, _ := ChatUncliSession("r1", "c1"); id != "s2" {
		t.Errorf("chat session = %q, want s2", id)
	}
	if _, ok := UncliSessionChat("s1"); ok {
		t.Error("the replaced session still names the chat")
	}
	if s, ok := UncliSessionChat("s2"); !ok || s != (UncliSession{RepoID: "r1", ChatID: "c1", Number: 2}) {
		t.Errorf("session chat = %+v, %v", s, ok)
	}
	if err := SetChatUncliSession("r1", "../c", "s3"); err == nil {
		t.Error("accepted a path-like chat id")
	}
}

// A chat's sessions are counted, so each new one can be numbered; the
// same session recorded again keeps its number.
func TestUncliSessionCount(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })
	count := func() int {
		t.Helper()
		n, err := ChatUncliSessionCount("r1", "c1")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 0 {
		t.Errorf("fresh chat count = %d", count())
	}
	for i, s := range []string{"s1", "s2", "s2", "s3"} {
		if err := SetChatUncliSession("r1", "c1", s); err != nil {
			t.Fatal(err)
		}
		if want := []int{1, 2, 2, 3}[i]; count() != want {
			t.Errorf("after %s: count = %d, want %d", s, count(), want)
		}
	}
	if n, _ := ChatUncliSessionCount("r1", "other"); n != 0 {
		t.Errorf("another chat's count = %d", n)
	}
}

// Sessions recorded by the first builds (card chats, "card_id") still
// name their chat.
func TestUncliSessionsLegacyCardID(t *testing.T) {
	dir := t.TempDir()
	SetConfigDir(dir)
	t.Cleanup(func() { SetConfigDir("") })
	legacy := `{"s1": {"repo_id": "r1", "card_id": "c1"}}`
	if err := os.WriteFile(filepath.Join(dir, "uncli_sessions.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if id, _ := ChatUncliSession("r1", "c1"); id != "s1" {
		t.Errorf("legacy chat session = %q, want s1", id)
	}
	if s, ok := UncliSessionChat("s1"); !ok || s.ChatID != "c1" {
		t.Errorf("legacy session chat = %+v, %v", s, ok)
	}
	if n, _ := ChatUncliSessionCount("r1", "c1"); n != 1 {
		t.Errorf("a legacy session counts as the first: %d", n)
	}
}
