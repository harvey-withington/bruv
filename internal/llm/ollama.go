package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultOllamaURL = "http://localhost:11434"

type ollamaProvider struct {
	baseURL string
	client  *http.Client
}

func NewOllama(baseURL string) Provider {
	if baseURL == "" {
		baseURL = defaultOllamaURL
	}
	return &ollamaProvider{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *ollamaProvider) Name() string { return "ollama" }

func (p *ollamaProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	msgs := make([]map[string]any, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		msgs = append(msgs, msg)
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": msgs,
		"stream":   false,
	}

	// Ollama supports tools for some models (llama3.1+, mistral, etc.)
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = tools
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API error (%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		Model           string `json:"model"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	cr := &ChatResponse{
		Content: result.Message.Content,
		Model:   result.Model,
	}
	if result.PromptEvalCount > 0 || result.EvalCount > 0 {
		total := result.PromptEvalCount + result.EvalCount
		cr.Usage = &Usage{
			PromptTokens:     result.PromptEvalCount,
			CompletionTokens: result.EvalCount,
			TotalTokens:      total,
		}
	}
	for i, tc := range result.Message.ToolCalls {
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:        fmt.Sprintf("ollama-%d", i),
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return cr, nil
}
