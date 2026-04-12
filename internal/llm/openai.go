package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultOpenAIURL = "https://api.openai.com/v1"

type openaiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, baseURL string) Provider {
	if baseURL == "" {
		baseURL = defaultOpenAIURL
	}
	return &openaiProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *openaiProvider) Name() string { return "openai" }

func (p *openaiProvider) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	msgs := make([]map[string]any, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if m.Role == "tool" && m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range m.ToolCalls {
				argJSON, _ := json.Marshal(tc.Arguments)
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": string(argJSON),
					},
				})
			}
			msg["tool_calls"] = tcs
		}
		msgs = append(msgs, msg)
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := map[string]any{
		"model":      req.Model,
		"messages":   msgs,
		"max_tokens": maxTokens,
	}

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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{
			Provider:   "openai",
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Body:       truncate(string(respBody), 200),
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error (%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := result.Choices[0]
	cr := &ChatResponse{
		Content: choice.Message.Content,
		Model:   result.Model,
	}
	if result.Usage.TotalTokens > 0 {
		cr.Usage = &Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return cr, nil
}
