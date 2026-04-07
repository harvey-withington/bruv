package repo

import (
	"bruv/internal/model"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// agentFilePath returns the path to a card's agent config file.
func (r *Repository) agentFilePath(cardID string) string {
	return filepath.Join(r.Root, cardsDir, cardID+".agent.json")
}

// GetAgentConfig retrieves the agent configuration for a card.
// Returns a default AgentFile if no agent config file exists yet.
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
func (r *Repository) AppendAgentRun(cardID string, run model.AgentRun) error {
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		return err
	}

	if run.ID == "" {
		run.ID = uuid.New().String()[:8]
	}
	run.CardID = cardID

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

// DeleteAgentFile removes a card's agent config file. No error if the file doesn't exist.
func (r *Repository) DeleteAgentFile(cardID string) error {
	err := os.Remove(r.agentFilePath(cardID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete agent file for card %q: %w", cardID, err)
	}
	return nil
}
