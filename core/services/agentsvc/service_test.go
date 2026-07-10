package agentsvc

import (
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/index"
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
