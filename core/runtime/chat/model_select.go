package chat

import (
	"fmt"
	"log/slog"

	llmsvc "bruv/core/services/llm"
	"bruv/core/services/llm/routing"
	"bruv/internal/config"
	"bruv/internal/model"
)

// selectModel resolves the model for a chat turn once its prompt and
// tools are known, so routers see the real request. useChatChoice lets
// the chat's own model choice override the task assignment.
//
// Same contract as Select: an error means AI is configured but broken
// (surface it so the chat doesn't silently never answer); a nil
// selection means not configured (silent no-op — the IsLLMConfigured
// first-run nudge owns that state).
func (rt *Runtime) selectModel(task llmsvc.Task, chatID string, useChatChoice bool, cf *model.ChatFile, systemPrompt string, toolsOffered bool) (*llmsvc.Selection, error) {
	var choice config.ModelRef
	if useChatChoice {
		c, err := config.GetChatModelChoice(rt.deps.Repo().Manifest.ID, chatID)
		if err != nil {
			slog.Warn("chat model choice unreadable, using task assignment", "chatID", chatID, "err", err)
		}
		choice = c
	}

	userMessage := ""
	contextTokens := routing.EstimateTokens(systemPrompt)
	for _, m := range cf.Messages {
		contextTokens += routing.EstimateTokens(m.Content)
		if m.Role == model.RoleUser {
			userMessage = m.Content
		}
	}

	sel, err := rt.deps.LLM().Select(rt.deps.Ctx(), llmsvc.RouteRequest{
		Task:          task,
		Choice:        choice,
		UserMessage:   userMessage,
		ContextTokens: contextTokens,
		ToolsOffered:  toolsOffered,
	})
	if err != nil {
		slog.Error("chat: llm model selection failed", "task", task, "err", err)
		return nil, fmt.Errorf("llm provider unavailable: %w", err)
	}
	return sel, nil
}
