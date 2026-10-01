package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultAnthropicURL = "https://api.anthropic.com"

type anthropicProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewAnthropic(apiKey, baseURL string) Provider {
	if baseURL == "" {
		baseURL = defaultAnthropicURL
	}
	return &anthropicProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

// anthropicBlock is one content block of a Messages API response.
type anthropicBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// anthropicUsage is the usage object of a response or a stream event.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicMessage is a whole response, whether it arrived as one JSON
// body or was assembled from a stream.
type anthropicMessage struct {
	Content    []anthropicBlock `json:"content"`
	Model      string           `json:"model"`
	StopReason string           `json:"stop_reason"`
	Usage      anthropicUsage   `json:"usage"`
}

func (p *anthropicProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	msgs := make([]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "tool" {
			// Anthropic uses tool_result content blocks inside a "user" message
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
			continue
		}
		if len(m.ToolCalls) > 0 {
			// Assistant message with tool_use blocks
			var content []map[string]any
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Arguments,
				})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": content})
			continue
		}
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}

	// Always stream: a long thinking turn can run for minutes, which an
	// idle non-streaming connection may not survive, and closing a stream
	// stops generation (and billing) when the user presses Stop.
	body := map[string]any{
		"model":      req.Model,
		"messages":   msgs,
		"max_tokens": maxTokensOrDefault(req.MaxTokens),
		"stream":     true,
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			})
		}
		body["tools"] = tools
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 529 {
			return nil, &RateLimitError{
				Provider:   "anthropic",
				StatusCode: resp.StatusCode,
				RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
				Body:       truncate(string(respBody), 200),
			}
		}
		return nil, fmt.Errorf("Anthropic API error (%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// A proxy or compatible endpoint may ignore "stream" and answer with
	// one JSON body; both shapes assemble into the same message.
	var result *anthropicMessage
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		result, err = readAnthropicStream(resp.Body, req.OnOutput)
		if err != nil {
			return nil, err
		}
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		result = &anthropicMessage{}
		if err := json.Unmarshal(respBody, result); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
	}

	cr := &ChatResponse{Model: result.Model, StopReason: anthropicStopReason(result.StopReason)}
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		total := result.Usage.InputTokens + result.Usage.OutputTokens
		cr.Usage = &Usage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      total,
		}
	}
	// Thinking blocks are dropped: Claude's thinking is hidden (empty
	// text) on current models, and reasoning must never leak into the
	// visible answer.
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			cr.Content += block.Text
		case "tool_use":
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return cr, nil
}

// anthropicStopReason maps the Messages API stop_reason onto StopReason.
// Running out of context window is the same "ran out of room" as the
// output cap; anything unrecognised (pause_turn needs server tools,
// which BRUV doesn't send) counts as finished.
func anthropicStopReason(s string) StopReason {
	switch s {
	case "tool_use":
		return StopToolUse
	case "max_tokens", "model_context_window_exceeded":
		return StopMaxTokens
	case "refusal":
		return StopRefusal
	default:
		return StopEnd
	}
}
