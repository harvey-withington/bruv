package server

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunesKeepsCharactersWhole(t *testing.T) {
	if got := truncateRunes("short", 240); got != "short" {
		t.Errorf("short body changed: %q", got)
	}
	long := strings.Repeat("—é", 200) // multi-byte runes
	got := truncateRunes(long, 240)
	if !utf8.ValidString(got) {
		t.Fatal("truncation split a multi-byte character")
	}
	if n := utf8.RuneCountInString(got); n != 240 {
		t.Errorf("rune count = %d, want 240", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("truncated body should end with an ellipsis")
	}
}
