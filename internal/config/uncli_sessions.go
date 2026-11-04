package config

// The card chats' UNCLI sessions (the opt-in chat engine,
// internal/uncliengine): which session each card's chat is, and which
// repo and card each session is about, so the engine can give a session
// that repo's board tools. One file for the machine, since the engine is
// one for the machine: <configDir>/uncli_sessions.json, a map of session
// ID → its card.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UncliSession is the card a UNCLI session is the chat of.
type UncliSession struct {
	RepoID string `json:"repo_id"`
	CardID string `json:"card_id"`
}

var (
	uncliSessionsMu sync.Mutex
	// uncliSessionsChanged is closed (and replaced) on every write, for
	// UncliSessionCard's waiters. Guarded by uncliSessionsMu.
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
	return sessions, nil
}

// CardUncliSession returns the UNCLI session of a card's chat, or "" when
// it has none yet.
func CardUncliSession(repoID, cardID string) (string, error) {
	uncliSessionsMu.Lock()
	defer uncliSessionsMu.Unlock()
	sessions, err := loadUncliSessions()
	if err != nil {
		return "", err
	}
	for id, s := range sessions {
		if s.RepoID == repoID && s.CardID == cardID {
			return id, nil
		}
	}
	return "", nil
}

// SetCardUncliSession records sessionID as the card's chat, replacing
// any session it had before.
func SetCardUncliSession(repoID, cardID, sessionID string) error {
	for _, seg := range []string{repoID, cardID, sessionID} {
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
		if s.RepoID == repoID && s.CardID == cardID {
			delete(sessions, id)
		}
	}
	sessions[sessionID] = UncliSession{RepoID: repoID, CardID: cardID}
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

// UncliSessionCard returns the card a session is the chat of, waiting up
// to wait for it to be recorded: a page records a new session's card just
// after creating it, and UNCLI may start the session's CLI (asking for
// its tools) in between.
func UncliSessionCard(sessionID string, wait time.Duration) (UncliSession, bool) {
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
