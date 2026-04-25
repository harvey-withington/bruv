package prompts

import (
	"strings"
	"testing"

	"bruv/core/services/card"
	"bruv/core/services/search"
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// stubDeps isolates the Builder from real services + repo for
// smoke-level tests. Proves the package can construct prompts given
// minimal, stub-able inputs — the key property headless agents rely
// on. Full integration coverage lives in the app-level tests that
// exercise the end-to-end chat loop.
type stubDeps struct {
	repo     *repo.Repository
	registry *schema.Registry
	card     *card.Service
	search   *search.Service
}

func (s stubDeps) Repo() *repo.Repository     { return s.repo }
func (s stubDeps) Registry() *schema.Registry { return s.registry }
func (s stubDeps) Card() *card.Service        { return s.card }
func (s stubDeps) Search() *search.Service    { return s.search }

func TestAgentPromptIncludesGoal(t *testing.T) {
	b := New(stubDeps{})
	out := b.Agent(
		&model.Card{Title: "test card", Type: "task"},
		model.AgentConfig{Goal: "do the thing"},
		config.LLMConfig{},
	)
	if !strings.Contains(out, "do the thing") {
		t.Errorf("agent prompt missing goal")
	}
	if !strings.Contains(out, "test card") {
		t.Errorf("agent prompt missing card title")
	}
}

func TestAgentPromptStillRunsWithoutRepo(t *testing.T) {
	// A Builder constructed with empty Deps must not panic when
	// generating an agent prompt for a card that has no repo
	// context — headless fixture tests rely on this.
	b := New(stubDeps{})
	out := b.Agent(&model.Card{Title: "bare"}, model.AgentConfig{}, config.LLMConfig{})
	if out == "" {
		t.Error("agent prompt unexpectedly empty")
	}
}
