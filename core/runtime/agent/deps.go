package agent

import (
	"context"
	"sync"

	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/prompts"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	llmsvc "bruv/core/services/llm"
	projectsvc "bruv/core/services/project"
	agentlib "bruv/internal/agent"
	"bruv/internal/index"
	"bruv/internal/mcp"
	"bruv/internal/notify"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// Deps is the narrow host contract for the agent Runtime. Covers repo
// + index + schema, context, event-bus publish, the four services the
// agent's built-in tool dispatch writes through (card / project /
// catalog / llm), the two sub-runtimes (tools + prompts + chat), and
// the two pieces of mutable runtime state that have to be shared with
// the App shell (MCP registry for dynamic tool catalogues, llmActors
// for activity-log attribution).
type Deps interface {
	Repo() *repo.Repository
	Index() *index.Index
	Registry() *schema.Registry
	Ctx() context.Context

	// Publish announces a domain event. Used for agent:started,
	// agent:completed, agent:failed, card:updated, scheduler:paused,
	// notification:new.
	Publish(topic string, payload any)

	// Service handles — agent tool implementations write through these.
	LLM() *llmsvc.Service
	Card() *card.Service
	Project() *projectsvc.Service
	Catalog() *catalog.Service

	// Sub-runtimes.
	Prompts() *prompts.Builder
	ChatRT() *chatrt.Runtime

	// Runtime state.
	MCPRegistry() *mcp.Registry
	LLMActors() *sync.Map
}

// Runtime owns the agent execution surface. Construct one per host
// (App or cmd/bruv-server) and call StartScheduler / StopScheduler +
// StartDueDateScanner / StopDueDateScanner during repo lifecycle.
type Runtime struct {
	deps Deps

	// agentCancels tracks per-card cancel funcs for running agents so
	// CancelAgent can interrupt mid-run. Lives here rather than on
	// the App shell because cancellation is a runtime concern, not a
	// UI one.
	agentCancels sync.Map

	// Scheduler + due-date scanner handles. Nil until the
	// corresponding Start* method runs. Exported via accessor
	// methods (Scheduler, DueDateScanner) so App can read state for
	// Pause/Resume/GetSchedulerStatus without re-implementing.
	scheduler      *agentlib.Scheduler
	dueDateScanner *agentlib.DueDateScanner
}

// Scheduler exposes the running scheduler so the App shell can
// implement Pause / Resume / TriggerNow without re-implementing the
// control surface. Returns nil when the scheduler isn't running.
func (rt *Runtime) Scheduler() *agentlib.Scheduler { return rt.scheduler }

// DueDateScanner exposes the scanner for App-shell integration.
// Returns nil when the scanner isn't running.
func (rt *Runtime) DueDateScanner() *agentlib.DueDateScanner { return rt.dueDateScanner }

// AgentCancels exposes the per-card cancel-func map so the App
// shell's Wails-bound CancelAgent method can reach running agents.
func (rt *Runtime) AgentCancels() *sync.Map { return &rt.agentCancels }

// New constructs an agent Runtime.
func New(deps Deps) *Runtime {
	return &Runtime{deps: deps}
}

// StartScheduler starts the agent scheduler. Exported so host shells
// (App / cmd/bruv-server) can call it from their repo-lifecycle paths.
func (rt *Runtime) StartScheduler() { rt.startScheduler() }

// StopScheduler stops the agent scheduler. Safe to call when idle.
func (rt *Runtime) StopScheduler() { rt.stopScheduler() }

// StartDueDateScanner starts the due-date scanner.
func (rt *Runtime) StartDueDateScanner() { rt.startDueDateScanner() }

// StopDueDateScanner stops the due-date scanner. Safe to call when idle.
func (rt *Runtime) StopDueDateScanner() { rt.stopDueDateScanner() }

// ExecuteAgent runs a single agent card end-to-end. Used for direct
// triggering when no scheduler is active (the scheduler path calls
// the internal executeAgent directly via the callback wired in
// startScheduler).
func (rt *Runtime) ExecuteAgent(ctx context.Context, cardID string) error {
	return rt.executeAgent(ctx, cardID)
}

// EmitCardUpdated publishes a card:updated event so open card detail
// views re-fetch. Exported for App-side code paths (alarms, external
// file changes) that also need to signal card mutations.
func (rt *Runtime) EmitCardUpdated(cardID string) { rt.emitCardUpdated(cardID) }

// MakeNotifier builds a notification dispatcher bound to the runtime's
// event bus. Exposed so App-side notifications share the same event
// surface (and, in future, the same rate-limiting).
func (rt *Runtime) MakeNotifier() *notify.Dispatcher { return rt.makeNotifier() }
