package llm

import (
	"context"
	"fmt"
)

// Message is a single chat message for the LLM API.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role="tool" messages
}

// ToolDef describes a tool the LLM can call.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema object
}

// ToolCall is a single tool invocation returned by the LLM.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatRequest is sent to the provider.
type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Model        string
	MaxTokens    int       // 0 = provider default
	Tools        []ToolDef // optional; empty = no tool calling
}

// ChatResponse is returned from the provider.
type ChatResponse struct {
	Content   string
	Model     string
	ToolCalls []ToolCall // non-empty when the LLM wants to call tools
}

// Provider is the interface all LLM backends implement.
type Provider interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}

// NewProvider creates a provider by name.
func NewProvider(provider, apiKey, baseURL string) (Provider, error) {
	switch provider {
	case "openai":
		return NewOpenAI(apiKey, baseURL), nil
	case "anthropic":
		return NewAnthropic(apiKey, baseURL), nil
	case "ollama":
		return NewOllama(baseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
}
