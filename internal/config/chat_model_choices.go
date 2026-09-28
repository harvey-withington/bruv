package config

// Per-chat model choice: the model or router a single chat uses instead
// of its task's assignment. Personal like the chat history itself, so it
// lives beside the chat files: chats/<repoID>/model_choices.json, a map
// of chatID → ModelRef.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var chatModelChoicesMu sync.Mutex

func chatModelChoicesPath(repoID string) (string, error) {
	dir, err := chatsDirForRepo(repoID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "model_choices.json"), nil
}

func loadChatModelChoices(repoID string) (map[string]ModelRef, error) {
	path, err := chatModelChoicesPath(repoID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]ModelRef{}, nil
	}
	if err != nil {
		return nil, err
	}
	choices := map[string]ModelRef{}
	if err := json.Unmarshal(data, &choices); err != nil {
		return nil, err
	}
	return choices, nil
}

// GetChatModelChoice returns the chat's own model choice, or "" when the
// chat inherits its task's assignment.
func GetChatModelChoice(repoID, chatID string) (ModelRef, error) {
	chatModelChoicesMu.Lock()
	defer chatModelChoicesMu.Unlock()
	choices, err := loadChatModelChoices(repoID)
	if err != nil {
		return "", err
	}
	return choices[chatID], nil
}

// SetChatModelChoice records the chat's model choice; "" clears it.
func SetChatModelChoice(repoID, chatID string, ref ModelRef) error {
	if err := validPathSegment(chatID); err != nil {
		return err
	}
	chatModelChoicesMu.Lock()
	defer chatModelChoicesMu.Unlock()
	choices, err := loadChatModelChoices(repoID)
	if err != nil {
		return err
	}
	if ref == "" {
		delete(choices, chatID)
	} else {
		choices[chatID] = ref
	}
	path, err := chatModelChoicesPath(repoID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(choices, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}
