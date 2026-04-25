package repo

import (
	"bruv/internal/model"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAgentSplitStorageBasics covers the happy path when a runs dir
// is configured from the start: config goes to the in-repo file,
// runs go to the side file, and both are read back merged.
func TestAgentSplitStorageBasics(t *testing.T) {
	r := setupTestRepo(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	if err := r.SetRunsDir(runsDir); err != nil {
		t.Fatalf("SetRunsDir: %v", err)
	}

	cardID := "test-card-1"
	if err := r.SaveAgentConfig(cardID, model.AgentConfig{
		Goal:    "write a haiku",
		Enabled: true,
	}); err != nil {
		t.Fatalf("SaveAgentConfig: %v", err)
	}
	if err := r.AppendAgentRun(cardID, model.AgentRun{
		ID: "run-1", Summary: "first run",
	}); err != nil {
		t.Fatalf("AppendAgentRun: %v", err)
	}

	// The in-repo file must NOT contain runs — that's the whole point
	// of the split.
	inRepoPath := r.agentFilePath(cardID)
	data, err := os.ReadFile(inRepoPath)
	if err != nil {
		t.Fatalf("read in-repo file: %v", err)
	}
	var inRepo model.AgentFile
	if err := json.Unmarshal(data, &inRepo); err != nil {
		t.Fatalf("unmarshal in-repo: %v", err)
	}
	if len(inRepo.Runs) != 0 {
		t.Errorf("in-repo .agent.json still contains %d runs, want 0", len(inRepo.Runs))
	}
	if inRepo.Config.Goal != "write a haiku" {
		t.Errorf("in-repo config lost; goal = %q", inRepo.Config.Goal)
	}

	// The side file must contain the runs.
	sidePath := r.agentRunsFilePath(cardID)
	if sidePath == "" {
		t.Fatal("agentRunsFilePath empty despite SetRunsDir")
	}
	if _, err := os.Stat(sidePath); err != nil {
		t.Errorf("side runs file missing: %v", err)
	}

	// Merged read surfaces both.
	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if af.Config.Goal != "write a haiku" {
		t.Errorf("merged config wrong; goal = %q", af.Config.Goal)
	}
	if len(af.Runs) != 1 || af.Runs[0].ID != "run-1" {
		t.Errorf("merged runs wrong; got %+v", af.Runs)
	}
}

// TestAgentMigrationFromLegacyMerged covers the interesting case: an
// existing repo has `.agent.json` with embedded runs from a pre-split
// version. On first GetAgentConfig call with a runs dir configured,
// the runs must migrate out — and the in-repo file must be rewritten
// without them so the next commit / git pull doesn't sync them.
func TestAgentMigrationFromLegacyMerged(t *testing.T) {
	r := setupTestRepo(t)

	cardID := "legacy-card"

	// Write a legacy merged file BEFORE configuring the runs dir,
	// mimicking a pre-split install.
	legacy := model.AgentFile{
		CardID: cardID,
		Config: model.AgentConfig{Goal: "legacy goal", Enabled: true},
		Runs: []model.AgentRun{
			{ID: "old-1", Summary: "run from before"},
			{ID: "old-2", Summary: "another run"},
		},
	}
	if err := writeJSON(r.agentFilePath(cardID), legacy); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	// Now configure runs dir and read — this should migrate.
	runsDir := filepath.Join(t.TempDir(), "runs")
	if err := r.SetRunsDir(runsDir); err != nil {
		t.Fatalf("SetRunsDir: %v", err)
	}

	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}

	// Merged read must still show the runs.
	if len(af.Runs) != 2 {
		t.Errorf("merged read lost runs during migration; got %d", len(af.Runs))
	}

	// In-repo file must now be config-only.
	data, err := os.ReadFile(r.agentFilePath(cardID))
	if err != nil {
		t.Fatalf("re-read in-repo: %v", err)
	}
	var after model.AgentFile
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatalf("unmarshal after: %v", err)
	}
	if len(after.Runs) != 0 {
		t.Errorf("in-repo file still has %d runs after migration", len(after.Runs))
	}
	if after.Config.Goal != "legacy goal" {
		t.Errorf("config lost during migration; goal = %q", after.Config.Goal)
	}

	// Side file must now hold the runs.
	sidePath := r.agentRunsFilePath(cardID)
	if _, err := os.Stat(sidePath); err != nil {
		t.Errorf("side file not created during migration: %v", err)
	}
}

// TestAgentLegacyModeStillWorks covers the no-runs-dir path: when
// SetRunsDir is never called, the legacy merged-file layout is
// preserved. Tests that don't care about the split stay green.
func TestAgentLegacyModeStillWorks(t *testing.T) {
	r := setupTestRepo(t)
	// Deliberately NOT calling SetRunsDir.

	cardID := "legacy-card"
	if err := r.SaveAgentConfig(cardID, model.AgentConfig{Goal: "merged"}); err != nil {
		t.Fatalf("SaveAgentConfig: %v", err)
	}
	if err := r.AppendAgentRun(cardID, model.AgentRun{ID: "r1"}); err != nil {
		t.Fatalf("AppendAgentRun: %v", err)
	}

	af, err := r.GetAgentConfig(cardID)
	if err != nil {
		t.Fatalf("GetAgentConfig: %v", err)
	}
	if af.Config.Goal != "merged" || len(af.Runs) != 1 {
		t.Errorf("legacy mode broken; config=%+v runs=%+v", af.Config, af.Runs)
	}
}

// TestAgentClearRunsSplit confirms that clearing runs when the split
// is active touches only the side file, not the in-repo config.
func TestAgentClearRunsSplit(t *testing.T) {
	r := setupTestRepo(t)
	_ = r.SetRunsDir(filepath.Join(t.TempDir(), "runs"))

	cardID := "clear-me"
	_ = r.SaveAgentConfig(cardID, model.AgentConfig{Goal: "keep this"})
	_ = r.AppendAgentRun(cardID, model.AgentRun{ID: "doomed"})

	if err := r.ClearAgentRuns(cardID); err != nil {
		t.Fatalf("ClearAgentRuns: %v", err)
	}

	af, _ := r.GetAgentConfig(cardID)
	if len(af.Runs) != 0 {
		t.Errorf("runs not cleared; got %d", len(af.Runs))
	}
	if af.Config.Goal != "keep this" {
		t.Errorf("config wiped during run clear; goal = %q", af.Config.Goal)
	}
}
