package llm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RateLimitError is returned when a provider responds with HTTP 429
// (or an equivalent rate-limit signal). RetryAfter is the suggested
// minimum wait before retrying, parsed from the Retry-After header
// when present. Callers can type-assert with errors.As to apply a
// longer backoff instead of the generic retry schedule.
type RateLimitError struct {
	Provider   string
	StatusCode int
	RetryAfter time.Duration // 0 if the server didn't send a hint
	Body       string        // truncated response body for logging
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s rate limit (HTTP %d, retry after %s): %s", e.Provider, e.StatusCode, e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("%s rate limit (HTTP %d): %s", e.Provider, e.StatusCode, e.Body)
}

// IsRateLimitError reports whether err (or anything it wraps) is a *RateLimitError.
func IsRateLimitError(err error) bool {
	var rle *RateLimitError
	return errors.As(err, &rle)
}

// AsRateLimitError unwraps err to a *RateLimitError, returning nil if not one.
func AsRateLimitError(err error) *RateLimitError {
	var rle *RateLimitError
	if errors.As(err, &rle) {
		return rle
	}
	return nil
}

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

// Usage reports token consumption for a single LLM call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is returned from the provider.
type ChatResponse struct {
	Content   string
	Model     string
	ToolCalls []ToolCall // non-empty when the LLM wants to call tools
	Usage     *Usage     // token usage; nil if provider doesn't report it
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
