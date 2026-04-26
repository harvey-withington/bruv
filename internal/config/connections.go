package config

// Per-machine known-connections list.
//
// A "connection" is a remote BRUV server the user has enrolled this
// device with: a friendly name, a URL, and a long-lived device token
// returned by /auth/enrol on the server. The active connection is
// what GetHTTPTransportInfo hands the frontend each session.
//
// The "Local" connection (this device's own loopback backend) is
// implicit — never stored, always available, used when Active == "".
// Removing every remote falls back to Local automatically.
//
// Storage: <clientdata>/connections.json. Per-device, never synced
// with a repo — "what servers do I know about" is a property of this
// machine, not of the data.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Connection is one remote server the user has enrolled with.
type Connection struct {
	ID          string    `json:"id"`           // stable UUID
	Name        string    `json:"name"`         // user-friendly label
	URL         string    `json:"url"`          // http(s)://host:port (no trailing slash)
	DeviceToken string    `json:"device_token"` // post-enrolment bearer token
	AddedAt     time.Time `json:"added_at"`
}

// ConnectionStore is the on-disk shape. Active="" means use the
// implicit Local connection.
type ConnectionStore struct {
	Active      string       `json:"active"`
	Connections []Connection `json:"connections"`
}

const connectionsFileName = "connections.json"

func connectionsFilePath() (string, error) {
	dir, err := ClientDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, connectionsFileName), nil
}

// LoadConnections reads the store. Returns an empty store (not an
// error) when the file doesn't exist — that's the normal state for
// any device that's never added a remote connection.
func LoadConnections() (ConnectionStore, error) {
	var s ConnectionStore
	path, err := connectionsFilePath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return ConnectionStore{}, err
	}
	// Defensive: drop dangling Active pointer if the connection is gone.
	if s.Active != "" && findConnection(s.Connections, s.Active) == nil {
		s.Active = ""
	}
	return s, nil
}

// SaveConnections writes the store atomically.
func SaveConnections(s ConnectionStore) error {
	path, err := connectionsFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddConnection persists a new connection and returns the stored
// entry (with generated ID and timestamp filled in). Validates that
// name + url + token are non-empty and that url doesn't already
// belong to another connection (avoids accidental duplicates).
func AddConnection(name, url, deviceToken string) (Connection, error) {
	name = trimSpace(name)
	url = trimTrailingSlash(trimSpace(url))
	deviceToken = trimSpace(deviceToken)
	if name == "" || url == "" || deviceToken == "" {
		return Connection{}, fmt.Errorf("name, url, and token are required")
	}

	store, err := LoadConnections()
	if err != nil {
		return Connection{}, err
	}
	for _, c := range store.Connections {
		if c.URL == url {
			return Connection{}, fmt.Errorf("a connection to %s already exists (named %q)", url, c.Name)
		}
	}

	c := Connection{
		ID:          uuid.NewString(),
		Name:        name,
		URL:         url,
		DeviceToken: deviceToken,
		AddedAt:     time.Now().UTC(),
	}
	store.Connections = append(store.Connections, c)
	if err := SaveConnections(store); err != nil {
		return Connection{}, err
	}
	return c, nil
}

// UpdateConnection edits an existing connection's name, URL, and/or
// device token. Pass empty strings for fields you don't want to
// change. The ID stays stable so per-machine state keyed off it
// (repo-recents.json, the active pointer) keeps working.
func UpdateConnection(id, name, url, deviceToken string) (Connection, error) {
	store, err := LoadConnections()
	if err != nil {
		return Connection{}, err
	}
	for i := range store.Connections {
		if store.Connections[i].ID != id {
			continue
		}
		if name != "" {
			store.Connections[i].Name = trimSpace(name)
		}
		if url != "" {
			store.Connections[i].URL = trimTrailingSlash(trimSpace(url))
		}
		if deviceToken != "" {
			store.Connections[i].DeviceToken = trimSpace(deviceToken)
		}
		if err := SaveConnections(store); err != nil {
			return Connection{}, err
		}
		return store.Connections[i], nil
	}
	return Connection{}, fmt.Errorf("connection %q not found", id)
}

// RemoveConnection drops an entry by ID. If the removed connection
// was the active one, Active is reset to "" (Local).
func RemoveConnection(id string) error {
	store, err := LoadConnections()
	if err != nil {
		return err
	}
	filtered := make([]Connection, 0, len(store.Connections))
	for _, c := range store.Connections {
		if c.ID == id {
			continue
		}
		filtered = append(filtered, c)
	}
	if len(filtered) == len(store.Connections) {
		return fmt.Errorf("connection %q not found", id)
	}
	store.Connections = filtered
	if store.Active == id {
		store.Active = ""
	}
	return SaveConnections(store)
}

// SetActiveConnection marks one connection (by ID) as active, or
// resets to Local if id is "".
func SetActiveConnection(id string) error {
	store, err := LoadConnections()
	if err != nil {
		return err
	}
	if id != "" && findConnection(store.Connections, id) == nil {
		return fmt.Errorf("connection %q not found", id)
	}
	store.Active = id
	return SaveConnections(store)
}

// ActiveConnection returns the resolved active entry, or nil when the
// active is Local (implicit). Use ResolvedActive on the store if you
// only have the store and not a fresh load.
func ActiveConnection() (*Connection, error) {
	store, err := LoadConnections()
	if err != nil {
		return nil, err
	}
	return store.ResolvedActive(), nil
}

// ResolvedActive returns the active connection from the in-memory
// store (or nil when Active is "" / unresolved).
func (s ConnectionStore) ResolvedActive() *Connection {
	if s.Active == "" {
		return nil
	}
	return findConnection(s.Connections, s.Active)
}

func findConnection(cs []Connection, id string) *Connection {
	for i := range cs {
		if cs[i].ID == id {
			return &cs[i]
		}
	}
	return nil
}

func trimSpace(s string) string {
	// Pulled out so future hardening (collapse internal whitespace
	// in names, lower-case hostnames in URLs) lives in one place.
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
