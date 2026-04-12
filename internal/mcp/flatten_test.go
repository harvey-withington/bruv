package mcp

import "testing"

// FlattenContent is the critical bridge between MCP's typed
// multi-modal results and BRUV's single-string tool-result interface.
// These tests lock in the rules documented in flatten.go so the
// behaviour stays predictable across future refactors.

func TestFlattenContentText(t *testing.T) {
	in := []Content{
		{Type: "text", Text: "hello"},
		{Type: "text", Text: "world"},
	}
	got := FlattenContent(in)
	if got != "hello\nworld" {
		t.Errorf("got %q, want %q", got, "hello\nworld")
	}
}

func TestFlattenContentEmpty(t *testing.T) {
	if got := FlattenContent(nil); got != "" {
		t.Errorf("nil content: got %q, want empty", got)
	}
	if got := FlattenContent([]Content{}); got != "" {
		t.Errorf("empty content: got %q, want empty", got)
	}
}

func TestFlattenContentImageIsPlaceholder(t *testing.T) {
	// Images are NOT embedded as base64 — they'd blow up LLM
	// context. Verify we get a concise placeholder with size info.
	in := []Content{
		{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"},
	}
	got := FlattenContent(in)
	// Exact format matters less than: contains "image", contains
	// mime type, and absolutely does NOT contain the base64 bytes.
	if !contains(got, "image") || !contains(got, "image/png") {
		t.Errorf("got %q, expected image placeholder with mime type", got)
	}
	if contains(got, "aGVsbG8=") {
		t.Errorf("got %q, must NOT embed base64 data in flattened output", got)
	}
}

func TestFlattenContentMissingMimeType(t *testing.T) {
	// Image with no mime type falls back to "unknown" — shouldn't
	// panic or emit a weird empty bracket pair.
	in := []Content{{Type: "image", Data: "xx"}}
	got := FlattenContent(in)
	if !contains(got, "unknown") {
		t.Errorf("got %q, expected 'unknown' fallback for missing mime type", got)
	}
}

func TestFlattenContentResourceLink(t *testing.T) {
	in := []Content{
		{Type: "resource_link", URI: "file:///foo.txt", Name: "foo"},
	}
	got := FlattenContent(in)
	if !contains(got, "file:///foo.txt") {
		t.Errorf("got %q, expected to contain the URI", got)
	}
}

func TestFlattenContentEmbeddedResource(t *testing.T) {
	in := []Content{
		{Type: "resource", Resource: &Resource{
			URI: "file:///a.txt", Text: "inline content",
		}},
	}
	got := FlattenContent(in)
	if got != "inline content" {
		t.Errorf("got %q, want inline resource text directly", got)
	}
}

func TestFlattenContentEmbeddedResourceBlob(t *testing.T) {
	// Resource with only a blob should get a placeholder, not
	// the base64.
	in := []Content{
		{Type: "resource", Resource: &Resource{
			URI: "file:///a.bin", MimeType: "application/octet-stream", Blob: "AAAA",
		}},
	}
	got := FlattenContent(in)
	if contains(got, "AAAA") {
		t.Errorf("got %q, must not embed blob bytes", got)
	}
	if !contains(got, "octet-stream") {
		t.Errorf("got %q, expected mime type in placeholder", got)
	}
}

func TestFlattenContentUnknownTypeFallback(t *testing.T) {
	// Unknown content types shouldn't cause a panic or empty
	// output — we emit something so the LLM at least knows
	// something happened.
	in := []Content{{Type: "future_type_we_dont_know_about"}}
	got := FlattenContent(in)
	if got == "" {
		t.Errorf("unknown content type should still produce output, got empty")
	}
}

func TestFlattenContentMixedTypes(t *testing.T) {
	// Real-world tool result: text intro, some inline text, and a
	// resource link. Should concatenate cleanly.
	in := []Content{
		{Type: "text", Text: "Here are your files:"},
		{Type: "resource_link", URI: "file:///a.txt"},
		{Type: "resource_link", URI: "file:///b.txt"},
	}
	got := FlattenContent(in)
	if !contains(got, "Here are your files") {
		t.Errorf("missing intro text: %q", got)
	}
	if !contains(got, "file:///a.txt") || !contains(got, "file:///b.txt") {
		t.Errorf("missing resource URIs: %q", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
