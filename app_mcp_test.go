package main

import (
	"strings"
	"testing"
)

func TestTruncateMCPOutput(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		wantTruncated  bool
		wantLenAtMost  int
		wantContainsIn string
	}{
		{
			name:           "short output passes through",
			input:          "ok",
			wantTruncated:  false,
			wantLenAtMost:  2,
			wantContainsIn: "ok",
		},
		{
			name:           "exactly at limit passes through",
			input:          strings.Repeat("x", mcpOutputLimit),
			wantTruncated:  false,
			wantLenAtMost:  mcpOutputLimit,
			wantContainsIn: "xxxx",
		},
		{
			name:          "one byte over gets truncated",
			input:         strings.Repeat("y", mcpOutputLimit+1),
			wantTruncated: true,
			// original bytes + the trailing marker
			wantLenAtMost:  mcpOutputLimit + 128,
			wantContainsIn: "[truncated:",
		},
		{
			name:           "huge payload capped",
			input:          strings.Repeat("z", 10*mcpOutputLimit),
			wantTruncated:  true,
			wantLenAtMost:  mcpOutputLimit + 128,
			wantContainsIn: "[truncated:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := truncateMCPOutput(tc.input)
			if truncated != tc.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tc.wantTruncated)
			}
			if len(got) > tc.wantLenAtMost {
				t.Errorf("output len = %d, want ≤ %d", len(got), tc.wantLenAtMost)
			}
			if !strings.Contains(got, tc.wantContainsIn) {
				t.Errorf("output missing %q; got first 80 chars: %q",
					tc.wantContainsIn, got[:min(80, len(got))])
			}
		})
	}
}
