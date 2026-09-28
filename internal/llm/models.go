package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// ProviderOpenAICompatible is an OpenAI-shaped endpoint that is not
// OpenAI itself (OpenRouter, LM Studio, llama.cpp server, vLLM, Groq…).
// It talks through the OpenAI adapter; it is a separate kind so setup
// can require a base URL and treat the API key as optional.
const ProviderOpenAICompatible = "openai_compatible"

// DiscoveredModel is one model a provider reports it offers.
type DiscoveredModel struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"` // provider's display name, when it has one
}

// ListModels asks a provider which models it offers: GET /models for
// OpenAI and OpenAI-compatible endpoints, GET /v1/models for Anthropic,
// GET /api/tags for Ollama. Results are sorted by id.
func ListModels(ctx context.Context, provider, apiKey, baseURL string) ([]DiscoveredModel, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var (
		url     string
		headers = map[string]string{}
	)
	switch provider {
	case "openai", ProviderOpenAICompatible:
		if baseURL == "" {
			baseURL = defaultOpenAIURL
		}
		url = baseURL + "/models"
		if apiKey != "" {
			headers["Authorization"] = "Bearer " + apiKey
		}
	case "anthropic":
		if baseURL == "" {
			baseURL = defaultAnthropicURL
		}
		url = baseURL + "/v1/models?limit=1000"
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
	case "ollama":
		if baseURL == "" {
			baseURL = defaultOllamaURL
		}
		url = baseURL + "/api/tags"
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models (%d): %s", resp.StatusCode, truncate(string(body), 200))
	}

	models, err := parseModelList(provider, body)
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func parseModelList(provider string, body []byte) ([]DiscoveredModel, error) {
	var out []DiscoveredModel
	if provider == "ollama" {
		var r struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("parse model list: %w", err)
		}
		for _, m := range r.Models {
			out = append(out, DiscoveredModel{ID: m.Name})
		}
		return out, nil
	}
	// OpenAI, OpenAI-compatible and Anthropic share the {data:[{id}]}
	// shape; Anthropic adds display_name, OpenRouter adds name.
	var r struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Name        string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}
	for _, m := range r.Data {
		label := m.DisplayName
		if label == "" {
			label = m.Name
		}
		out = append(out, DiscoveredModel{ID: m.ID, Label: label})
	}
	return out, nil
}
