package chat

import (
	"context"
	"sync"
	"testing"

	"bruv/core/runtime/prompts"
	"bruv/core/runtime/tools"
	"bruv/core/services/catalog"
	"bruv/core/services/card"
	llmsvc "bruv/core/services/llm"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// stubDeps exercises the Runtime construction path without
// requiring real services. Proves the chat package can be
// instantiated from arbitrary hosts (desktop App or the future
// headless cmd/bruv-server), which is the key affordance stage 3
// delivers.
type stubDeps struct {
	actors sync.Map
}

func (s *stubDeps) Repo() *repo.Repository     { return nil }
func (s *stubDeps) Registry() *schema.Registry { return nil }
func (s *stubDeps) Ctx() context.Context       { return context.Background() }
func (s *stubDeps) LLM() *llmsvc.Service       { return nil }
func (s *stubDeps) Card() *card.Service        { return nil }
func (s *stubDeps) Catalog() *catalog.Service  { return nil }
func (s *stubDeps) Tools() *tools.Dispatcher   { return nil }
func (s *stubDeps) Prompts() *prompts.Builder  { return nil }
func (s *stubDeps) MCPRegistry() *mcp.Registry { return nil }
func (s *stubDeps) LLMActors() *sync.Map       { return &s.actors }

func TestRuntimeConstruction(t *testing.T) {
	// The Runtime must instantiate without touching any concrete
	// service — the headless binary needs to build this with nil
	// services available and then populate them later.
	rt := New(&stubDeps{})
	if rt == nil {
		t.Fatal("New returned nil")
	}
}

// toolCallKey must separate distinct calls to the same tool (ten
// add_field calls in one response are ten fields, not nine duplicates)
// while still collapsing exact repeats regardless of argument order.
func TestToolCallKey(t *testing.T) {
	a := llm.ToolCall{ID: "1", Name: "add_field", Arguments: map[string]any{"key": "traits", "field_type": "list"}}
	b := llm.ToolCall{ID: "2", Name: "add_field", Arguments: map[string]any{"key": "goals", "field_type": "list"}}
	aReordered := llm.ToolCall{ID: "3", Name: "add_field", Arguments: map[string]any{"field_type": "list", "key": "traits"}}

	if toolCallKey(a) == toolCallKey(b) {
		t.Errorf("distinct arguments produced the same key: %q", toolCallKey(a))
	}
	if toolCallKey(a) != toolCallKey(aReordered) {
		t.Errorf("same arguments in a different order produced different keys: %q vs %q", toolCallKey(a), toolCallKey(aReordered))
	}
	if toolCallKey(a) == toolCallKey(llm.ToolCall{ID: "4", Name: "set_fields", Arguments: a.Arguments}) {
		t.Error("different tool names produced the same key")
	}
}
