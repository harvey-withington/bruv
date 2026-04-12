package repo

import (
	"bruv/internal/model"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Per-field caps applied to tool-call payloads before they're written to
// disk. Without these, an agent that writes large files (e.g. a news
// scanner dumping web-fetch results to disk) can balloon the agent.json
// file into megabytes — every GetAgentConfig call then drags that whole
// blob across the Wails bridge and through trace logging.
const (
	maxToolInputChars  = 800
	maxToolResultChars = 400
)

// sanitizeToolCalls returns a copy of the tool actions with oversized
// Input and Result fields truncated. This is a one-way operation: the
// full payloads are not preserved. Intended to be applied just before
// persisting a run so the stored history stays small.
func sanitizeToolCalls(actions []model.ToolAction) []model.ToolAction {
	if len(actions) == 0 {
		return actions
	}
	out := make([]model.ToolAction, len(actions))
	for i, a := range actions {
		out[i] = model.ToolAction{
			Tool:   a.Tool,
			Input:  truncateAny(a.Input, maxToolInputChars),
			Result: truncateString(a.Result, maxToolResultChars),
		}
	}
	return out
}

// truncateString shortens s to maxLen characters, appending an ellipsis
// marker so consumers can tell the value was trimmed. Empty strings are
// returned as-is.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "… (truncated)"
}

// truncateAny renders v as JSON and truncates it if the result is too
// long. The returned value is always safe to re-marshal. For small values
// the original is returned unchanged so typed data (objects, numbers)
// keeps its shape in the UI.
func truncateAny(v any, maxLen int) any {
	if v == nil {
		return nil
	}
	buf, err := json.Marshal(v)
	if err != nil || len(buf) <= maxLen {
		return v
	}
	return truncateString(string(buf), maxLen)
}

// agentFilePath returns the path to a card's agent config file.
func (r *Repository) agentFilePath(cardID string) string {
	return filepath.Join(r.Root, cardsDir, cardID+".agent.json")
}

// GetAgentConfig retrieves the agent configuration for a card.
// Returns a default AgentFile if no agent config file exists yet.
//
// Historical run data is sanitized on read so the returned payload
// never carries the full tool-call bodies that older versions stored.
// This is important both for bridge payload size and for log noise
// when Wails trace-logs JSON results.
func (r *Repository) GetAgentConfig(cardID string) (*model.AgentFile, error) {
	path := r.agentFilePath(cardID)
	if !fileExists(path) {
		return &model.AgentFile{
			CardID: cardID,
			Config: model.AgentConfig{
				Status:       model.AgentStatusDisabled,
				AllowedTools: []string{},
			},
			Runs: []model.AgentRun{},
		}, nil
	}

	var af model.AgentFile
	if err := readJSON(path, &af); err != nil {
		return nil, fmt.Errorf("read agent file for card %q: %w", cardID, err)
	}
	// Sanitize historical runs and compact the on-disk file if it
	// contains pre-cap bloat. We compare the serialized size of the
	// sanitized file to the file's current size; if the sanitized
	// version is meaningfully smaller, we rewrite the file so the
	// next read (and the next Wails trace log) is fast.
	origSize := int64(0)
	if fi, err := os.Stat(path); err == nil {
		origSize = fi.Size()
	}
	for i := range af.Runs {
		af.Runs[i].ToolCalls = sanitizeToolCalls(af.Runs[i].ToolCalls)
		af.Runs[i].Summary = truncateString(af.Runs[i].Summary, 600)
		af.Runs[i].Error = truncateString(af.Runs[i].Error, 600)
	}
	if origSize > 0 {
		if sanitized, err := json.MarshalIndent(af, "", "  "); err == nil {
			// Only rewrite if we actually saved at least 20% or 8KB —
			// avoids churning files that were already close to the cap.
			saved := origSize - int64(len(sanitized))
			if saved > 8*1024 || (origSize > 0 && saved*5 > origSize) {
				_ = writeJSON(path, af)
			}
		}
	}
	return &af, nil
}

// SaveAgentConfig persists the agent configuration for a card.
func (r *Repository) SaveAgentConfig(cardID string, config model.AgentConfig) error {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}
	af.CardID = cardID
	af.Config = config
	return writeJSON(r.agentFilePath(cardID), af)
}

// AppendAgentRun adds a run to a card's agent history, keeping the most recent 50.
// Tool call payloads are truncated before persistence to keep the agent.json
// file from ballooning when an agent writes large content via tools.
func (r *Repository) AppendAgentRun(cardID string, run model.AgentRun) error {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}

	if run.ID == "" {
		run.ID = uuid.New().String()[:8]
	}
	run.CardID = cardID
	run.ToolCalls = sanitizeToolCalls(run.ToolCalls)
	run.Summary = truncateString(run.Summary, 600)
	run.Error = truncateString(run.Error, 600)

	// Prepend (newest first)
	af.Runs = append([]model.AgentRun{run}, af.Runs...)

	// Trim to 50 runs
	const maxRuns = 50
	if len(af.Runs) > maxRuns {
		af.Runs = af.Runs[:maxRuns]
	}

	return writeJSON(r.agentFilePath(cardID), af)
}

// GetAgentRuns returns the run history for a card's agent.
func (r *Repository) GetAgentRuns(cardID string) ([]model.AgentRun, error) {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return nil, err
	}
	return af.Runs, nil
}

// ClearAgentRuns removes all run history for a card's agent, preserving config.
func (r *Repository) ClearAgentRuns(cardID string) error {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}
	af.Runs = []model.AgentRun{}
	return writeJSON(r.agentFilePath(cardID), af)
}

// DeleteAgentFile removes a card's agent config file. No error if the file doesn't exist.
func (r *Repository) DeleteAgentFile(cardID string) error {
	err := os.Remove(r.agentFilePath(cardID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent file for card %q: %w", cardID, err)
	}
	return nil
}
