package config

// The chats' UNCLI sessions (the opt-in chat engine,
// internal/uncliengine): which session each chat is, and which repo and
// chat each session belongs to, so the engine can give a session that
// repo's board tools. A chat is a card's (its card id) or a project's
// (its chat id, chatsvc.ProjectChatID). One file for the machine, since
// the engine is one for the machine: <configDir>/uncli_sessions.json, a
// map of session ID → its chat.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// UncliChatHeader names the UNCLI session on the board-tool calls the
// engine's sessions make (internal/uncliengine sets it, internal/mcpserver
// reads it), so they're treated as that BRUV chat's: its Chat / Suggest /
// Edit mode applies. It can only restrict a caller, never widen access.
const UncliChatHeader = "X-Bruv-Uncli-Session"

// UncliChatMetaHeader carries, beside UncliChatHeader, the meta the page
// made the session with (shared/uncliChat.chatMeta), which names its chat
// from the start, before BRUV has recorded the session.
const UncliChatMetaHeader = "X-Bruv-Uncli-Chat"

// UncliChatModeRefusal is what a UNCLI chat's model is told when it tries
// to change the board in Chat mode (by UNCLI, from the engine's tool
// policy, and by BRUV's MCP endpoint behind it).
const UncliChatModeRefusal = "This chat is in Chat mode, so it can't change the board. " +
	"Describe the change instead, or ask the user to switch the chat to Suggest or Edit."

// UncliSession is the chat a UNCLI session is.
type UncliSession struct {
	RepoID string `json:"repo_id"`
	ChatID string `json:"chat_id"`
	// Number is which of the chat's sessions this is, counting from 1, so
	// a new one can be titled "<name> 2" (shared/uncliChat.chatTitle).
	// Records from before the count are the first.
	Number int `json:"number,omitempty"`
	// LegacyCardID is how the first builds (card chats only) named the
	// chat; read into ChatID, never written.
	LegacyCardID string `json:"card_id,omitempty"`
}

var uncliSessionsMu sync.Mutex

func uncliSessionsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "uncli_sessions.json"), nil
}

func loadUncliSessions() (map[string]UncliSession, error) {
	path, err := uncliSessionsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]UncliSession{}, nil
	}
	if err != nil {
		return nil, err
	}
	sessions := map[string]UncliSession{}
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}
	for id, s := range sessions {
		if s.ChatID == "" {
			s.ChatID, s.LegacyCardID = s.LegacyCardID, ""
			sessions[id] = s
		}
	}
	return sessions, nil
}

// ChatUncliSession returns the UNCLI session of a chat, or "" when it has
// none yet.
func ChatUncliSession(repoID, chatID string) (string, error) {
	uncliSessionsMu.Lock()
	defer uncliSessionsMu.Unlock()
	sessions, err := loadUncliSessions()
	if err != nil {
		return "", err
	}
	for id, s := range sessions {
		if s.RepoID == repoID && s.ChatID == chatID {
			return id, nil
		}
	}
	return "", nil
}

// ChatUncliSessionCount is how many UNCLI sessions a chat has had: the
// number of its current one, 0 before its first.
func ChatUncliSessionCount(repoID, chatID string) (int, error) {
	uncliSessionsMu.Lock()
	defer uncliSessionsMu.Unlock()
	sessions, err := loadUncliSessions()
	if err != nil {
		return 0, err
	}
	for _, s := range sessions {
		if s.RepoID == repoID && s.ChatID == chatID {
			return max(s.Number, 1), nil
		}
	}
	return 0, nil
}

// SetChatUncliSession records sessionID as the chat's, replacing any
// session it had before and numbering it after that one.
func SetChatUncliSession(repoID, chatID, sessionID string) error {
	for _, seg := range []string{repoID, chatID, sessionID} {
		if err := validPathSegment(seg); err != nil {
			return err
		}
	}
	uncliSessionsMu.Lock()
	defer uncliSessionsMu.Unlock()
	sessions, err := loadUncliSessions()
	if err != nil {
		return err
	}
	number := 1
	for id, s := range sessions {
		if s.RepoID == repoID && s.ChatID == chatID {
			if id == sessionID {
				number = max(s.Number, 1) // recorded again: same number
			} else {
				number = max(s.Number, 1) + 1
			}
			delete(sessions, id)
		}
	}
	sessions[sessionID] = UncliSession{RepoID: repoID, ChatID: chatID, Number: number}
	path, err := uncliSessionsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
}

// UncliSessionChat returns the chat a session is, as recorded by its page.
// A brand-new session's first CLI start may come before that record; the
// engine reads the page's meta for it instead (uncli.Core.SessionMeta).
func UncliSessionChat(sessionID string) (UncliSession, bool) {
	uncliSessionsMu.Lock()
	defer uncliSessionsMu.Unlock()
	sessions, err := loadUncliSessions()
	if err != nil {
		return UncliSession{}, false
	}
	s, ok := sessions[sessionID]
	return s, ok
}
