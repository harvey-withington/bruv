package tools

// Bridge from the chat dispatcher to BRUV's native tool registry
// (core/boardtools) — the same tools, names and handlers the MCP server
// exposes. The registry imports this package, so the dispatcher reaches
// it through the NativeTools interface the host provides (Deps.Native).
//
// Chat calls are scoped to one project (ruling 2026-09-29: "keep the
// project limit"), and in Suggest mode every native write is staged for
// the user's approval while reads run straight away.

import (
	"github.com/google/uuid"

	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"
)

// NativeTools is the registry as the dispatcher sees it.
type NativeTools interface {
	// Defs returns the tool definitions; scoped drops board-only tools.
	Defs(scoped bool) []llm.ToolDef
	Has(name string) bool
	IsWrite(name string) bool
	// Check validates a call against a scope without running it.
	Check(scope *ProjectChatScope, name string, args map[string]any) error
	// Call runs the tool; scope nil = the whole board.
	Call(scope *ProjectChatScope, name string, args map[string]any) (result string, isErr bool)
	// Summary is a one-line outcome for action lists and pending edits.
	Summary(name string, args map[string]any, result string) string
}

// ScopeForProject builds a project scope: its slugs plus every card
// pinned in it. The one builder for project chat, card chat, and the
// pending-edit apply path.
func ScopeForProject(r *repo.Repository, brandSlug, streamSlug, projectSlug string) ProjectChatScope {
	scope := ProjectChatScope{BrandSlug: brandSlug, StreamSlug: streamSlug, ProjectSlug: projectSlug, CardIDs: map[string]bool{}}
	if r == nil {
		return scope
	}
	cats, _ := r.ListCategories(brandSlug, streamSlug, projectSlug)
	for _, cat := range cats {
		pins, _ := r.ListCardsInCategory(cat.ID)
		for _, p := range pins {
			scope.CardIDs[p.CardID] = true
		}
	}
	return scope
}

// cardScope is card chat's scope: the project the card is filed in
// (its first pin), which always includes the card itself. An unfiled
// card reaches only itself.
func (d *Dispatcher) cardScope(cardID string, allCats []CategoryPath) *ProjectChatScope {
	scope := ProjectChatScope{CardIDs: map[string]bool{}}
	if pins, _ := d.deps.Repo().GetCardPins(cardID); len(pins) > 0 {
		for _, c := range allCats {
			if c.CategoryID == pins[0].CategoryID {
				scope = ScopeForProject(d.deps.Repo(), c.BrandSlug, c.StreamSlug, c.ProjectSlug)
				break
			}
		}
	}
	scope.CardIDs[cardID] = true
	return &scope
}

// NativeDefs returns the native tool definitions for a chat session
// (scoped: board-only tools are left out). Nil when no registry is wired.
func (d *Dispatcher) NativeDefs(scoped bool) []llm.ToolDef {
	if n := d.deps.Native(); n != nil {
		return n.Defs(scoped)
	}
	return nil
}

// isNative reports whether a call goes to the native registry.
func (d *Dispatcher) isNative(name string) bool {
	n := d.deps.Native()
	return n != nil && n.Has(name)
}

// executeNative runs a native tool inside scope and records the action.
func (d *Dispatcher) executeNative(scope *ProjectChatScope, tc llm.ToolCall) (string, *model.ToolAction) {
	n := d.deps.Native()
	result, isErr := n.Call(scope, tc.Name, tc.Arguments)
	summary := result
	if !isErr {
		summary = n.Summary(tc.Name, tc.Arguments, result)
	}
	return result, &model.ToolAction{Tool: tc.Name, Input: tc.Arguments, Result: summary}
}

// stageNative stages a native write as a pending edit (after checking
// its scope, so a bad call is bounced back to the model now rather than
// failing on the user at apply time); a read runs straight away.
func (d *Dispatcher) stageNative(scope *ProjectChatScope, tc llm.ToolCall) (string, []model.PendingEdit) {
	n := d.deps.Native()
	if !n.IsWrite(tc.Name) {
		result, _ := n.Call(scope, tc.Name, tc.Arguments)
		return result, nil
	}
	if err := n.Check(scope, tc.Name, tc.Arguments); err != nil {
		return "error: " + err.Error(), nil
	}
	label := n.Summary(tc.Name, tc.Arguments, "")
	return "Staged for the user's approval: " + label, []model.PendingEdit{{
		ID: uuid.New().String(), Tool: tc.Name, Input: tc.Arguments, Label: label, Status: "pending",
	}}
}
