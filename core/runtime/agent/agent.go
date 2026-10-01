package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/tools"
	llmsvc "bruv/core/services/llm"
	"bruv/core/services/llm/routing"
	agentlib "bruv/internal/agent"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/notify"
	"bruv/internal/repo"
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
		// An agent whose card is missing (deleted, or not synced yet)
		// never runs; its file is left for the card to arrive.
		if !rt.deps.Repo().CardExists(cardID) {
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
				rt.patchAgentConfig(cardID, "reset stuck agent", func(cfg *model.AgentConfig) {
					if cfg.Status == model.AgentStatusRunning {
						cfg.Status = model.AgentStatusIdle
						cfg.RunStartedAt = nil
					}
				})
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
			rt.patchAgentConfig(cardID, "disable past end date", func(cfg *model.AgentConfig) {
				if cfg.EndDate != nil && now.After(*cfg.EndDate) {
					cfg.Enabled = false
					cfg.Status = model.AgentStatusDisabled
					cfg.NextRunAt = nil
				}
			})
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

// patchAgentConfig applies a runtime-owned change to an agent's current
// config. Best-effort: a gone agent is skipped, a failed write logged.
func (rt *Runtime) patchAgentConfig(cardID, op string, patch func(cfg *model.AgentConfig)) {
	_, err := rt.deps.Repo().UpdateAgentConfig(cardID, func(cfg *model.AgentConfig) error {
		patch(cfg)
		return nil
	})
	if err != nil && !agentGone(err) {
		slog.Error("agent config write failed", "op", op, "card_id", cardID, "err", err)
	}
}

func (rt *Runtime) stopScheduler() {
	if rt.scheduler != nil {
		rt.scheduler.Stop()
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
	_, err := rt.deps.Repo().MutateCard(cardID, func(card *model.Card) error {
		for i := range card.Blocks {
			if card.Blocks[i].ID == blockID {
				if card.Blocks[i].Meta == nil {
					card.Blocks[i].Meta = make(map[string]any)
				}
				card.Blocks[i].Meta["alarm_fired"] = true
				return nil
			}
		}
		return repo.ErrNoChange // the block was removed meanwhile
	})
	if err != nil {
		slog.Warn("alarm: mark block fired failed", "card_id", cardID, "err", err)
		return
	}
	rt.emitCardUpdated(cardID)
}

func (rt *Runtime) stopDueDateScanner() {
	if rt.dueDateScanner != nil {
		rt.dueDateScanner.Stop()
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

	// 1-2. Mark the agent running on its current config, which is also
	// the config this run uses. A disabled or gone agent (or card) is
	// skipped. If the save fails, skip this tick entirely: running anyway
	// would leave the on-disk status stale, and a crash mid-run would
	// re-queue the agent on restart (the in-process `running` map only
	// guards within this process's lifetime).
	agentCfg, err := rt.markRunning(cardID, time.Now().UTC())
	switch {
	case errors.Is(err, errAgentNotEnabled):
		return nil
	case agentGone(err):
		slog.Warn("agent run skipped: agent or its card is missing", "cardID", cardID, "err", err)
		return nil
	case err != nil:
		slog.Error("agent run skipped: persist running status failed", "cardID", cardID, "err", err)
		return fmt.Errorf("save agent config (mark running): %w", err)
	}
	af := &model.AgentFile{CardID: cardID, Config: *agentCfg}
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

	// Emit started event, and show the run on the card itself.
	rt.deps.Publish("agent:started", map[string]any{"cardID": cardID})
	afterStart := rt.stampCard(cardID, startValues(), nil)

	// 3. Register in llmActors for activity attribution
	rt.deps.LLMActors().Store(cardID, "agent")
	defer rt.deps.LLMActors().Delete(cardID)

	// Use the cancellable context from here on
	ctx = agentCtx

	run := newRun(cardID)

	// Defer: finalize run, update status, calculate next run
	defer func() {
		finishedAt := time.Now().UTC()
		run.FinishedAt = &finishedAt
		rt.finishRun(cardID, run, finishedAt, afterStart, af.Config)
	}()

	// 4. Load card
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		run.Status = "failure"
		run.Error = fmt.Sprintf("load card: %v", err)
		return err
	}

	// 5. Load AI behaviour config (user context for the prompt)
	cfg, err := rt.deps.LLM().GetConfig()
	if err != nil {
		run.Status = "failure"
		run.Error = "LLM config load failed: " + err.Error()
		return fmt.Errorf("llm config load failed: %w", err)
	}

	// 6. Build system prompt
	systemPrompt := rt.deps.Prompts().Agent(card, af.Config, cfg)

	// 7. Build the tools this agent was granted: agent-only built-ins,
	// native board tools and MCP tools (server__tool ids, which never
	// collide with built-in names). An empty allowed_tools grants none.
	toolDefs := append(llm.AgentTools(af.Config.AllowedTools), rt.nativeToolDefs(af.Config.AllowedTools)...)
	toolDefs = append(toolDefs, rt.mcpToolDefs(af.Config.AllowedTools)...)
	offered := offeredSet(toolDefs)
	guard := newRunGuard()

	// Select the model: the agent's own choice (or its pre-routing
	// account/model pair), else the agent_run task's assignment.
	sel, err := rt.deps.LLM().Select(ctx, llmsvc.RouteRequest{
		Task:            llmsvc.TaskAgentRun,
		Choice:          config.ModelRef(af.Config.LLM),
		LegacyAccountID: af.Config.LLMAccountID,
		LegacyModel:     af.Config.LLMModel,
		UserMessage:     af.Config.Goal,
		ContextTokens:   routing.EstimateTokens(systemPrompt),
		ToolsOffered:    len(toolDefs) > 0,
	})
	if err != nil || sel == nil {
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
	run.ModelUsed = sel.Model
	run.ProviderUsed = sel.Decision.Provider
	run.Route = &sel.Decision

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
	maxTurns := af.Config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = model.DefaultAgentMaxTurns
	}
	var exhausted bool

	resultCf, err := rt.deps.ChatRT().RunLoop(runCtx, sel.Provider, sel.Model, cf, chatrt.LoopConfig{
		ChatID:          "__agent__" + cardID,
		SystemPrompt:    systemPrompt,
		Tools:           toolDefs,
		MaxIter:         maxTurns,
		TokenBudget:     budget,
		TotalTokensUsed: &tokensUsed,
		// Out of turns: one tool-less call so the run still ends with a
		// real report, and the run is marked failed below — hitting a
		// limit is never a successful run (the token budget already
		// fails the same way).
		Exhausted: &exhausted,
		WrapUpPrompt: "You have used all of this run's turns and cannot call any more tools. " +
			"Report now: what you completed, what you did not get to, and anything the user should check.",
		// Stop a run that keeps failing (a blocked site, a dead API)
		// instead of letting it search until the budget runs out.
		Stop: guard.stopReason,
		ExecuteTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			if !offered[tc.Name] {
				return fmt.Sprintf("error: this agent isn't allowed to use %s", tc.Name), nil, nil
			}
			if note, repeat := guard.repeat(tc); repeat {
				return note, nil, nil
			}
			result, action := rt.executeAgentToolCall(runCtx, cardID, card, tc)
			guard.record(result)
			if action != nil {
				allToolActions = append(allToolActions, *action)
			}
			return result, action, nil
		},
		FallbackContent: "The run stopped at its turn limit before the agent wrote a report.",
	})

	run.TokensUsed = tokensUsed

	// Always record tool actions, even on failure (partial runs)
	run.ToolCalls = allToolActions

	// 10. Status, error and summary.
	return concludeRun(&run, ctx.Err(), err, resultCf, exhausted, maxTurns)
}

// newRun is a run record as it starts: FAILED, marked a success only by
// concludeRun once the run really finishes. A panic the scheduler
// recovers still runs executeAgent's deferred finalize, which must not
// record the run as a success.
func newRun(cardID string) model.AgentRun {
	return model.AgentRun{
		ID:        uuid.New().String()[:8],
		CardID:    cardID,
		StartedAt: time.Now().UTC(),
		Status:    "failure",
		Error:     "the run stopped unexpectedly (internal error)",
	}
}

// concludeRun settles a run from how RunLoop ended: ctxErr is the run
// context's error (a cancel), loopErr and cf are RunLoop's results.
func concludeRun(run *model.AgentRun, ctxErr, loopErr error, cf *model.ChatFile, exhausted bool, maxTurns int) error {
	// A cancel comes back from RunLoop as a provider "Error:" message with
	// a nil error, so it is checked first: a cancelled run is not a
	// failure — no retry, no failure notification, no "Failed" stamp.
	if ctxErr != nil {
		run.Status = "cancelled"
		run.Error = "cancelled by user"
		return ctxErr
	}
	if loopErr != nil {
		run.Error = loopErr.Error()
		return loopErr
	}

	// Take the outcome from the message THIS run appended. The agent's
	// chat file keeps every earlier run too, so walking further back could
	// report a previous run's summary as this one's.
	lastMsg, ok := runReply(cf, run.StartedAt)
	if !ok {
		run.Error = "the run finished without saving a reply"
		return nil
	}
	// A provider error (e.g. network failure) ends the loop with a system
	// "Error:" message and a nil error.
	if lastMsg.Role == model.RoleSystem && strings.HasPrefix(lastMsg.Content, "Error: ") {
		run.Error = strings.TrimPrefix(lastMsg.Content, "Error: ")
		return nil
	}
	if lastMsg.Role == model.RoleAssistant {
		run.Summary = lastMsg.Content
	}
	// A reply that was cut off, refused or empty is not a finished run,
	// whatever text came with it.
	if n := lastMsg.Notice; n != nil && n.Code != model.ChatNoticeOverBudget {
		run.Error = n.Text()
		return nil
	}

	if exhausted {
		run.Error = fmt.Sprintf("ran out of turns (%d) before finishing; raise the agent's max turns or narrow its goal", maxTurns)
		return nil
	}
	run.Status, run.Error = "success", ""
	return nil
}

// runReply returns the message RunLoop appended for this run — the last
// one in the chat, provided it was written after the run started.
func runReply(cf *model.ChatFile, startedAt time.Time) (model.ChatMessage, bool) {
	if cf == nil || len(cf.Messages) == 0 {
		return model.ChatMessage{}, false
	}
	last := cf.Messages[len(cf.Messages)-1]
	if last.Timestamp.Before(startedAt) {
		return model.ChatMessage{}, false
	}
	return last, true
}

func (rt *Runtime) mcpToolDefs(allowedTools []string) []llm.ToolDef {
	reg := rt.deps.MCPRegistry() // one snapshot: it can be swapped or nil'd meanwhile
	if reg == nil {
		return nil
	}
	tools := reg.Tools()
	if len(tools) == 0 {
		return nil
	}
	// Only granted MCP tools; an empty allow list grants none.
	if len(allowedTools) == 0 {
		return nil
	}
	allow := allowedSet(allowedTools)
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
	// One registry snapshot for the whole call: MCPRegistry() can be
	// swapped or nil'd between calls (a settings reload).
	if reg := rt.deps.MCPRegistry(); reg != nil && reg.OwnsTool(tc.Name) {
		return rt.executeMCPToolCall(ctx, reg, tc, action)
	}

	// Web tools: one implementation shared with card and project chat.
	if result, webAction, ok := tools.RunWebTool(tc); ok {
		return result, webAction
	}

	// BRUV's native board tools (get_card, create_card, update_card,
	// search_cards, …) — the same registry MCP and chat use, board-wide.
	if n := rt.deps.Native(); n != nil && n.Has(tc.Name) {
		result, isErr := n.Call(nil, tc.Name, tc.Arguments)
		action.Result = result
		if !isErr {
			action.Result = n.Summary(tc.Name, tc.Arguments, result)
		}
		return result, action
	}

	switch tc.Name {
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
		// The native update_card, pinned to this agent's own card: one
		// update implementation, saved through the card service so the
		// activity log and live events see the agent's edits.
		n := rt.deps.Native()
		if n == nil {
			action.Result = "error: card tools unavailable"
			return action.Result, action
		}
		args := make(map[string]any, len(tc.Arguments)+1)
		for k, v := range tc.Arguments {
			args[k] = v
		}
		args["card_id"] = cardID
		result, isErr := n.Call(nil, "update_card", args)
		if isErr {
			action.Result = result
			return result, action
		}
		action.Result = "card updated"
		return "Card blocks updated successfully.", action

	default:
		action.Result = "unknown tool"
		return fmt.Sprintf("Unknown tool: %s", tc.Name), action
	}
}

func (rt *Runtime) executeMCPToolCall(ctx context.Context, reg *mcp.Registry, tc llm.ToolCall, action *model.ToolAction) (string, *model.ToolAction) {
	serverName, toolName := mcp.SplitNamespacedTool(tc.Name)
	// Derive from the agent's run context so cancelling the agent also
	// cancels any in-flight MCP call, instead of letting it run to the
	// full 60s on a detached background context.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := reg.CallTool(ctx, tc.Name, tc.Arguments)
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

// finishRun records a finished run: the agent's runtime-owned config
// fields, the run history, the card's tracking blocks, the completion
// event and notifications. runCfg is the config the run started with,
// used only if the current one can't be written. An agent or card removed
// during the run gets no record — writing one would re-create it.
func (rt *Runtime) finishRun(cardID string, run model.AgentRun, finishedAt time.Time, afterStart fieldSnapshot, runCfg model.AgentConfig) {
	defer rt.publishRunEnd(cardID, run)

	cfg, budgetExceeded, err := rt.finishRunConfig(cardID, run, finishedAt)
	if agentGone(err) {
		slog.Warn("agent run not recorded: agent or its card was removed during the run", "cardID", cardID, "err", err)
		return
	}
	if err != nil {
		slog.Error("agent run: save config failed", "cardID", cardID, "err", err)
		cfg = &runCfg
	}
	if err := rt.deps.Repo().AppendAgentRun(cardID, run); err != nil {
		slog.Error("agent run: save run history failed", "cardID", cardID, "err", err)
	}
	rt.finishStamp(cardID, run, finishedAt, afterStart)

	notifyAgentEvent := agentlib.ShouldNotifyForStatus(run.Status, cfg.NotifyOn) && cfg.NotifyChannel != ""
	if !budgetExceeded && !notifyAgentEvent {
		return
	}
	cardTitle := ""
	if c, err := rt.deps.Repo().GetCard(cardID); err == nil {
		cardTitle = c.Title
	}
	notifier := rt.makeNotifier()
	if budgetExceeded {
		notifier.Send(notify.Request{
			Title:     fmt.Sprintf("Budget exceeded: %s", cardTitle),
			Body:      fmt.Sprintf("Agent disabled — cost $%.4f exceeded budget $%.2f", cfg.CostSpentUSD, cfg.CostBudgetUSD),
			Source:    "budget",
			CardID:    cardID,
			CardTitle: cardTitle,
			Channels:  notify.ParseChannels("in-app,system"),
		})
	}
	// The status-vs-triggers decision is agentlib.ShouldNotifyForStatus,
	// unit-tested against every status/trigger combination.
	if notifyAgentEvent {
		notifier.Send(notify.Request{
			Title:     fmt.Sprintf("Agent %s: %s", run.Status, cardTitle),
			Body:      run.Summary,
			Source:    "agent",
			CardID:    cardID,
			CardTitle: cardTitle,
			Channels:  notify.ParseChannels(cfg.NotifyChannel),
		})
	}
}

// publishRunEnd emits agent:completed / agent:failed for a finished run.
func (rt *Runtime) publishRunEnd(cardID string, run model.AgentRun) {
	eventName := "agent:completed"
	eventData := map[string]any{"cardID": cardID, "status": run.Status, "summary": run.Summary}
	if run.Status == "failure" {
		eventName = "agent:failed"
		eventData["error"] = run.Error
	}
	rt.deps.Publish(eventName, eventData)
}

func truncateMCPOutput(content string) (string, bool) {
	if len(content) <= mcpOutputLimit {
		return content, false
	}
	return content[:mcpOutputLimit] + "\n\n[truncated: output exceeded 8KB limit]", true
}
