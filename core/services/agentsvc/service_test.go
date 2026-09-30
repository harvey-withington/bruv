package agentsvc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	llmsvc "bruv/core/services/llm"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
)

type testDeps struct {
	r      *repo.Repository
	topics []string
}

func (d *testDeps) Repo() *repo.Repository      { return d.r }
func (d *testDeps) Index() *index.Index         { return nil }
func (d *testDeps) Publish(topic string, _ any) { d.topics = append(d.topics, topic) }
func (d *testDeps) LLM() *llmsvc.Service        { return nil }
func (d *testDeps) MCPRegistry() *mcp.Registry  { return nil }
func (d *testDeps) NativeToolDefs() []llm.ToolDef {
	return []llm.ToolDef{{Name: "get_card"}, {Name: "create_card"}, {Name: "update_card"}}
}

func (d *testDeps) emitted(topic string) bool {
	for _, t := range d.topics {
		if t == topic {
			return true
		}
	}
	return false
}

func newTestService(t *testing.T) (*Service, *testDeps) {
	t.Helper()
	r, err := repo.InitAt(filepath.Join(t.TempDir(), "vault"), "Vault")
	if err != nil {
		t.Fatal(err)
	}
	deps := &testDeps{r: r}
	return New(deps), deps
}

// TestDeleteLegacyMerged: with no runs dir configured, runs live inside
// the .agent.json itself — deleting the agent must drop config AND runs.
func TestDeleteLegacyMerged(t *testing.T) {
	svc, deps := newTestService(t)
	const cardID = "card-legacy"

	if err := svc.SaveConfig(cardID, model.AgentConfig{Goal: "g", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := deps.r.AppendAgentRun(cardID, model.AgentRun{ID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	deps.topics = nil
	if err := svc.Delete(cardID); err != nil {
		t.Fatal(err)
	}
	if !deps.emitted("card:updated") {
		t.Error("delete must publish card:updated")
	}

	// Config file gone → GetConfig falls back to the plain-card default.
	af, err := svc.GetConfig(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if af.Config.Enabled || af.Config.Goal != "" || len(af.Runs) != 0 {
		t.Errorf("agent not fully removed: %+v", af)
	}
}

// TestDeleteSplitStorage: with a runs dir configured, the side runs
// file must be removed along with the in-repo config file.
func TestDeleteSplitStorage(t *testing.T) {
	svc, deps := newTestService(t)
	runsDir := filepath.Join(t.TempDir(), "runs")
	if err := deps.r.SetRunsDir(runsDir); err != nil {
		t.Fatal(err)
	}
	const cardID = "card-split"

	if err := svc.SaveConfig(cardID, model.AgentConfig{Goal: "g", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := deps.r.AppendAgentRun(cardID, model.AgentRun{ID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	sidePath := filepath.Join(runsDir, cardID+".json")
	if _, err := os.Stat(sidePath); err != nil {
		t.Fatalf("side runs file missing before delete: %v", err)
	}

	if err := svc.Delete(cardID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidePath); !os.IsNotExist(err) {
		t.Errorf("side runs file still on disk: %v", err)
	}
	runs, err := svc.GetRuns(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("runs survived delete: %+v", runs)
	}
}

// TestSaveConfigKeepsRuntimeFields: the Agent tab saves a config it loaded
// before the runs since; the runtime-owned fields on disk must survive
// (pre-release sweep 2026-09-29, §2).
func TestSaveConfigKeepsRuntimeFields(t *testing.T) {
	svc, deps := newTestService(t)
	card, err := deps.r.CreateCard("brainstorm", "Agent")
	if err != nil {
		t.Fatal(err)
	}
	stale := model.AgentConfig{Enabled: true, Goal: "g", Schedule: "1d", OneShot: true, CostBudgetUSD: 1, CostSpentUSD: 0.2}
	if err := svc.SaveConfig(card.ID, stale); err != nil {
		t.Fatal(err)
	}

	// A run is in progress and earlier runs spent money.
	ran := time.Now().UTC().Add(-time.Hour)
	if _, err := deps.r.UpdateAgentConfig(card.ID, func(cfg *model.AgentConfig) error {
		cfg.Status, cfg.RunStartedAt = model.AgentStatusRunning, &ran
		cfg.LastRunAt, cfg.RetryCount, cfg.CostSpentUSD = &ran, 2, 0.9
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	stale.Goal = "edited"
	if err := svc.SaveConfig(card.ID, stale); err != nil {
		t.Fatal(err)
	}
	af, _ := svc.GetConfig(card.ID)
	got := af.Config
	if got.Goal != "edited" {
		t.Errorf("goal %q, want the edit", got.Goal)
	}
	if got.Status != model.AgentStatusRunning || got.RunStartedAt == nil {
		t.Errorf("a mid-run save changed status to %q", got.Status)
	}
	if got.LastRunAt == nil || got.RetryCount != 2 || got.CostSpentUSD != 0.9 {
		t.Errorf("runtime fields overwritten: %+v", got)
	}
	if got.NextRunAt != nil {
		t.Errorf("one-shot that already ran re-scheduled for %v", got.NextRunAt)
	}

	// Reset cost (the tab sends 0) is honoured.
	stale.CostSpentUSD = 0
	if err := svc.SaveConfig(card.ID, stale); err != nil {
		t.Fatal(err)
	}
	if af, _ := svc.GetConfig(card.ID); af.Config.CostSpentUSD != 0 {
		t.Errorf("cost reset ignored: %v", af.Config.CostSpentUSD)
	}
}

func TestPatchKeepsARunningAgentRunning(t *testing.T) {
	svc, deps := newTestService(t)
	card, _ := deps.r.CreateCard("brainstorm", "Agent")
	if err := svc.SaveConfig(card.ID, model.AgentConfig{Enabled: true, Goal: "g"}); err != nil {
		t.Fatal(err)
	}
	if _, err := deps.r.UpdateAgentConfig(card.ID, func(cfg *model.AgentConfig) error {
		cfg.Status = model.AgentStatusRunning
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	goal := "new"
	got, err := svc.Patch(card.ID, ConfigPatch{Goal: &goal})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.AgentStatusRunning || got.Goal != "new" {
		t.Errorf("patch result %+v", got)
	}
}
