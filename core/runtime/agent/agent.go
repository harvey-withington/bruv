package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/promptfmt"
	"bruv/core/runtime/tools"
	llmsvc "bruv/core/services/llm"
	agentlib "bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/notify"
)

// logIdxErr mirrors the App-shell helper: warn + emit an index:stale
// event so the UI can prompt for a rebuild. Non-fatal — a failed index
// update just means the in-memory search/agent-presence indexes are
// temporarily stale.
func (rt *Runtime) logIdxErr(op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("index update failed", "op", op, "err", err)
	rt.deps.Publish("index:stale", op)
}

// idxIncrementalRefresh wraps Index.IncrementalRefresh. Call sites
// already guard against a nil index, so this assumes the index is
// present.
func (rt *Runtime) idxIncrementalRefresh() {
	if _, err := rt.deps.Index().IncrementalRefresh(rt.deps.Repo().Root); err != nil {
		rt.logIdxErr("IncrementalRefresh", err)
	}
}

// mcpOutputLimit caps MCP tool output at 8KB before truncating.
const mcpOutputLimit = 8 * 1024

func (rt *Runtime) startScheduler() {
	if rt.deps.Repo() == nil {
		return
	}
	rt.scheduler = agentlib.NewScheduler(
		func() ([]agentlib.DueAgent, error) {
			return rt.queryDueAgentsFromDisk()
		},
		func(ctx context.Context, cardID string) error {
			return rt.executeAgent(ctx, cardID)
		},
	)
	rt.scheduler.Start(rt.deps.Ctx())
}

func (rt *Runtime) queryDueAgentsFromDisk() ([]agentlib.DueAgent, error) {
	if rt.deps.Repo() == nil {
		return nil, nil
	}
	cardsDir := filepath.Join(rt.deps.Repo().Root, "cards")
	entries, err := os.ReadDir(cardsDir)
	if err != nil {
		return nil, nil
	}
	now := time.Now()
	var due []agentlib.DueAgent
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".agent.json") {
			continue
		}
		cardID := strings.TrimSuffix(name, ".agent.json")
		af, err := rt.deps.Repo().GetAgentConfig(cardID)
		if err != nil || !af.Config.Enabled {
			continue
		}
		if af.Config.Status == model.AgentStatusRunning {
			// Check if genuinely running (has active cancel func)
			if _, active := rt.agentCancels.Load(cardID); active {
				continue
			}
			// No active goroutine — check if stuck for >10 min
			stuckThreshold := 10 * time.Minute
			if af.Config.RunStartedAt != nil && time.Since(*af.Config.RunStartedAt) > stuckThreshold {
				slog.Warn("agent scheduler resetting stuck agent",
					"card_id", cardID,
					"running_since", af.Config.RunStartedAt.Format(time.RFC3339))
				af.Config.Status = model.AgentStatusIdle
				af.Config.RunStartedAt = nil
				_ = rt.deps.Repo().SaveAgentConfig(cardID, af.Config)
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
			_ = rt.deps.Repo().SaveAgentConfig(cardID, af.Config)
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
			startH, startM := agentlib.ParseHM(af.Config.ActiveWindowStart)
			endH, endM := agentlib.ParseHM(af.Config.ActiveWindowEnd)
			dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), startH, startM, 0, 0, loc)
			dayEnd := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), endH, endM, 0, 0, loc)
			if localNow.Before(dayStart) || localNow.After(dayEnd) {
				continue
			}
		}
		if af.Config.NextRunAt.Before(now) || af.Config.NextRunAt.Equal(now) {
			due = append(due, agentlib.DueAgent{CardID: cardID, NextRunAt: *af.Config.NextRunAt})
		}
	}
	return due, nil
}

func (rt *Runtime) stopScheduler() {
	if rt.scheduler != nil {
		rt.scheduler.Stop()
		rt.scheduler = nil
	}
}

func (rt *Runtime) startDueDateScanner() {
	if rt.deps.Repo() == nil {
		return
	}
	prefs, _ := config.LoadPreferences()
	configDir, _ := config.ConfigDir()

	rt.dueDateScanner = agentlib.NewDueDateScanner(
		filepath.Join(rt.deps.Repo().Root, "cards"),
		configDir,
		func(cardID, cardTitle string, threshold time.Duration, overdue bool) {
			notifier := rt.makeNotifier()
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
			rt.markAlarmBlockFired(cardID, blockID)
		},
	)
	rt.dueDateScanner.Configure(prefs.DueDateNotify, prefs.DueDateThresholds, prefs.DueDateChannels)
	rt.dueDateScanner.Start()
}

func (rt *Runtime) markAlarmBlockFired(cardID, blockID string) {
	if rt.deps.Repo() == nil {
		return
	}
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		slog.Warn("alarm: load card failed", "card_id", cardID, "err", err)
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
	if _, err := rt.deps.Repo().UpdateCardBlocks(cardID, card.Blocks); err != nil {
		slog.Warn("alarm: save card failed", "card_id", cardID, "err", err)
		return
	}
	rt.emitCardUpdated(cardID)
}

func (rt *Runtime) stopDueDateScanner() {
	if rt.dueDateScanner != nil {
		rt.dueDateScanner.Stop()
		rt.dueDateScanner = nil
	}
}

func (rt *Runtime) emitCardUpdated(cardID string) {
	if cardID == "" {
		return
	}
	rt.deps.Publish("card:updated", map[string]any{"cardID": cardID})
}

func (rt *Runtime) makeNotifier() *notify.Dispatcher {
	cfg, _ := config.LoadNotifyConfig()
	return notify.NewDispatcher(cfg, func(name string, data any) {
		rt.deps.Publish(name, data)
	})
}

func (rt *Runtime) executeAgent(ctx context.Context, cardID string) error {
	if rt.deps.Repo() == nil {
		return fmt.Errorf("no repository open")
	}

	// 1. Load agent config
	af, err := rt.deps.Repo().GetAgentConfig(cardID)
	if err != nil {
		return fmt.Errorf("load agent config: %w", err)
	}
	if !af.Config.Enabled {
		return nil
	}

	// 2. Set status to running. If the save fails, skip this tick
	// entirely: running anyway would leave the on-disk status stale, and
	// a crash mid-run would re-queue the agent on restart (the in-process
	// `running` map only guards within this process's lifetime).
	now := time.Now().UTC()
	af.Config.Status = model.AgentStatusRunning
	af.Config.RunStartedAt = &now
	if err := rt.deps.Repo().SaveAgentConfig(cardID, af.Config); err != nil {
		slog.Error("agent run skipped: persist running status failed", "cardID", cardID, "err", err)
		return fmt.Errorf("save agent config (mark running): %w", err)
	}
	if rt.deps.Index() != nil {
		rt.logIdxErr("UpdateAgentIndex", rt.deps.Index().UpdateAgentIndex(cardID, true, string(model.AgentStatusRunning), ""))
	}

	// Create cancellable context for this agent run
	agentCtx, agentCancel := context.WithCancel(ctx)
	rt.agentCancels.Store(cardID, agentCancel)
	defer func() {
		agentCancel()
		rt.agentCancels.Delete(cardID)
	}()

	// Emit started event
	rt.deps.Publish("agent:started", map[string]any{"cardID": cardID})

	// 3. Register in llmActors for activity attribution
	rt.deps.LLMActors().Store(cardID, "agent")
	defer rt.deps.LLMActors().Delete(cardID)

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

		// Retry logic for failed runs — the retry-delay math (linear
		// backoff for generic failures, Retry-After-hint-aware +
		// exponential for rate limits) is extracted into
		// internal/agent/retry.go so the policy is covered by unit
		// tests rather than trusting inline logic.
		if run.Status == "failure" && af.Config.MaxRetries > 0 {
			af.Config.RetryCount++
			if af.Config.RetryCount <= af.Config.MaxRetries {
				retryDelay := agentlib.RetryDelay(run.Error, af.Config.RetryBackoffMins, af.Config.RetryCount)
				retryAt := finishedAt.Add(retryDelay)
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
			opts := agentlib.ScheduleOpts{
				StartDate:         af.Config.StartDate,
				EndDate:           af.Config.EndDate,
				ActiveWindowStart: af.Config.ActiveWindowStart,
				ActiveWindowEnd:   af.Config.ActiveWindowEnd,
				OneShot:           af.Config.OneShot,
				LastRunAt:         af.Config.LastRunAt,
				Timezone:          af.Config.Timezone,
			}
			if next, err := agentlib.NextRunTimeWithOpts(af.Config.Schedule, now, opts); err == nil {
				af.Config.NextRunAt = &next
			} else {
				// One-shot completed or past end date — disable
				af.Config.NextRunAt = nil
				if af.Config.OneShot {
					af.Config.Enabled = false
				}
			}
		}

		// Track estimated cost + budget enforcement. BudgetExceeded
		// encapsulates the "0 means unlimited" sentinel, kept as a
		// named helper so the sentinel contract is test-covered.
		if run.TokensUsed > 0 {
			runCost := config.EstimateCost(run.ModelUsed, run.TokensUsed)
			af.Config.CostSpentUSD += runCost

			if agentlib.BudgetExceeded(af.Config.CostSpentUSD, af.Config.CostBudgetUSD) {
				af.Config.Enabled = false
				af.Config.Status = model.AgentStatusDisabled
				cardTitle := ""
				if c, err := rt.deps.Repo().GetCard(cardID); err == nil {
					cardTitle = c.Title
				}
				notifier := rt.makeNotifier()
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

		_ = rt.deps.Repo().SaveAgentConfig(cardID, af.Config)
		_ = rt.deps.Repo().AppendAgentRun(cardID, run)

		// Emit completion event
		eventName := "agent:completed"
		eventData := map[string]any{"cardID": cardID, "status": run.Status, "summary": run.Summary}
		if run.Status == "failure" {
			eventName = "agent:failed"
			eventData["error"] = run.Error
		}
		rt.deps.Publish(eventName, eventData)

		// Dispatch notifications based on agent config. The
		// status-vs-triggers decision is extracted to
		// agentlib.ShouldNotifyForStatus so it's unit-tested against
		// every combination (success/failure/cancelled + each
		// trigger shape) rather than re-derived inline.
		if agentlib.ShouldNotifyForStatus(run.Status, af.Config.NotifyOn) && af.Config.NotifyChannel != "" {
			cardTitle := ""
			if c, err := rt.deps.Repo().GetCard(cardID); err == nil {
				cardTitle = c.Title
			}
			notifier := rt.makeNotifier()
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
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		run.Status = "failure"
		run.Error = fmt.Sprintf("load card: %v", err)
		return err
	}

	// 5. Load LLM provider (per-agent account/model override)
	cfg, provider, err := rt.deps.LLM().LoadProviderForAccount(af.Config.LLMAccountID, af.Config.LLMModel)
	if err != nil || provider == nil {
		run.Status = "failure"
		// Distinguish "nothing configured" from "configured but failed
		// to load" — the run history is where the user debugs this.
		if err != nil {
			run.Error = "LLM provider load failed: " + err.Error()
			return fmt.Errorf("llm provider load failed: %w", err)
		}
		run.Error = "LLM not configured"
		return fmt.Errorf("LLM not configured")
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = llmsvc.DefaultModelForProvider(cfg.Provider)
	}
	run.ModelUsed = modelName
	run.ProviderUsed = cfg.Provider

	// 6. Build system prompt
	systemPrompt := rt.deps.Prompts().Agent(card, af.Config, cfg)

	// 7. Build filtered tools
	toolDefs := llm.AgentTools(af.Config.AllowedTools)
	// Append any MCP tools exposed by the per-repo registry that
	// this agent has been granted via per-card allowed-tools. MCP
	// tool IDs are namespaced (server__tool) so they never collide
	// with built-in tool names. Passing an empty allowed list
	// means "all MCP tools", matching the built-in behaviour.
	toolDefs = append(toolDefs, rt.mcpToolDefs(af.Config.AllowedTools)...)

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

	resultCf, err := rt.deps.ChatRT().RunLoop(runCtx, provider, modelName, cf, chatrt.LoopConfig{
		ChatID:       "__agent__" + cardID,
		SystemPrompt: systemPrompt,
		Tools:        toolDefs,
		MaxIter:      10,
		TokenBudget:     budget,
		TotalTokensUsed: &tokensUsed,
		ExecuteTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := rt.executeAgentToolCall(runCtx, cardID, card, tc)
			if action != nil {
				allToolActions = append(allToolActions, *action)
			}
			return result, action, nil
		},
		FallbackContent: "Agent run completed.",
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

func (rt *Runtime) mcpToolDefs(allowedTools []string) []llm.ToolDef {
	if rt.deps.MCPRegistry() == nil {
		return nil
	}
	tools := rt.deps.MCPRegistry().Tools()
	if len(tools) == 0 {
		return nil
	}
	// Build allow-set once. An empty allow list means "allow all"
	// for both built-ins and MCP tools.
	var allow map[string]bool
	if len(allowedTools) > 0 {
		allow = make(map[string]bool, len(allowedTools))
		for _, t := range allowedTools {
			allow[t] = true
		}
	}
	out := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		if allow != nil && !allow[t.NamespaceID] {
			continue
		}
		// MCP InputSchema is the JSON Schema object we pass
		// verbatim to the LLM. If a server omits it entirely
		// (technically spec-noncompliant but some do it) we
		// supply a minimal object-typed schema so providers
		// that require one don't reject the tool.
		params := t.Tool.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		description := t.Tool.Description
		if description == "" && t.Tool.Title != "" {
			description = t.Tool.Title
		}
		// Prepend the server name to the description so the LLM
		// has context about which external source this tool
		// belongs to — useful when an agent has tools from
		// multiple servers and needs to pick between them.
		description = fmt.Sprintf("[via %s MCP server] %s", t.ServerName, description)
		out = append(out, llm.ToolDef{
			Name:        t.NamespaceID,
			Description: description,
			Parameters:  params,
		})
	}
	return out
}

func (rt *Runtime) executeAgentToolCall(ctx context.Context, cardID string, card *model.Card, tc llm.ToolCall) (string, *model.ToolAction) {
	action := &model.ToolAction{
		Tool:  tc.Name,
		Input: tc.Arguments,
	}

	// MCP tool calls are namespaced. Detect via the registry's
	// OwnsTool check (O(1) map lookup) and route through the
	// MCP registry. This branch runs BEFORE the built-in switch
	// so namespaced IDs can never accidentally match a built-in
	// name, even if a future BRUV release adds a built-in tool
	// with a name that happens to include the separator.
	if rt.deps.MCPRegistry() != nil && rt.deps.MCPRegistry().OwnsTool(tc.Name) {
		return rt.executeMCPToolCall(ctx, tc, action)
	}

	switch tc.Name {
	case "web_fetch":
		url, _ := tc.Arguments["url"].(string)
		result, err := agentlib.WebFetch(url)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = "fetched " + url
		return result, action

	case "web_search":
		query, _ := tc.Arguments["query"].(string)
		result, err := agentlib.WebSearch(query)
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
		result, err := agentlib.HTTPRequest(method, url, body)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = fmt.Sprintf("%s %s", method, url)
		return result, action

	case "notify":
		title, _ := tc.Arguments["title"].(string)
		body, _ := tc.Arguments["body"].(string)
		notifier := rt.makeNotifier()
		// In-app is always included by ParseChannels. Merge in whatever
		// extra channels (system, email, webhook) the user picked on the
		// agent's Permissions tab — otherwise ticking "System" there is
		// silently ignored by tool-initiated notifications.
		channelSpec := "in-app"
		if af, err := rt.deps.Repo().GetAgentConfig(cardID); err == nil && af != nil && af.Config.NotifyChannel != "" {
			channelSpec = "in-app," + af.Config.NotifyChannel
		}
		notifier.Send(notify.Request{
			Title:     title,
			Body:      body,
			Source:    "agent",
			CardID:    cardID,
			CardTitle: card.Title,
			Channels:  notify.ParseChannels(channelSpec),
		})
		action.Result = "notification sent"
		return "Notification sent to user.", action

	case "update_self":
		updatedCard, err := rt.deps.Repo().GetCard(cardID)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		// Optional top-level intrinsic field updates
		if newTitle, ok := tc.Arguments["title"].(string); ok && newTitle != "" {
			updatedCard.Title = newTitle
		}
		if newDueDate, ok := tc.Arguments["due_date"].(string); ok && newDueDate != "" {
			parsed, perr := time.Parse("2006-01-02", newDueDate)
			if perr != nil {
				// Try RFC3339 as fallback
				parsed, perr = time.Parse(time.RFC3339, newDueDate)
			}
			if perr == nil {
				updatedCard.DueDate = &parsed
			}
		}
		if newTags, ok := tc.Arguments["tags"].([]any); ok {
			tags := make([]string, 0, len(newTags))
			for _, t := range newTags {
				if s, ok := t.(string); ok && s != "" {
					tags = append(tags, s)
				}
			}
			if len(tags) > 0 {
				updatedCard.Tags = tags
			}
		}
		updates, _ := tc.Arguments["updates"].([]any)
		for _, u := range updates {
			upd, ok := u.(map[string]any)
			if !ok {
				continue
			}
			key, _ := upd["key"].(string)
			rawValue := upd["value"]
			if key == "" {
				continue
			}
			// LLMs frequently put intrinsic fields in the updates array
			// instead of using top-level parameters. Intercept them here
			// so the actual card fields change rather than creating
			// spurious text blocks — but only when no real block with
			// that key/label exists (a user-created "Tags" block wins).
			if strings.EqualFold(key, "title") && !cardHasBlock(updatedCard, key) {
				if s, ok := rawValue.(string); ok && s != "" {
					updatedCard.Title = s
				}
				continue
			}
			if strings.EqualFold(key, "description") {
				// Description is intrinsic on the card — never a block.
				// Agents that send key="description" target Card.Description
				// regardless of any block keyed similarly (which shouldn't
				// exist post-refactor, but we don't trust that here).
				if s, ok := rawValue.(string); ok {
					updatedCard.Description = s
				} else if rawValue != nil {
					updatedCard.Description = fmt.Sprintf("%v", rawValue)
				}
				continue
			}
			if (strings.EqualFold(key, "due_date") || strings.EqualFold(key, "due date") || strings.EqualFold(key, "duedate")) && !cardHasBlock(updatedCard, key) {
				if s, ok := rawValue.(string); ok && s != "" {
					parsed, perr := time.Parse("2006-01-02", s)
					if perr != nil {
						parsed, perr = time.Parse(time.RFC3339, s)
					}
					if perr == nil {
						updatedCard.DueDate = &parsed
					}
				}
				continue
			}
			if strings.EqualFold(key, "tags") && !cardHasBlock(updatedCard, key) {
				switch v := rawValue.(type) {
				case []any:
					for _, item := range v {
						if s, ok := item.(string); ok && s != "" {
							updatedCard.Tags = append(updatedCard.Tags, s)
						}
					}
				case string:
					if v != "" {
						updatedCard.Tags = append(updatedCard.Tags, v)
					}
				}
				continue
			}
			found := false
			// Match by key first, then by label (case-insensitive) as
			// fallback. tools.CoerceBlockValueForBlock (in app.go) reshapes the
			// raw LLM input to match the target block's type AND applies
			// meta-aware constraints: select/radio option validation,
			// rating and progress clamping. A constraint violation is
			// returned to the LLM so it can retry, rather than silently
			// writing an invalid value.
			for i, b := range updatedCard.Blocks {
				if b.Key == key || strings.EqualFold(b.Label, key) {
					coerced, cerr := tools.CoerceBlockValueForBlock(&updatedCard.Blocks[i], rawValue)
					if cerr != nil {
						slog.Warn("update_self coerce failed",
							"block_key", key, "block_type", b.Type, "err", cerr)
						action.Result = fmt.Sprintf("error: block %q: %v", key, cerr)
						return action.Result, action
					}
					updatedCard.Blocks[i].Value = coerced
					found = true
					break
				}
			}
			if !found {
				// Create a new text block only if no existing block matches.
				// New blocks are always text — the LLM can create a new list
				// block via the editor if needed, but update_self never
				// guesses at block types it didn't request.
				strValue := ""
				if s, ok := rawValue.(string); ok {
					strValue = s
				} else if rawValue != nil {
					strValue = fmt.Sprintf("%v", rawValue)
				}
				updatedCard.Blocks = append(updatedCard.Blocks, model.Block{
					ID:    fmt.Sprintf("blk-%s", uuid.New().String()[:8]),
					Type:  model.BlockText,
					Label: key,
					Key:   strings.ToLower(strings.ReplaceAll(key, " ", "_")),
					Value: strValue,
				})
			}
		}
		updatedCard.UpdatedAt = time.Now().UTC()
		if err := rt.deps.Repo().UpdateCardDirect(cardID, updatedCard); err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		if rt.deps.Index() != nil {
			rt.idxIncrementalRefresh()
		}
		// Notify any open card detail view so it re-fetches the new content.
		rt.emitCardUpdated(cardID)
		action.Result = "card updated"
		return "Card blocks updated successfully.", action

	case "read_card":
		targetID, _ := tc.Arguments["card_id"].(string)
		targetCard, err := rt.deps.Repo().GetCard(targetID)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		action.Result = "read card: " + targetCard.Title
		return promptfmt.FormatCardContent(targetCard), action

	case "create_card":
		title, _ := tc.Arguments["title"].(string)
		cardType, _ := tc.Arguments["card_type"].(string)
		if title == "" {
			action.Result = "error: title is required"
			return action.Result, action
		}
		newCard, err := rt.deps.Repo().CreateCard(cardType, title)
		if err != nil {
			action.Result = "error: " + err.Error()
			return action.Result, action
		}
		if rt.deps.Index() != nil {
			rt.idxIncrementalRefresh()
		}
		action.Result = fmt.Sprintf("created card: %s (%s)", newCard.Title, newCard.ID)
		return fmt.Sprintf("Created card '%s' with ID %s.", newCard.Title, newCard.ID), action

	default:
		action.Result = "unknown tool"
		return fmt.Sprintf("Unknown tool: %s", tc.Name), action
	}
}

func (rt *Runtime) executeMCPToolCall(ctx context.Context, tc llm.ToolCall, action *model.ToolAction) (string, *model.ToolAction) {
	serverName, toolName := mcp.SplitNamespacedTool(tc.Name)
	// Derive from the agent's run context so cancelling the agent also
	// cancels any in-flight MCP call, instead of letting it run to the
	// full 60s on a detached background context.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := rt.deps.MCPRegistry().CallTool(ctx, tc.Name, tc.Arguments)
	if err != nil {
		slog.Warn("mcp tool protocol error",
			"server", serverName, "tool", toolName, "err", err)
		action.Result = fmt.Sprintf("mcp error: %s", err.Error())
		return fmt.Sprintf("MCP tool %q failed: %s", tc.Name, err.Error()), action
	}

	rawContent := mcp.FlattenContent(result.Content)
	content, truncated := truncateMCPOutput(rawContent)

	// Audit log: every MCP tool invocation, with argument + result
	// sizes. Argument count is often more useful than value for
	// diagnosing "the model called this with wrong params" bugs; we
	// keep the payload small so the log stays readable.
	slog.Info("mcp tool call",
		"server", serverName,
		"tool", toolName,
		"arg_count", len(tc.Arguments),
		"raw_bytes", len(rawContent),
		"returned_bytes", len(content),
		"truncated", truncated,
		"is_error", result.IsError)

	if result.IsError {
		// Tool-level error — prefix with "Error:" so the LLM
		// recognises this as a failure it should react to.
		action.Result = "mcp tool error"
		if content == "" {
			content = "tool returned an error without a message"
		}
		return "Error: " + content, action
	}

	action.Result = fmt.Sprintf("mcp[%s].%s ok", serverName, toolName)
	if content == "" {
		content = "(tool returned no content)"
	}
	return content, action
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

func cardHasBlock(card *model.Card, name string) bool {
	for _, b := range card.Blocks {
		if strings.EqualFold(b.Key, name) || strings.EqualFold(b.Label, name) {
			return true
		}
	}
	return false
}

func truncateMCPOutput(content string) (string, bool) {
	if len(content) <= mcpOutputLimit {
		return content, false
	}
	return content[:mcpOutputLimit] + "\n\n[truncated: output exceeded 8KB limit]", true
}

