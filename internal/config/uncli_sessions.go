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
	"time"
)

// UncliChatHeader names the UNCLI session on the board-tool calls the
// engine's sessions make (internal/uncliengine sets it, internal/mcpserver
// reads it), so they're treated as that BRUV chat's: its Chat / Suggest /
// Edit mode applies. It can only restrict a caller, never widen access.
const UncliChatHeader = "X-Bruv-Uncli-Session"

// UncliSession is the chat a UNCLI session is.
type UncliSession struct {
	RepoID string `json:"repo_id"`
	ChatID string `json:"chat_id"`
	// LegacyCardID is how the first builds (card chats only) named the
	// chat; read into ChatID, never written.
	LegacyCardID string `json:"card_id,omitempty"`
}

var (
	uncliSessionsMu sync.Mutex
	// uncliSessionsChanged is closed (and replaced) on every write, for
	// UncliSessionChat's waiters. Guarded by uncliSessionsMu.
	uncliSessionsChanged = make(chan struct{})
)

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

// SetChatUncliSession records sessionID as the chat's, replacing any
// session it had before.
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
	for id, s := range sessions {
		if s.RepoID == repoID && s.ChatID == chatID {
			delete(sessions, id)
		}
	}
	sessions[sessionID] = UncliSession{RepoID: repoID, ChatID: chatID}
	path, err := uncliSessionsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWriteFile(path, data, 0o644); err != nil {
		return err
	}
	close(uncliSessionsChanged)
	uncliSessionsChanged = make(chan struct{})
	return nil
}

// UncliSessionChat returns the chat a session is, waiting up to wait for
// it to be recorded: a page records a new session's chat just after
// creating it, and UNCLI may start the session's CLI (asking for its
// tools) in between.
func UncliSessionChat(sessionID string, wait time.Duration) (UncliSession, bool) {
	deadline := time.After(wait)
	for {
		uncliSessionsMu.Lock()
		sessions, err := loadUncliSessions()
		changed := uncliSessionsChanged
		uncliSessionsMu.Unlock()
		if s, ok := sessions[sessionID]; err == nil && ok {
			return s, true
		}
		select {
		case <-changed:
		case <-deadline:
			return UncliSession{}, false
		}
	}
}
