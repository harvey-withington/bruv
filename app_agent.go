package main

import (
	"bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startScheduler creates and starts the agent scheduler.
func (a *App) startScheduler() {
	if a.idx == nil {
		return
	}
	a.scheduler = agent.NewScheduler(
		func() ([]agent.DueAgent, error) {
			results, err := a.idx.QueryDueAgents(time.Now())
			if err != nil {
				return nil, err
			}
			// Convert index.DueAgent to agent.DueAgent
			agents := make([]agent.DueAgent, len(results))
			for i, r := range results {
				agents[i] = agent.DueAgent{CardID: r.CardID, NextRunAt: r.NextRunAt}
			}
			return agents, nil
		},
		func(ctx context.Context, cardID string) error {
			return a.executeAgent(ctx, cardID)
		},
	)
	a.scheduler.Start(a.ctx)
}

// stopScheduler stops the agent scheduler if running.
func (a *App) stopScheduler() {
	if a.scheduler != nil {
		a.scheduler.Stop()
		a.scheduler = nil
	}
}

// executeAgent runs a single agent card end-to-end.
// Called by the scheduler or TriggerAgent.
func (a *App) executeAgent(ctx context.Context, cardID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}

	// 1. Load agent config
	af, err := a.repo.GetAgentConfig(cardID)
	if err != nil {
		return fmt.Errorf("load agent config: %w", err)
	}
	if !af.Config.Enabled {
		return nil
	}

	// 2. Set status to running
	af.Config.Status = model.AgentStatusRunning
	_ = a.repo.SaveAgentConfig(cardID, af.Config)
	if a.idx != nil {
		_ = a.idx.UpdateAgentIndex(cardID, true, string(model.AgentStatusRunning), "")
	}

	// Emit started event
	wailsRuntime.EventsEmit(a.ctx, "agent:started", map[string]any{"cardID": cardID})

	// 3. Register in llmActors for activity attribution
	a.llmActors.Store(cardID, "agent")
	defer a.llmActors.Delete(cardID)

	// Track the run
	run := model.AgentRun{
		ID:        uuid.New().String()[:8],
		CardID:    cardID,
		StartedAt: time.Now().UTC(),
		Status:    "success",
	}

	// Defer: finalize run, update status, calculate next run
	defer func() {
		now := time.Now().UTC()
		run.FinishedAt = &now

		// Calculate next run
		finalStatus := model.AgentStatusIdle
		if run.Status == "failure" {
			finalStatus = model.AgentStatusFailed
		}
		af.Config.Status = finalStatus
		af.Config.LastRunAt = &now

		if af.Config.Schedule != "" {
			if next, err := agent.NextRunTime(af.Config.Schedule, now); err == nil {
				af.Config.NextRunAt = &next
			}
		}

		_ = a.repo.SaveAgentConfig(cardID, af.Config)
		_ = a.repo.AppendAgentRun(cardID, run)

		if a.idx != nil {
			nextRun := ""
			if af.Config.NextRunAt != nil {
				nextRun = af.Config.NextRunAt.Format(time.RFC3339)
			}
			_ = a.idx.UpdateAgentIndex(cardID, af.Config.Enabled, string(finalStatus), nextRun)
		}

		// Emit completion event
		eventName := "agent:completed"
		eventData := map[string]any{"cardID": cardID, "status": run.Status, "summary": run.Summary}
		if run.Status == "failure" {
			eventName = "agent:failed"
			eventData["error"] = run.Error
		}
		wailsRuntime.EventsEmit(a.ctx, eventName, eventData)
	}()

	// 4. Load card
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		run.Status = "failure"
		run.Error = fmt.Sprintf("load card: %v", err)
		return err
	}

	// 5. Load LLM provider
	cfg, provider, err := a.loadLLMProvider()
	if err != nil || provider == nil {
		run.Status = "failure"
		run.Error = "LLM not configured"
		return fmt.Errorf("LLM not configured")
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}

	// 6. Build system prompt
	systemPrompt := a.buildAgentSystemPrompt(card, af.Config, cfg)

	// 7. Build filtered tools
	toolDefs := llm.AgentTools(af.Config.AllowedTools)

	// 8. Create ephemeral chat file (not persisted to card chat)
	cf := &model.ChatFile{
		CardID: "__agent__" + cardID,
		Messages: []model.ChatMessage{
			{
				ID:        uuid.New().String(),
				Role:      model.RoleUser,
				Content:   af.Config.Goal,
				Timestamp: time.Now().UTC(),
			},
		},
	}

	// 9. Run tool loop with timeout
	timeout := 5 * time.Minute
	agentCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var allToolActions []model.ToolAction

	resultCf, err := a.runChatLoop(agentCtx, provider, modelName, cf, chatLoopConfig{
		chatID:       "__agent__" + cardID,
		systemPrompt: systemPrompt,
		tools:        toolDefs,
		maxIter:      10,
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := a.executeAgentToolCall(cardID, card, tc)
			if action != nil {
				allToolActions = append(allToolActions, *action)
			}
			return result, action, nil
		},
		fallbackContent: "Agent run completed.",
	})

	if err != nil {
		run.Status = "failure"
		run.Error = err.Error()
		return err
	}

	// 10. Extract summary from last assistant message
	run.ToolCalls = allToolActions
	if resultCf != nil && len(resultCf.Messages) > 0 {
		for i := len(resultCf.Messages) - 1; i >= 0; i-- {
			if resultCf.Messages[i].Role == model.RoleAssistant && resultCf.Messages[i].Content != "" {
				run.Summary = resultCf.Messages[i].Content
				break
			}
		}
	}

	return nil
}

// executeAgentToolCall dispatches a single agent tool call.
func (a *App) executeAgentToolCall(cardID string, card *model.Card, tc llm.ToolCall) (string, *model.ToolAction) {
	action := &model.ToolAction{
		Tool:  tc.Name,
		Input: tc.Arguments,
	}

	switch tc.Name {
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agent.WebFetch(url)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = "fetched " + url
		return result, action

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agent.WebSearch(query)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = fmt.Sprintf("searched: %s", query)
		return result, action

	case "http_request":
		method, _ := tc.Arguments["method"].(string)
		url, _ := tc.Arguments["url"].(string)
		body, _ := tc.Arguments["body"].(string)
		result, err := agent.HTTPRequest(method, url, body)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = fmt.Sprintf("%s %s", method, url)
		return result, action

	case "notify":
		title, _ := tc.Arguments["title"].(string)
		body, _ := tc.Arguments["body"].(string)
		wailsRuntime.EventsEmit(a.ctx, "agent:notification", map[string]any{
			"cardID": cardID,
			"title":  title,
			"body":   body,
		})
		action.Result = "notification sent"
		return "Notification sent to user.", action

	case "update_self":
		updates, ok := tc.Arguments["updates"].([]any)
		if !ok {
			action.Result = "error: invalid updates format"
			return action.Result, action
		}
		updatedCard, err := a.repo.GetCard(cardID)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		for _, u := range updates {
			upd, ok := u.(map[string]any)
			if !ok {
				continue
			}
			key, _ := upd["key"].(string)
			value, _ := upd["value"].(string)
			if key == "" {
				continue
			}
			found := false
			for i, b := range updatedCard.Blocks {
				if b.Key == key {
					updatedCard.Blocks[i].Value = value
					found = true
					break
				}
			}
			if !found {
				// Create a new text block
				updatedCard.Blocks = append(updatedCard.Blocks, model.Block{
					ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
					Type:  model.BlockText,
					Label: key,
					Key:   key,
					Value: value,
				})
			}
		}
		updatedCard.UpdatedAt = time.Now().UTC()
		if err := a.repo.UpdateCardDirect(cardID, updatedCard); err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		if a.idx != nil {
			_, _ = a.idx.IncrementalRefresh(a.repo.Root)
		}
		action.Result = "card updated"
		return "Card blocks updated successfully.", action

	case "read_card":
		targetID, _ := tc.Arguments["card_id"].(string)
		targetCard, err := a.repo.GetCard(targetID)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = "read card: " + targetCard.Title
		return formatCardContent(targetCard), action

	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		cardType, _ := tc.Arguments["card_type"].(string)
		if title == "" {
			action.Result = "error: title is required"
			return action.Result, action
		}
		newCard, err := a.repo.CreateCard(cardType, title)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		if a.idx != nil {
			_, _ = a.idx.IncrementalRefresh(a.repo.Root)
		}
		action.Result = fmt.Sprintf("created card: %s (%s)", newCard.Title, newCard.ID)
		return fmt.Sprintf("Created card '%s' with ID %s.", newCard.Title, newCard.ID), action

	default:
		action.Result = "unknown tool"
		return fmt.Sprintf("Unknown tool: %s", tc.Name), action
	}
}

// buildAgentSystemPrompt constructs the system prompt for an agent run.
func (a *App) buildAgentSystemPrompt(card *model.Card, agentCfg model.AgentConfig, llmCfg config.LLMConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are BRUV Agent, an autonomous AI assistant running on a schedule. Today is %s.

You are executing a task defined by the user. Complete the goal using the tools available to you.

RULES:
- Focus on completing the goal efficiently.
- Use the notify tool to alert the user about important findings.
- Use update_self to record your findings on the card for future reference.
- If a web search or fetch fails, try alternative approaches.
- Be concise in your responses.

## Your Goal
%s
`, time.Now().Format("2006-01-02 (Monday)"), agentCfg.Goal))

	// Card context
	sb.WriteString("\n## Current Card State\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", card.Title))
	if card.Type != "" {
		sb.WriteString(fmt.Sprintf("Type: %s\n", card.Type))
	}
	if len(card.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(card.Tags, ", ")))
	}
	if len(card.Blocks) > 0 {
		sb.WriteString("\n### Card Content\n")
		for _, b := range card.Blocks {
			label := b.Label
			if label == "" {
				label = b.Key
			}
			if label == "" {
				label = b.Type
			}
			sb.WriteString(fmt.Sprintf("- %s: %v\n", label, b.Value))
		}
	}

	// Repository context
	if a.repo != nil && a.repo.Manifest.Description != "" {
		sb.WriteString(fmt.Sprintf("\n## Repository: %s\n%s\n", a.repo.Manifest.Name, a.repo.Manifest.Description))
	}

	// User-provided LLM context
	if llmCfg.Context != "" {
		sb.WriteString("\n## User Context\n" + llmCfg.Context + "\n")
	}

	return sb.String()
}

// formatCardContent formats a card's content as readable text.
func formatCardContent(card *model.Card) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Title: %s\n", card.Title))
	if card.Type != "" {
		sb.WriteString(fmt.Sprintf("Type: %s\n", card.Type))
	}
	if len(card.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(card.Tags, ", ")))
	}
	if card.DueDate != nil {
		sb.WriteString(fmt.Sprintf("Due: %s\n", card.DueDate.Format("2006-01-02")))
	}
	for _, b := range card.Blocks {
		label := b.Label
		if label == "" {
			label = b.Key
		}
		if label == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n%s: %v\n", label, b.Value))
	}
	return sb.String()
}

// --- Wails-bound agent methods ---

// TriggerAgent runs an agent immediately, bypassing the schedule.
func (a *App) TriggerAgent(cardID string) error {
	if a.scheduler == nil {
		// No scheduler — run directly
		go func() {
			_ = a.executeAgent(a.ctx, cardID)
		}()
		return nil
	}
	return a.scheduler.TriggerNow(a.ctx, cardID)
}

// PauseAllAgents pauses the agent scheduler.
func (a *App) PauseAllAgents() error {
	if a.scheduler != nil {
		a.scheduler.Pause()
	}
	return nil
}

// ResumeAllAgents resumes the agent scheduler.
func (a *App) ResumeAllAgents() error {
	if a.scheduler != nil {
		a.scheduler.Resume()
	}
	return nil
}

// GetAgentSchedulerStatus returns the current scheduler state.
func (a *App) GetAgentSchedulerStatus() map[string]any {
	if a.scheduler == nil {
		return map[string]any{"active": false, "paused": false, "runningCount": 0}
	}
	return map[string]any{
		"active":       true,
		"paused":       a.scheduler.IsPaused(),
		"runningCount": a.scheduler.RunningCount(),
	}
}

// ForceQuit actually terminates the app (bypasses hide-to-tray).
func (a *App) ForceQuit() {
	a.forceQuit = true
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	wailsRuntime.Quit(a.ctx)
}
