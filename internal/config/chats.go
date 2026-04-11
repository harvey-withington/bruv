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
	"fmt"
	"os"
	"path/filepath"
)

// chatsDirForRepo returns the directory where a repo's chat files live,
// creating it if necessary. All reads and writes funnel through here so
// the path construction stays in one place.
func chatsDirForRepo(repoID string) (string, error) {
	if repoID == "" {
		return "", fmt.Errorf("chat storage: repoID is required")
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
// chat directory.
func SaveChatFor(repoID string, cf *model.ChatFile) error {
	path, err := chatFilePathFor(repoID, cf.CardID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AppendChatMessage loads, appends, and saves in one step — the same
// convenience method the old repo.AppendMessage provided.
func AppendChatMessage(repoID, chatID string, msg model.ChatMessage) (*model.ChatFile, error) {
	cf, err := LoadChatFor(repoID, chatID)
	if err != nil {
		return nil, err
	}
	cf.Messages = append(cf.Messages, msg)
	if err := SaveChatFor(repoID, cf); err != nil {
		return nil, err
	}
	return cf, nil
}

// DeleteChatFor removes a chat file. Missing files are not an error —
// the operation is idempotent so callers can use it as a cleanup hook
// without checking for existence first.
func DeleteChatFor(repoID, chatID string) error {
	path, err := chatFilePathFor(repoID, chatID)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete chat file %q: %w", chatID, err)
	}
	return nil
}
