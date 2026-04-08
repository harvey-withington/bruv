package config

import "testing"

func TestGetDefaultAccount(t *testing.T) {
	accounts := []LLMAccount{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B", IsDefault: true},
	}
	def := GetDefaultAccount(accounts)
	if def == nil || def.ID != "b" {
		t.Errorf("expected default account 'b', got %v", def)
	}

	// No default marked — should return first
	accounts2 := []LLMAccount{
		{ID: "x", Label: "X"},
		{ID: "y", Label: "Y"},
	}
	def2 := GetDefaultAccount(accounts2)
	if def2 == nil || def2.ID != "x" {
		t.Errorf("expected first account 'x' as fallback default, got %v", def2)
	}

	// Empty list
	def3 := GetDefaultAccount([]LLMAccount{})
	if def3 != nil {
		t.Errorf("expected nil for empty list, got %v", def3)
	}
}

func TestFindAccountByID(t *testing.T) {
	accounts := []LLMAccount{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
	}
	found := FindAccountByID(accounts, "b")
	if found == nil || found.Label != "B" {
		t.Errorf("expected to find account 'b', got %v", found)
	}
	notFound := FindAccountByID(accounts, "z")
	if notFound != nil {
		t.Errorf("expected nil for unknown ID, got %v", notFound)
	}
}

func TestEnsureOneDefault(t *testing.T) {
	accounts := []LLMAccount{
		{ID: "a", Label: "A"},
		{ID: "b", Label: "B"},
	}
	ensureOneDefault(accounts)
	if !accounts[0].IsDefault {
		t.Error("expected first account to become default")
	}

	// Already has default — should not change
	accounts2 := []LLMAccount{
		{ID: "x", Label: "X"},
		{ID: "y", Label: "Y", IsDefault: true},
	}
	ensureOneDefault(accounts2)
	if accounts2[0].IsDefault {
		t.Error("first account should not become default when another already is")
	}
	if !accounts2[1].IsDefault {
		t.Error("existing default should be preserved")
	}
}

func TestCapitalizeProvider(t *testing.T) {
	tests := map[string]string{
		"openai":    "OpenAI",
		"anthropic": "Anthropic",
		"ollama":    "Ollama",
		"custom":    "Custom",
		"":          "",
	}
	for in, want := range tests {
		got := capitalizeProvider(in)
		if got != want {
			t.Errorf("capitalizeProvider(%q) = %q, want %q", in, got, want)
		}
	}
}
