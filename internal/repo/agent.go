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
	return filepath.Join(r.Root, cardsDir, safeSeg(cardID)+".agent.json")
}

// agentRunsFilePath returns the path to a card's agent run history.
// Empty string when RunsDir isn't configured — in that mode the
// repository falls back to the legacy merged-file behaviour where
// runs live inside the `.agent.json` alongside config.
//
// When set, runs live in <RunsDir>/<cardID>.json. That's under
// serverdata/runs/<repoID>/ in desktop mode — outside the repo tree,
// so git/Syncthing/Dropbox don't pick them up. The "I shared my
// repo with a friend and their BRUV history doesn't mix with mine"
// invariant depends on this split.
func (r *Repository) agentRunsFilePath(cardID string) string {
	if r.RunsDir == "" {
		return ""
	}
	return filepath.Join(r.RunsDir, safeSeg(cardID)+".json")
}

// SetRunsDir configures the server-side runs directory. Called by
// the host (app.go) after Open/Init with a path like
// <serverdata>/runs/<repoID>/. Ensures the directory exists.
//
// Nil/empty is a valid state — it means "use legacy merged storage".
// Tests that don't care about the split can omit this call.
func (r *Repository) SetRunsDir(dir string) error {
	if dir == "" {
		r.RunsDir = ""
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runs dir: %w", err)
	}
	r.RunsDir = dir
	return nil
}

// loadAgentRuns reads runs from the split runs file. Returns empty
// slice when RunsDir is unset, the file doesn't exist, or it's
// malformed — run history is non-authoritative, so failing loud
// here would just punish users whose history file corrupted.
func (r *Repository) loadAgentRuns(cardID string) []model.AgentRun {
	path := r.agentRunsFilePath(cardID)
	if path == "" {
		return nil
	}
	if !fileExists(path) {
		return nil
	}
	var file struct {
		CardID string           `json:"card_id"`
		Runs   []model.AgentRun `json:"runs"`
	}
	if err := readJSON(path, &file); err != nil {
		return nil
	}
	return file.Runs
}

// saveAgentRuns writes runs to the split runs file. No-op when
// RunsDir isn't configured — the caller is expected to write the
// merged legacy file in that case.
func (r *Repository) saveAgentRuns(cardID string, runs []model.AgentRun) error {
	path := r.agentRunsFilePath(cardID)
	if path == "" {
		return nil
	}
	payload := struct {
		CardID string           `json:"card_id"`
		Runs   []model.AgentRun `json:"runs"`
	}{CardID: cardID, Runs: runs}
	return writeJSON(path, payload)
}

// GetAgentConfig retrieves the agent configuration for a card.
// Returns a default AgentFile if no agent config file exists yet.
//
// Storage split: config lives in cards/<id>.agent.json (syncs with the
// repo); runs live in <RunsDir>/<id>.json (server-side, doesn't sync).
// Legacy .agent.json files with embedded runs are migrated inline on
// first read — both files are rewritten and the in-repo copy no
// longer carries run history.
//
// Historical run data is sanitized on read so the returned payload
// never carries the full tool-call bodies that older versions stored.
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

	// Migration: if the in-repo file carries embedded runs AND a
	// runs directory is configured, move the runs out. Rewrites the
	// in-repo file without runs so next read is clean. One-shot per
	// file — idempotent once migrated.
	migratedFromMerged := false
	if r.RunsDir != "" && len(af.Runs) > 0 {
		sideRuns := r.loadAgentRuns(cardID)
		if len(sideRuns) == 0 {
			// Nothing side-stored yet — the embedded runs ARE the
			// history. Split them out.
			if err := r.saveAgentRuns(cardID, af.Runs); err != nil {
				return nil, fmt.Errorf("migrate agent runs for card %q: %w", cardID, err)
			}
		}
		af.Runs = nil // drop from in-repo file
		migratedFromMerged = true
	}

	// Overlay server-side runs if the split is active.
	if r.RunsDir != "" {
		af.Runs = r.loadAgentRuns(cardID)
	}

	// Sanitize any runs we ended up with.
	origSize := int64(0)
	if fi, err := os.Stat(path); err == nil {
		origSize = fi.Size()
	}
	for i := range af.Runs {
		af.Runs[i].ToolCalls = sanitizeToolCalls(af.Runs[i].ToolCalls)
		af.Runs[i].Summary = truncateString(af.Runs[i].Summary, 600)
		af.Runs[i].Error = truncateString(af.Runs[i].Error, 600)
	}

	// Rewrite the in-repo file if the migration stripped runs, or
	// if the legacy compaction heuristic finds meaningful savings.
	if migratedFromMerged {
		configOnly := model.AgentFile{CardID: cardID, Config: af.Config}
		_ = writeJSON(path, configOnly)
	} else if origSize > 0 {
		if sanitized, err := json.MarshalIndent(af, "", "  "); err == nil {
			saved := origSize - int64(len(sanitized))
			if saved > 8*1024 || (origSize > 0 && saved*5 > origSize) {
				_ = writeJSON(path, af)
			}
		}
	}
	return &af, nil
}

// SaveAgentConfig persists the agent configuration for a card. When
// the runs-dir split is active, config is written alone; run history
// stays in its separate file untouched.
func (r *Repository) SaveAgentConfig(cardID string, config model.AgentConfig) error {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}
	af.CardID = cardID
	af.Config = config
	if r.RunsDir != "" {
		// Write config-only to the in-repo file.
		configOnly := model.AgentFile{CardID: cardID, Config: af.Config}
		return writeJSON(r.agentFilePath(cardID), configOnly)
	}
	return writeJSON(r.agentFilePath(cardID), af)
}

// AppendAgentRun adds a run to a card's agent history, keeping the
// most recent 50. Tool call payloads are truncated before persistence
// so disk usage stays bounded even with verbose agents.
func (r *Repository) AppendAgentRun(cardID string, run model.AgentRun) error {
	// Read existing runs (from split file if configured, else from
	// merged in-repo file via GetAgentConfig).
	var runs []model.AgentRun
	if r.RunsDir != "" {
		runs = r.loadAgentRuns(cardID)
	} else {
		af, err := r.GetAgentConfig(cardID)
		if err != nil {
			return err
		}
		runs = af.Runs
	}

	if run.ID == "" {
		run.ID = uuid.New().String()[:8]
	}
	run.CardID = cardID
	run.ToolCalls = sanitizeToolCalls(run.ToolCalls)
	run.Summary = truncateString(run.Summary, 600)
	run.Error = truncateString(run.Error, 600)

	runs = append([]model.AgentRun{run}, runs...)
	const maxRuns = 50
	if len(runs) > maxRuns {
		runs = runs[:maxRuns]
	}

	if r.RunsDir != "" {
		return r.saveAgentRuns(cardID, runs)
	}
	// Legacy merged-file path.
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}
	af.Runs = runs
	return writeJSON(r.agentFilePath(cardID), af)
}

// GetAgentRuns returns the run history for a card's agent.
func (r *Repository) GetAgentRuns(cardID string) ([]model.AgentRun, error) {
	if r.RunsDir != "" {
		return r.loadAgentRuns(cardID), nil
	}
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return nil, err
	}
	return af.Runs, nil
}

// ClearAgentRuns removes all run history for a card's agent,
// preserving config.
func (r *Repository) ClearAgentRuns(cardID string) error {
	if r.RunsDir != "" {
		return r.saveAgentRuns(cardID, []model.AgentRun{})
	}
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}
	af.Runs = []model.AgentRun{}
	return writeJSON(r.agentFilePath(cardID), af)
}

// DeleteAgentFile removes a card's agent config file AND its runs
// file. No error if either doesn't exist.
func (r *Repository) DeleteAgentFile(cardID string) error {
	if err := os.Remove(r.agentFilePath(cardID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent file for card %q: %w", cardID, err)
	}
	if runsPath := r.agentRunsFilePath(cardID); runsPath != "" {
		if err := os.Remove(runsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete agent runs file for card %q: %w", cardID, err)
		}
	}
	return nil
}
