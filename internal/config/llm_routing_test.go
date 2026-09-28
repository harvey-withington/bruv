package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelRefParse(t *testing.T) {
	cases := []struct {
		ref  ModelRef
		kind ModelRefKind
		id   string
	}{
		{"model:abc", RefModel, "abc"},
		{"router:r1", RefRouter, "r1"},
		{"tier:fast", RefTier, "fast"},
		{"", RefNone, ""},
		{"model:", RefNone, ""},
		{"bogus:x", RefNone, ""},
		{"noколон", RefNone, ""},
	}
	for _, c := range cases {
		kind, id := c.ref.Parse()
		if kind != c.kind || id != c.id {
			t.Errorf("%q.Parse() = (%q, %q), want (%q, %q)", c.ref, kind, id, c.kind, c.id)
		}
	}
	if got := ModelRefTo(RefRouter, "x"); got != "router:x" {
		t.Errorf("ModelRefTo = %q", got)
	}
}

func TestGuessModelTier(t *testing.T) {
	cases := map[string]ModelTier{
		"gpt-5.4-mini":     TierFast,
		"claude-haiku-4-5": TierFast,
		"llama3.1:8b":      TierFast,
		"claude-opus-5":    TierPowerful,
		"claude-fable-5-1": TierPowerful,
		"claude-sonnet-5":  TierBalanced,
		"gpt-5.5":          TierBalanced,
	}
	for name, want := range cases {
		if got := GuessModelTier(name); got != want {
			t.Errorf("GuessModelTier(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLoadLLMRoutingMigratesFromAccounts(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })

	accounts := []LLMAccount{
		{ID: "rt-anth", Label: "Anthropic", Provider: "anthropic", Model: "claude-sonnet-5"},
		{ID: "rt-olla", Label: "Local", Provider: "ollama", IsDefault: true},
	}
	if err := SaveLLMAccounts(accounts); err != nil {
		t.Fatal(err)
	}

	r, err := LoadLLMRouting()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(r.Models))
	}
	if r.Models[0].Name != "claude-sonnet-5" || r.Models[0].AccountID != "rt-anth" || !r.Models[0].Enabled || !r.Models[0].SupportsTools {
		t.Errorf("first model = %+v", r.Models[0])
	}
	if r.Models[1].Name != "llama3.1" {
		t.Errorf("ollama model = %q, want provider default llama3.1", r.Models[1].Name)
	}
	if r.Default != ModelRefTo(RefModel, r.Models[1].ID) {
		t.Errorf("default = %q, want the default account's model %q", r.Default, r.Models[1].ID)
	}

	// Second load reads the saved file rather than migrating again.
	r2, err := LoadLLMRouting()
	if err != nil {
		t.Fatal(err)
	}
	if r2.Models[0].ID != r.Models[0].ID {
		t.Error("migration ran twice: model ids changed")
	}
}

func TestChatModelChoiceRoundTrip(t *testing.T) {
	SetConfigDir(t.TempDir())
	t.Cleanup(func() { SetConfigDir("") })

	if err := SetChatModelChoice("repo1", "card1", "model:m1"); err != nil {
		t.Fatal(err)
	}
	got, err := GetChatModelChoice("repo1", "card1")
	if err != nil || got != "model:m1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := SetChatModelChoice("repo1", "card1", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := GetChatModelChoice("repo1", "card1"); got != "" {
		t.Errorf("cleared choice = %q", got)
	}
}

// The frontend indexes routing.tasks directly; an empty map must reach
// it as {} rather than being omitted (regression 2026-09-26).
func TestLLMRoutingJSONAlwaysHasTasks(t *testing.T) {
	r := LLMRouting{}
	normalizeRouting(&r)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tasks":{}`) {
		t.Errorf("tasks missing from %s", data)
	}
}
