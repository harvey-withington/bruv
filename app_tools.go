package main

// Forwarders for the LLM tool execution surface. Domain logic lives
// in core/runtime/tools after the LLM-runtime-extraction stage-1
// pass — see plan/llm-runtime-extraction-2026-04-24.md.
//
// App callers (app_chat.go, app_agent.go, app_pending.go) still reach
// the tool dispatcher through a.tools, and the entry-point names kept
// their lowercase forms locally for source-diff minimalism. The
// canonical dispatcher lives on *tools.Dispatcher.

import (
	"bruv/core/runtime/tools"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// projectChatScope is the main-package alias for tools.ProjectChatScope
// so existing construction sites and parameter names don't churn.
type projectChatScope = tools.ProjectChatScope

// executeToolCall dispatches an LLM tool call into the card-scope
// executor and returns (result, action, pin suggestion).
func (a *App) executeToolCall(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	return a.tools.ExecuteCard(cardID, card, tc, allCats)
}

// executeProjectToolCall dispatches a project-chat tool call.
func (a *App) executeProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, *model.ToolAction) {
	return a.tools.ExecuteProject(tc, scope)
}

// stageToolCall stages a card-chat tool call as PendingEdits (suggest mode).
func (a *App) stageToolCall(tc llm.ToolCall, allCats []CategoryPath) (string, []model.PendingEdit) {
	return a.tools.StageCard(tc, allCats)
}

// stageProjectToolCall stages a project-chat tool call as PendingEdits.
func (a *App) stageProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, []model.PendingEdit) {
	return a.tools.StageProject(tc, scope)
}

// coerceBlockValueForBlock is the package-level coercion helper.
// Called from app_agent.go during agent tool execution; mirrored here
// as a forwarder so that file doesn't need to import tools directly.
func coerceBlockValueForBlock(b *model.Block, val any) (any, error) {
	return tools.CoerceBlockValueForBlock(b, val)
}
