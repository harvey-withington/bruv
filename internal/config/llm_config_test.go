package config

import "testing"

func TestPromptCacheTTLDefaults(t *testing.T) {
	for _, c := range []struct{ set, want string }{
		{"", PromptCacheShort},
		{"bogus", PromptCacheShort},
		{PromptCacheShort, PromptCacheShort},
		{PromptCacheLong, PromptCacheLong},
	} {
		if got := (LLMConfig{PromptCache: c.set}).PromptCacheTTL(); got != c.want {
			t.Errorf("PromptCache %q: PromptCacheTTL() = %q, want %q", c.set, got, c.want)
		}
	}
}
