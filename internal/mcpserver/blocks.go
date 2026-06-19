package mcpserver

import (
	"encoding/json"
	"strings"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/model"

	"github.com/google/uuid"
)

// --- argument helpers (MCP tool arguments arrive as map[string]any) ---

func argStr(a map[string]any, key string) string {
	s, _ := a[key].(string)
	return strings.TrimSpace(s)
}

// argInt reads an integer argument. JSON numbers decode as float64.
func argInt(a map[string]any, key string, def int) int {
	switch v := a[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return def
}

func argStrSlice(a map[string]any, key string) []string {
	raw, ok := a[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// parseBlocks converts the MCP block argument shape ({type,label,value,
// key?}) into model.Block values, coercing each value through the same
// path the internal AI chat uses so behaviour is identical (checklists
// become arrays, dates normalise, etc.). Blocks get a fresh id.
func parseBlocks(raw any) []model.Block {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]model.Block, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := m["type"].(string)
		blockType = strings.TrimSpace(blockType)
		if blockType == "" {
			blockType = model.BlockText
		}
		label, _ := m["label"].(string)
		key, _ := m["key"].(string)
		b := model.Block{
			ID:    "blk-" + uuid.New().String()[:8],
			Type:  blockType,
			Label: strings.TrimSpace(label),
			Key:   strings.TrimSpace(key),
			Value: m["value"],
		}
		// CoerceBlockValueForBlock returns the best-effort coerced value
		// even when it reports a constraint violation, so we always take
		// the coerced form and ignore the advisory error.
		if coerced, _ := cardtools.CoerceBlockValueForBlock(&b, b.Value); coerced != nil {
			b.Value = coerced
		}
		out = append(out, b)
	}
	return out
}
