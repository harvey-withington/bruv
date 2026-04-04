package llm

import "testing"

func TestTruncateShorterThanMax(t *testing.T) {
	result := truncate("hello", 10)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

func TestTruncateExactLength(t *testing.T) {
	result := truncate("hello", 5)
	if result != "hello" {
		t.Errorf("expected %q, got %q", "hello", result)
	}
}

func TestTruncateLongerThanMax(t *testing.T) {
	result := truncate("hello world", 5)
	if result != "hello..." {
		t.Errorf("expected %q, got %q", "hello...", result)
	}
}

func TestTruncateEmptyString(t *testing.T) {
	result := truncate("", 5)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTruncateZeroMaxLen(t *testing.T) {
	result := truncate("hello", 0)
	if result != "..." {
		t.Errorf("expected %q, got %q", "...", result)
	}
}
