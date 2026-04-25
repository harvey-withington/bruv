package chat

import (
	"context"
	"sync"

	"bruv/core/runtime/prompts"
	"bruv/core/runtime/tools"
	"bruv/core/services/card"
	llmsvc "bruv/core/services/llm"
	"bruv/internal/mcp"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// Deps is the narrow host contract for Runtime. Covers repo + schema,
// the LLM provider resolver, the card service (for category lookups
// during project chat), the tool dispatcher, the prompt builder, and
// the two pieces of mutable runtime state the chat loop threads
// through (MCP registry for dynamic tool catalogue, llmActors for
// activity-log attribution).
type Deps interface {
	Repo() *repo.Repository
	Registry() *schema.Registry
	Ctx() context.Context

	LLM() *llmsvc.Service
	Card() *card.Service
	Tools() *tools.Dispatcher
	Prompts() *prompts.Builder

	// MCPRegistry may be nil when no repo is open or MCP startup failed.
	// Callers must nil-check.
	MCPRegistry() *mcp.Registry

	// LLMActors is the per-card actor tracker the activity log reads
	// to attribute edits to "llm:<model>" instead of the current user
	// profile. Passed through so the runtime can Store/Delete during
	// a chat turn.
	LLMActors() *sync.Map
}

// Runtime is the chat entry point. Construct once per App / headless
// binary and reuse for every SendCard / SendProject call.
type Runtime struct {
	deps Deps
}

// New constructs a chat Runtime.
func New(deps Deps) *Runtime {
	return &Runtime{deps: deps}
}
