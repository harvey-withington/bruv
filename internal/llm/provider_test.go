package llm

import "testing"

func TestNewProviderOpenAI(t *testing.T) {
	p, err := NewProvider("openai", "test-key", "", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider("anthropic", "test-key", "", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q, want %q", p.Name(), "anthropic")
	}
}

func TestNewProviderOllama(t *testing.T) {
	p, err := NewProvider("ollama", "", "", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider("unknown", "", "", Options{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestNewProviderCustomBaseURL(t *testing.T) {
	p, err := NewProvider("openai", "key", "https://custom.api.com", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}
