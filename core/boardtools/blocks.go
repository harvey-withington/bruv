package boardtools

import (
	"encoding/json"
	"strings"
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
