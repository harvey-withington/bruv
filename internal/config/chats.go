package config

// Per-user, per-repo chat history storage.
//
// Chat is conversation data between the user and an LLM — it is personal
// and must not travel with a shared repo. Storing it in the OS config
// folder keyed by the stable repo ID (from manifest.json) keeps Alice's
// chat from leaking to Bob when she shares her repo, while still letting
// each user have their own chat history per repo on their own machine.
//
// File layout under the config directory:
//
//   chats/<repoID>/<chatID>.messages.json
//
// where <chatID> is either a real card ID (for card-level chats) or a
// synthetic __project__<projectID> string (for project-level chats).
// The repo layer used to maintain that distinction; now it flows through
// here unchanged — same filenames, just a different root.

import (
	"bruv/internal/model"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// chatsMu serializes every chat write. UpdateChat (and AppendChatMessage /
// ToggleChatBookmark on top of it) is load → modify → save; two
// concurrent writers (user send racing an agent reply, a pending-edit
// resolution racing either) would otherwise drop one change.
var chatsMu sync.Mutex

// chatsDirForRepo returns the directory where a repo's chat files live,
// creating it if necessary. All reads and writes funnel through here so
// the path construction stays in one place.
func chatsDirForRepo(repoID string) (string, error) {
	if err := validPathSegment(repoID); err != nil {
		return "", fmt.Errorf("chat storage: %w", err)
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "chats", repoID)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// chatFilePathFor returns the on-disk location for one chat file.
func chatFilePathFor(repoID, chatID string) (string, error) {
	if err := validPathSegment(chatID); err != nil {
		return "", fmt.Errorf("chat storage: %w", err)
	}
	dir, err := chatsDirForRepo(repoID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, chatID+".messages.json"), nil
}

// LoadChatFor reads a chat file for the given repo/chat combination.
// Returns an empty ChatFile (not an error) when no chat exists yet —
// the same "create on demand" semantics the old in-repo API provided.
func LoadChatFor(repoID, chatID string) (*model.ChatFile, error) {
	path, err := chatFilePathFor(repoID, chatID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.ChatFile{
				CardID:   chatID,
				Messages: []model.ChatMessage{},
			}, nil
		}
		return nil, fmt.Errorf("read chat file %q: %w", chatID, err)
	}
	var cf model.ChatFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse chat file %q: %w", chatID, err)
	}
	return &cf, nil
}

// SaveChatFor persists the entire chat file to disk under the repo's
// chat directory. A blind replace, but taken under the same lock as
// every other chat writer so it can't interleave with a read-modify-write;
// anything that edits an existing chat should use UpdateChat instead.
func SaveChatFor(repoID string, cf *model.ChatFile) error {
	chatsMu.Lock()
	defer chatsMu.Unlock()
	return saveChatUnlocked(repoID, cf.CardID, cf)
}

// saveChatUnlocked writes cf as chatID's file. Caller holds chatsMu.
func saveChatUnlocked(repoID, chatID string, cf *model.ChatFile) error {
	path, err := chatFilePathFor(repoID, chatID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}

// ErrChatUnchanged, returned from an UpdateChat closure, means "nothing to
// save".
var ErrChatUnchanged = errors.New("chat unchanged")

// UpdateChat is the locked read-modify-write for one chat: it loads the
// current file (an empty one when none exists yet), applies fn and saves
// the result. An error from fn aborts without writing. Keep fn short —
// it runs under the lock every chat writer shares, so slow work (tool
// execution, LLM calls) belongs before the call, with fn only applying
// its outcome to the fresh copy. fn returning ErrChatUnchanged skips the
// write and returns the fresh copy with a nil error.
func UpdateChat(repoID, chatID string, fn func(cf *model.ChatFile) error) (*model.ChatFile, error) {
	chatsMu.Lock()
	defer chatsMu.Unlock()
	cf, err := LoadChatFor(repoID, chatID)
	if err != nil {
		return nil, err
	}
	if err := fn(cf); err != nil {
		if errors.Is(err, ErrChatUnchanged) {
			return cf, nil
		}
		return nil, err
	}
	if err := saveChatUnlocked(repoID, chatID, cf); err != nil {
		return nil, err
	}
	return cf, nil
}

// AppendChatMessage loads, appends, and saves in one step — the same
// convenience method the old repo.AppendMessage provided. The lock
// makes the load-append-save atomic with respect to other appends.
func AppendChatMessage(repoID, chatID string, msg model.ChatMessage) (*model.ChatFile, error) {
	return UpdateChat(repoID, chatID, func(cf *model.ChatFile) error {
		cf.Messages = append(cf.Messages, msg)
		return nil
	})
}

// ToggleChatBookmark flips the Bookmarked flag on one message and saves.
// Same locked load-modify-save shape as AppendChatMessage so a toggle
// can't race an append and drop either change. Returns the saved file.
func ToggleChatBookmark(repoID, chatID, messageID string) (*model.ChatFile, error) {
	return UpdateChat(repoID, chatID, func(cf *model.ChatFile) error {
		for i := range cf.Messages {
			if cf.Messages[i].ID == messageID {
				cf.Messages[i].Bookmarked = !cf.Messages[i].Bookmarked
				return nil
			}
		}
		return fmt.Errorf("message %q not found in chat %q", messageID, chatID)
	})
}

// DeleteChatFor removes a chat file. Missing files are not an error —
// the operation is idempotent so callers can use it as a cleanup hook
// without checking for existence first.
func DeleteChatFor(repoID, chatID string) error {
	path, err := chatFilePathFor(repoID, chatID)
	if err != nil {
		return err
	}
	chatsMu.Lock()
	defer chatsMu.Unlock()
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete chat file %q: %w", chatID, err)
	}
	return nil
}
