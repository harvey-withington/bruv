package main

import (
	"bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/notify"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startScheduler creates and starts the agent scheduler.
func (a *App) startScheduler() {
	if a.repo == nil {
		return
	}
	a.scheduler = agent.NewScheduler(
		func() ([]agent.DueAgent, error) {
			return a.queryDueAgentsFromDisk()
		},
		func(ctx context.Context, cardID string) error {
			return a.executeAgent(ctx, cardID)
		},
	)
	a.scheduler.Start(a.ctx)
}

// queryDueAgentsFromDisk scans agent config files to find agents due to run.
// This bypasses the index entirely for reliability.
func (a *App) queryDueAgentsFromDisk() ([]agent.DueAgent, error) {
	if a.repo == nil {
		return nil, nil
	}
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}
	now := time.Now()
	var due []agent.DueAgent
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil || !af.Config.Enabled {
			continue
		}
		if af.Config.Status == model.AgentStatusRunning {
			// Check if genuinely running (has active cancel func)
			if _, active := a.agentCancels.Load(cardID); active {
				continue
			}
			// No active goroutine — check if stuck for >10 min
			stuckThreshold := 10 * time.Minute
			if af.Config.RunStartedAt != nil && time.Since(*af.Config.RunStartedAt) > stuckThreshold {
				log.Printf("agent scheduler: resetting stuck agent %s (running since %s)", cardID, af.Config.RunStartedAt.Format(time.RFC3339))
				af.Config.Status = model.AgentStatusIdle
				af.Config.RunStartedAt = nil
				_ = a.repo.SaveAgentConfig(cardID, af.Config)
			}
			continue
		}
		if af.Config.NextRunAt == nil {
			continue
		}
		// Rate limiting: enforce minimum interval between runs
		minInterval := af.Config.MinIntervalMins
		if minInterval == 0 {
			minInterval = 5 // default 5 minutes
		}
		if af.Config.LastRunAt != nil && time.Since(*af.Config.LastRunAt) < time.Duration(minInterval)*time.Minute {
			continue
		}
		// StartDate: skip if now is before start date
		if af.Config.StartDate != nil && now.Before(*af.Config.StartDate) {
			continue
		}
		// EndDate: auto-disable if now is past end date
		if af.Config.EndDate != nil && now.After(*af.Config.EndDate) {
			af.Config.Enabled = false
			af.Config.Status = model.AgentStatusDisabled
			af.Config.NextRunAt = nil
			_ = a.repo.SaveAgentConfig(cardID, af.Config)
			continue
		}
		// Active window: skip if current time is outside the active window
		if af.Config.ActiveWindowStart != "" && af.Config.ActiveWindowEnd != "" {
			loc := time.Local
			if af.Config.Timezone != "" {
				if l, err := time.LoadLocation(af.Config.Timezone); err == nil {
					loc = l
				}
			}
			localNow := now.In(loc)
			startH, startM := agent.ParseHM(af.Config.ActiveWindowStart)
			endH, endM := agent.ParseHM(af.Config.ActiveWindowEnd)
			dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), startH, startM, 0, 0, loc)
			dayEnd := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), endH, endM, 0, 0, loc)
			if localNow.Before(dayStart) || localNow.After(dayEnd) {
				continue
			}
		}
		if af.Config.NextRunAt.Before(now) || af.Config.NextRunAt.Equal(now) {
			due = append(due, agent.DueAgent{CardID: cardID, NextRunAt: *af.Config.NextRunAt})
		}
	}
	return due, nil
}

// stopScheduler stops the agent scheduler if running.
func (a *App) stopScheduler() {
	if a.scheduler != nil {
		a.scheduler.Stop()
		a.scheduler = nil
	}
}

// startDueDateScanner creates and starts the due-date notification scanner.
func (a *App) startDueDateScanner() {
	if a.repo == nil {
		return
	}
	prefs, _ := config.LoadPreferences()
	configDir, _ := config.ConfigDir()

	a.dueDateScanner = agent.NewDueDateScanner(
		filepath.Join(a.repo.Root, "cards"),
		configDir,
		func(cardID, cardTitle string, threshold time.Duration, overdue bool) {
			notifier := a.makeNotifier()
			var title, body, source string
			if threshold == -2 {
				// Alarm block fired
				title = fmt.Sprintf("Alarm: %s", cardTitle)
				body = "An alarm on this card has fired."
				source = "alarm"
			} else if overdue {
				title = fmt.Sprintf("Overdue: %s", cardTitle)
				body = "This card is past its due date."
				source = "due_date"
			} else if threshold == 0 {
				title = fmt.Sprintf("Due now: %s", cardTitle)
				body = "This card is due now."
				source = "due_date"
			} else {
				title = fmt.Sprintf("Due in %s: %s", formatDuration(threshold), cardTitle)
				body = fmt.Sprintf("This card is due in %s.", formatDuration(threshold))
				source = "due_date"
			}
			channels := notify.ParseChannels(prefs.DueDateChannels)
			notifier.Send(notify.Request{
				Title:     title,
				Body:      body,
				Source:    source,
				CardID:    cardID,
				CardTitle: cardTitle,
				Channels:  channels,
			})
		},
		func(cardID, blockID string) {
			a.markAlarmBlockFired(cardID, blockID)
		},
	)
	a.dueDateScanner.Configure(prefs.DueDateNotify, prefs.DueDateThresholds, prefs.DueDateChannels)
	a.dueDateScanner.Start()
}

// markAlarmBlockFired loads a card, sets alarm_fired=true on the specified block, and saves it back.
func (a *App) markAlarmBlockFired(cardID, blockID string) {
	if a.repo == nil {
		return
	}
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		log.Printf("alarm: failed to load card %s: %v", cardID, err)
		return
	}
	for i := range card.Blocks {
		if card.Blocks[i].ID == blockID {
			if card.Blocks[i].Meta == nil {
				card.Blocks[i].Meta = make(map[string]any)
			}
			card.Blocks[i].Meta["alarm_fired"] = true
			break
		}
	}
	if _, err := a.repo.UpdateCardBlocks(cardID, card.Blocks); err != nil {
		log.Printf("alarm: failed to save card %s: %v", cardID, err)
	}
}

// stopDueDateScanner stops the due-date scanner if running.
func (a *App) stopDueDateScanner() {
	if a.dueDateScanner != nil {
		a.dueDateScanner.Stop()
		a.dueDateScanner = nil
	}
}

func formatDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	if d >= time.Hour {
		return fmt.Sprintf("%d hour(s)", int(d.Hours()))
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}

// makeNotifier creates a notification dispatcher using the current config.
func (a *App) makeNotifier() *notify.Dispatcher {
	cfg, _ := config.LoadNotifyConfig()
	return notify.NewDispatcher(cfg, func(name string, data any) {
		wailsRuntime.EventsEmit(a.ctx, name, data)
		if name == "notification:new" {
			go a.refreshTrayTooltip()
		}
	})
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
	now := time.Now().UTC()
	af.Config.Status = model.AgentStatusRunning
	af.Config.RunStartedAt = &now
	_ = a.repo.SaveAgentConfig(cardID, af.Config)
	if a.idx != nil {
		_ = a.idx.UpdateAgentIndex(cardID, true, string(model.AgentStatusRunning), "")
	}

	// Create cancellable context for this agent run
	agentCtx, agentCancel := context.WithCancel(ctx)
	a.agentCancels.Store(cardID, agentCancel)
	defer func() {
		agentCancel()
		a.agentCancels.Delete(cardID)
	}()

	// Emit started event
	wailsRuntime.EventsEmit(a.ctx, "agent:started", map[string]any{"cardID": cardID})

	// 3. Register in llmActors for activity attribution
	a.llmActors.Store(cardID, "agent")
	defer a.llmActors.Delete(cardID)

	// Use the cancellable context from here on
	ctx = agentCtx

	// Track the run
	run := model.AgentRun{
		ID:        uuid.New().String()[:8],
		CardID:    cardID,
		StartedAt: time.Now().UTC(),
		Status:    "success",
	}

	// Defer: finalize run, update status, calculate next run
	defer func() {
		finishedAt := time.Now().UTC()
		run.FinishedAt = &finishedAt

		// Calculate next run
		finalStatus := model.AgentStatusIdle
		if run.Status == "failure" {
			finalStatus = model.AgentStatusFailed
		}

		// Retry logic for failed runs
		if run.Status == "failure" && af.Config.MaxRetries > 0 {
			af.Config.RetryCount++
			if af.Config.RetryCount <= af.Config.MaxRetries {
				backoff := af.Config.RetryBackoffMins
				if backoff == 0 {
					backoff = 5
				}
				retryAt := finishedAt.Add(time.Duration(backoff*af.Config.RetryCount) * time.Minute)
				af.Config.NextRunAt = &retryAt
				finalStatus = model.AgentStatusIdle // allow re-scheduling
			}
		} else if run.Status != "failure" {
			af.Config.RetryCount = 0 // reset on success
		}

		af.Config.Status = finalStatus
		af.Config.RunStartedAt = nil // clear stuck-detection timestamp
		af.Config.LastRunAt = &finishedAt

		if af.Config.Schedule != "" {
			opts := agent.ScheduleOpts{
				StartDate:         af.Config.StartDate,
				EndDate:           af.Config.EndDate,
				ActiveWindowStart: af.Config.ActiveWindowStart,
				ActiveWindowEnd:   af.Config.ActiveWindowEnd,
				OneShot:           af.Config.OneShot,
				LastRunAt:         af.Config.LastRunAt,
				Timezone:          af.Config.Timezone,
			}
			if next, err := agent.NextRunTimeWithOpts(af.Config.Schedule, now, opts); err == nil {
				af.Config.NextRunAt = &next
			} else {
				// One-shot completed or past end date — disable
				af.Config.NextRunAt = nil
				if af.Config.OneShot {
					af.Config.Enabled = false
				}
			}
		}

		// Track estimated cost
		if run.TokensUsed > 0 {
			runCost := config.EstimateCost(run.ModelUsed, run.TokensUsed)
			af.Config.CostSpentUSD += runCost

			// Budget enforcement
			if af.Config.CostBudgetUSD > 0 && af.Config.CostSpentUSD >= af.Config.CostBudgetUSD {
				af.Config.Enabled = false
				af.Config.Status = model.AgentStatusDisabled
				cardTitle := ""
				if c, err := a.repo.GetCard(cardID); err == nil {
					cardTitle = c.Title
				}
				notifier := a.makeNotifier()
				notifier.Send(notify.Request{
					Title:     fmt.Sprintf("Budget exceeded: %s", cardTitle),
					Body:      fmt.Sprintf("Agent disabled — cost $%.4f exceeded budget $%.2f", af.Config.CostSpentUSD, af.Config.CostBudgetUSD),
					Source:    "budget",
					CardID:    cardID,
					CardTitle: cardTitle,
					Channels:  notify.ParseChannels("in-app,system"),
				})
			}
		}

		_ = a.repo.SaveAgentConfig(cardID, af.Config)
		_ = a.repo.AppendAgentRun(cardID, run)

		// Emit completion event
		eventName := "agent:completed"
		eventData := map[string]any{"cardID": cardID, "status": run.Status, "summary": run.Summary}
		if run.Status == "failure" {
			eventName = "agent:failed"
			eventData["error"] = run.Error
		}
		wailsRuntime.EventsEmit(a.ctx, eventName, eventData)

		// Dispatch notifications based on agent config
		shouldNotify := false
		for _, trigger := range af.Config.NotifyOn {
			if (trigger == "success" && run.Status == "success") ||
				(trigger == "failure" && run.Status == "failure") {
				shouldNotify = true
				break
			}
		}
		if shouldNotify && af.Config.NotifyChannel != "" {
			cardTitle := ""
			if c, err := a.repo.GetCard(cardID); err == nil {
				cardTitle = c.Title
			}
			notifier := a.makeNotifier()
			notifier.Send(notify.Request{
				Title:     fmt.Sprintf("Agent %s: %s", run.Status, cardTitle),
				Body:      run.Summary,
				Source:    "agent",
				CardID:    cardID,
				CardTitle: cardTitle,
				Channels:  notify.ParseChannels(af.Config.NotifyChannel),
			})
		}
	}()

	// 4. Load card
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		run.Status = "failure"
		run.Error = fmt.Sprintf("load card: %v", err)
		return err
	}

	// 5. Load LLM provider (per-agent account/model override)
	cfg, provider, err := a.loadLLMProviderForAccount(af.Config.LLMAccountID, af.Config.LLMModel)
	if err != nil || provider == nil {
		run.Status = "failure"
		run.Error = "LLM not configured"
		return fmt.Errorf("LLM not configured")
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	run.ModelUsed = modelName
	run.ProviderUsed = cfg.Provider

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
	runCtx, runCancel := context.WithTimeout(ctx, timeout)
	defer runCancel()

	var allToolActions []model.ToolAction
	var tokensUsed int

	budget := af.Config.MaxTokensBudget
	if budget == 0 {
		budget = 50000
	}

	resultCf, err := a.runChatLoop(runCtx, provider, modelName, cf, chatLoopConfig{
		chatID:       "__agent__" + cardID,
		systemPrompt: systemPrompt,
		tools:        toolDefs,
		maxIter:      10,
		tokenBudget:     budget,
		totalTokensUsed: &tokensUsed,
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := a.executeAgentToolCall(cardID, card, tc)
			if action != nil {
				allToolActions = append(allToolActions, *action)
			}
			return result, action, nil
		},
		fallbackContent: "Agent run completed.",
	})

	run.TokensUsed = tokensUsed

	// Always record tool actions, even on failure (partial runs)
	run.ToolCalls = allToolActions

	if err != nil {
		if ctx.Err() != nil {
			run.Status = "cancelled"
			run.Error = "cancelled by user"
		} else {
			run.Status = "failure"
			run.Error = err.Error()
		}
		return err
	}

	// 10. Extract summary from last message
	if resultCf != nil && len(resultCf.Messages) > 0 {
		lastMsg := resultCf.Messages[len(resultCf.Messages)-1]

		// If the last message is a system error (e.g. network failure),
		// mark the run as failed — runChatLoop returns nil error for these.
		if lastMsg.Role == model.RoleSystem && strings.HasPrefix(lastMsg.Content, "Error: ") {
			run.Status = "failure"
			run.Error = strings.TrimPrefix(lastMsg.Content, "Error: ")
			return nil
		}

		// Otherwise find the last assistant message as the summary
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
		notifier := a.makeNotifier()
		// Always include in-app; add agent's configured channels
		channels := notify.ParseChannels("in-app")
		notifier.Send(notify.Request{
			Title:     title,
			Body:      body,
			Source:    "agent",
			CardID:    cardID,
			CardTitle: card.Title,
			Channels:  channels,
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
			// Match by key first, then by label (case-insensitive) as fallback
			for i, b := range updatedCard.Blocks {
				if b.Key == key || strings.EqualFold(b.Label, key) {
					updatedCard.Blocks[i].Value = value
					found = true
					break
				}
			}
			if !found {
				// Create a new text block only if no existing block matches
				updatedCard.Blocks = append(updatedCard.Blocks, model.Block{
					ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
					Type:  model.BlockText,
					Label: key,
					Key:   strings.ToLower(strings.ReplaceAll(key, " ", "_")),
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

	// Agent card type guidance
	if card.Type == "agent" {
		sb.WriteString(`
## Agent Card Fields
This card has standard agent fields. Use update_self to maintain them if they exist:
- "status": Set to your current state (e.g. "success", "failed", "idle").
- "last_run": Write a brief summary of what you did this run.
- "last_run_at": Set to the current ISO 8601 timestamp.
- "findings": Append or update with accumulated results across runs.
- "description": Update if the card has one, to reflect what this agent does.
You may also create new fields with update_self if you have useful data to store.
`)
	}

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

	// System context — timezone, date/time, OS, locale
	now := time.Now()
	zone, _ := now.Zone()
	sb.WriteString("\n## System Context\n")
	sb.WriteString(fmt.Sprintf("- Current time: %s (%s)\n", now.Format("2006-01-02 15:04:05 MST"), now.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- Timezone: %s (UTC%s)\n", zone, now.Format("-07:00")))
	sb.WriteString(fmt.Sprintf("- OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))

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

// CancelAgent cancels a running agent, or resets a stuck status.
func (a *App) CancelAgent(cardID string) error {
	if cancelFn, ok := a.agentCancels.Load(cardID); ok {
		cancelFn.(context.CancelFunc)()
		return nil
	}
	// No active goroutine — reset stuck status from a previous crash
	if a.repo != nil {
		af, err := a.repo.GetAgentConfig(cardID)
		if err == nil && af.Config.Status == model.AgentStatusRunning {
			af.Config.Status = model.AgentStatusIdle
			_ = a.repo.SaveAgentConfig(cardID, af.Config)
			if a.idx != nil {
				nextRun := ""
				if af.Config.NextRunAt != nil {
					nextRun = af.Config.NextRunAt.Format(time.RFC3339)
				}
				_ = a.idx.UpdateAgentIndex(cardID, af.Config.Enabled, string(model.AgentStatusIdle), nextRun)
			}
			wailsRuntime.EventsEmit(a.ctx, "agent:completed", map[string]any{"cardID": cardID, "status": "cancelled"})
			return nil
		}
	}
	return nil
}

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
	if a.trayPauseItem != nil && !a.trayPauseItem.Checked() {
		a.trayPauseItem.Check()
	}
	wailsRuntime.EventsEmit(a.ctx, "scheduler:paused", map[string]any{"paused": true})
	return nil
}

// ResumeAllAgents resumes the agent scheduler.
func (a *App) ResumeAllAgents() error {
	if a.scheduler != nil {
		a.scheduler.Resume()
	}
	if a.trayPauseItem != nil && a.trayPauseItem.Checked() {
		a.trayPauseItem.Uncheck()
	}
	wailsRuntime.EventsEmit(a.ctx, "scheduler:paused", map[string]any{"paused": false})
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

// GetAllAgents returns a summary of every agent across all cards.
func (a *App) GetAllAgents() ([]map[string]any, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}
	var agents []map[string]any
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil {
			continue
		}

		// Skip cards with no meaningful agent configuration
		cfg := af.Config
		if !cfg.Enabled && cfg.Goal == "" && cfg.Schedule == "" && len(af.Runs) == 0 {
			continue
		}

		cardTitle := cardID
		if c, err := a.repo.GetCard(cardID); err == nil {
			cardTitle = c.Title
		}

		_, isRunning := a.agentCancels.Load(cardID)

		entry := map[string]any{
			"card_id":    cardID,
			"card_title": cardTitle,
			"enabled":    af.Config.Enabled,
			"status":     string(af.Config.Status),
			"schedule":   af.Config.Schedule,
			"goal":       af.Config.Goal,
			"is_running": isRunning,
			"one_shot":   af.Config.OneShot,
		}
		if af.Config.StartDate != nil {
			entry["start_date"] = af.Config.StartDate.Format(time.RFC3339)
		}
		if af.Config.EndDate != nil {
			entry["end_date"] = af.Config.EndDate.Format(time.RFC3339)
		}
		if af.Config.LastRunAt != nil {
			entry["last_run_at"] = af.Config.LastRunAt.Format(time.RFC3339)
		}
		if af.Config.NextRunAt != nil {
			entry["next_run_at"] = af.Config.NextRunAt.Format(time.RFC3339)
		}

		// Include last run info if available (runs are newest-first)
		if len(af.Runs) > 0 {
			lastRun := af.Runs[0]
			entry["last_run_status"] = lastRun.Status
			entry["last_run_summary"] = lastRun.Summary
			entry["last_run_tokens"] = lastRun.TokensUsed
			if lastRun.Error != "" {
				entry["last_run_error"] = lastRun.Error
			}
		}

		agents = append(agents, entry)
	}
	return agents, nil
}

// GetAllAgentRuns returns a unified run history across all agents, most recent first.
func (a *App) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 100
	}
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}

	// Collect all runs with card context
	type runWithContext struct {
		run       model.AgentRun
		cardTitle string
	}
	var allRuns []runWithContext

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil || len(af.Runs) == 0 {
			continue
		}
		cardTitle := cardID
		if c, err := a.repo.GetCard(cardID); err == nil {
			cardTitle = c.Title
		}
		for _, r := range af.Runs {
			r.CardID = cardID
			allRuns = append(allRuns, runWithContext{run: r, cardTitle: cardTitle})
		}
	}

	// Sort by started_at descending (most recent first)
	for i := 0; i < len(allRuns)-1; i++ {
		for j := i + 1; j < len(allRuns); j++ {
			if allRuns[j].run.StartedAt.After(allRuns[i].run.StartedAt) {
				allRuns[i], allRuns[j] = allRuns[j], allRuns[i]
			}
		}
	}

	// Limit
	if len(allRuns) > limit {
		allRuns = allRuns[:limit]
	}

	var result []map[string]any
	for _, rc := range allRuns {
		r := rc.run
		entry := map[string]any{
			"id":          r.ID,
			"card_id":     r.CardID,
			"card_title":  rc.cardTitle,
			"started_at":  r.StartedAt.Format(time.RFC3339),
			"status":      r.Status,
			"tokens_used":    r.TokensUsed,
			"tool_count":     len(r.ToolCalls),
			"model_used":     r.ModelUsed,
			"estimated_cost": config.EstimateCost(r.ModelUsed, r.TokensUsed),
		}
		if r.FinishedAt != nil {
			entry["finished_at"] = r.FinishedAt.Format(time.RFC3339)
			entry["duration_secs"] = int(r.FinishedAt.Sub(r.StartedAt).Seconds())
		}
		if r.Summary != "" {
			entry["summary"] = r.Summary
		}
		if r.Error != "" {
			entry["error"] = r.Error
		}
		result = append(result, entry)
	}
	return result, nil
}

// GetAgentAnalytics returns aggregate statistics across all agents.
func (a *App) GetAgentAnalytics() (map[string]any, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cardsDir := filepath.Join(a.repo.Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}

	var totalAgents, enabledAgents, totalRuns, successRuns, failedRuns, totalTokens int
	var totalCost, costToday, cost7d float64
	costByModel := make(map[string]float64)

	now := time.Now()
	now7d := now.AddDate(0, 0, -7)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := a.repo.GetAgentConfig(cardID)
		if err != nil {
			continue
		}
		cfg := af.Config
		if !cfg.Enabled && cfg.Goal == "" && cfg.Schedule == "" && len(af.Runs) == 0 {
			continue
		}
		totalAgents++
		if cfg.Enabled {
			enabledAgents++
		}
		for _, r := range af.Runs {
			totalRuns++
			totalTokens += r.TokensUsed
			if r.Status == "success" {
				successRuns++
			} else if r.Status == "failure" {
				failedRuns++
			}
			runCost := config.EstimateCost(r.ModelUsed, r.TokensUsed)
			totalCost += runCost
			if r.ModelUsed != "" {
				costByModel[r.ModelUsed] += runCost
			}
			if r.StartedAt.After(todayStart) {
				costToday += runCost
			}
			if r.StartedAt.After(now7d) {
				cost7d += runCost
			}
		}
	}

	return map[string]any{
		"total_agents":   totalAgents,
		"enabled_agents": enabledAgents,
		"total_runs":     totalRuns,
		"success_runs":   successRuns,
		"failed_runs":    failedRuns,
		"total_tokens":   totalTokens,
		"total_cost":     totalCost,
		"cost_today":     costToday,
		"cost_7d":        cost7d,
		"cost_by_model":  costByModel,
	}, nil
}

// ForceQuit actually terminates the app (bypasses hide-to-tray).
func (a *App) ForceQuit() {
	a.forceQuit = true
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	wailsRuntime.Quit(a.ctx)
}
