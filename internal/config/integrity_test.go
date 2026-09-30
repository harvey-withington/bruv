package config

import (
	"bruv/internal/model"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Regression tests for the 2026-09-29 pre-release sweep: locked chat
// read-modify-write, workspace-checkout serialization, connections 0600.

// A slow resolution (tool execution between load and save) must not drop
// messages appended meanwhile: UpdateChat applies its change to the fresh
// copy it loads under the lock.
func TestUpdateChatKeepsConcurrentAppends(t *testing.T) {
	redirectConfig(t)
	const repoID, chatID = "repo-1", "card-1"
	if err := SaveChatFor(repoID, &model.ChatFile{CardID: chatID, Messages: []model.ChatMessage{
		{ID: "m1", PendingEdits: []model.PendingEdit{{ID: "e1", Status: "pending"}}},
	}}); err != nil {
		t.Fatal(err)
	}

	// A reply lands while the "tool" runs; the old code then saved its
	// pre-tool snapshot, dropping m2.
	if _, err := AppendChatMessage(repoID, chatID, model.ChatMessage{ID: "m2"}); err != nil {
		t.Fatal(err)
	}

	cf, err := UpdateChat(repoID, chatID, func(fresh *model.ChatFile) error {
		fresh.Messages[0].PendingEdits[0].Status = "accepted"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.Messages) != 2 || cf.Messages[0].PendingEdits[0].Status != "accepted" {
		t.Errorf("chat = %+v, want both messages and the edit accepted", cf.Messages)
	}
}

func TestUpdateChatConcurrentAppendsAllLand(t *testing.T) {
	redirectConfig(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, _ = AppendChatMessage("repo-1", "card-1", model.ChatMessage{ID: fmt.Sprintf("a%d", i)})
			} else {
				_, _ = UpdateChat("repo-1", "card-1", func(cf *model.ChatFile) error {
					cf.Messages = append(cf.Messages, model.ChatMessage{ID: fmt.Sprintf("u%d", i)})
					return nil
				})
			}
		}(i)
	}
	wg.Wait()
	cf, _ := LoadChatFor("repo-1", "card-1")
	if len(cf.Messages) != 30 {
		t.Errorf("messages = %d, want 30", len(cf.Messages))
	}
}

func TestUpdateChatUnchangedAndAbortSkipWrite(t *testing.T) {
	redirectConfig(t)
	path, _ := chatFilePathFor("repo-1", "card-x")

	if _, err := UpdateChat("repo-1", "card-x", func(*model.ChatFile) error { return ErrChatUnchanged }); err != nil {
		t.Fatalf("ErrChatUnchanged: %v", err)
	}
	boom := errors.New("boom")
	if _, err := UpdateChat("repo-1", "card-x", func(*model.ChatFile) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("abort: err = %v, want boom", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("no chat file should have been written")
	}
}

func TestSaveWorkspaceCheckoutConcurrent(t *testing.T) {
	redirectConfig(t)
	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = SaveWorkspaceCheckout(WorkspaceCheckout{ConnectionID: "local", RepoID: "r", WorkspaceID: fmt.Sprintf("ws%d", i), LocalPath: t.TempDir()})
		}(i)
	}
	wg.Wait()
	if got := len(ListWorkspaceCheckouts()); got != n {
		t.Errorf("checkouts = %d, want %d (lost records)", got, n)
	}
}

func TestConnectionsFileIsAtomicAndPrivate(t *testing.T) {
	redirectConfig(t)
	if _, err := AddConnection("Home", "https://example.invalid", "tok"); err != nil {
		t.Fatal(err)
	}
	path, err := connectionsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows only models the read-only bit; the mode check is POSIX-only.
	if os.PathSeparator == '/' && info.Mode().Perm() != 0o600 {
		t.Errorf("connections.json mode = %v, want 0600 (holds device tokens)", info.Mode().Perm())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}
