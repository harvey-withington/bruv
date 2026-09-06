package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// DefaultPricing contains built-in pricing data (USD per million tokens,
// standard tier, uncached). Refreshed 2026-09-06 from the providers'
// published price lists. Older models stay listed so historical runs
// still cost out correctly; users can override any entry via the
// custom pricing file.
var DefaultPricing = map[string]ModelPricing{
	// OpenAI
	"gpt-5.5":      {InputPerMTok: 5.00, OutputPerMTok: 30.00},
	"gpt-5.4":      {InputPerMTok: 2.50, OutputPerMTok: 15.00},
	"gpt-5.4-mini": {InputPerMTok: 0.75, OutputPerMTok: 4.50},
	"gpt-5.4-nano": {InputPerMTok: 0.20, OutputPerMTok: 1.25},
	"gpt-5.2":      {InputPerMTok: 1.75, OutputPerMTok: 14.00},
	"gpt-5.1":      {InputPerMTok: 1.25, OutputPerMTok: 10.00},
	"gpt-5":        {InputPerMTok: 1.25, OutputPerMTok: 10.00},
	"gpt-5-mini":   {InputPerMTok: 0.25, OutputPerMTok: 2.00},
	"gpt-5-nano":   {InputPerMTok: 0.05, OutputPerMTok: 0.40},
	"gpt-4.1":      {InputPerMTok: 2.00, OutputPerMTok: 8.00},
	"gpt-4.1-mini": {InputPerMTok: 0.40, OutputPerMTok: 1.60},
	"gpt-4.1-nano": {InputPerMTok: 0.10, OutputPerMTok: 0.40},
	"gpt-4o":       {InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4o-mini":  {InputPerMTok: 0.15, OutputPerMTok: 0.60},
	// Anthropic
	"claude-fable-5-1":          {InputPerMTok: 10.00, OutputPerMTok: 50.00},
	"claude-opus-5":             {InputPerMTok: 5.00, OutputPerMTok: 25.00},
	"claude-opus-4-8":           {InputPerMTok: 5.00, OutputPerMTok: 25.00},
	"claude-sonnet-5":           {InputPerMTok: 2.00, OutputPerMTok: 10.00},
	"claude-sonnet-4-6":         {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-haiku-4-5":          {InputPerMTok: 1.00, OutputPerMTok: 5.00},
	"claude-sonnet-4-20250514":  {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-opus-4-20250514":    {InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-3-5-haiku-20241022": {InputPerMTok: 0.80, OutputPerMTok: 4.00},
	// Local
	"llama3.1": {InputPerMTok: 0.00, OutputPerMTok: 0.00},
	"llama3":   {InputPerMTok: 0.00, OutputPerMTok: 0.00},
}

// fallbackPricing is used when a model is not found in the pricing map.
var fallbackPricing = ModelPricing{InputPerMTok: 1.00, OutputPerMTok: 3.00}

// EstimateCost returns the estimated USD cost for a given model and token count.
// It uses a blended rate of 60% input / 40% output. If the model is not found
// in the pricing map, a fallback rate of $1.00/$3.00 per MTok is used.
func EstimateCost(model string, tokens int) float64 {
	pricing, ok := mergedPricing()[model]
	if !ok {
		pricing = fallbackPricing
	}
	blended := pricing.InputPerMTok*0.6 + pricing.OutputPerMTok*0.4
	return blended * float64(tokens) / 1_000_000.0
}

// mergedPricing returns DefaultPricing with any custom overrides applied.
func mergedPricing() map[string]ModelPricing {
	merged := make(map[string]ModelPricing, len(DefaultPricing))
	for k, v := range DefaultPricing {
		merged[k] = v
	}
	custom, err := loadCustomPricingFromDisk()
	if err != nil {
		return merged
	}
	for k, v := range custom {
		merged[k] = v
	}
	return merged
}

func pricingFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pricing.json"), nil
}

func loadCustomPricingFromDisk() (map[string]ModelPricing, error) {
	fp, err := pricingFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	var custom map[string]ModelPricing
	if err := json.Unmarshal(data, &custom); err != nil {
		return nil, err
	}
	return custom, nil
}

// The Get/SaveTokenPricing RPC surface was deleted 2026-07-10 (ruled:
// no in-app pricing editor). Hand-edited overrides in
// <configDir>/pricing.json are still merged by mergedPricing above.
