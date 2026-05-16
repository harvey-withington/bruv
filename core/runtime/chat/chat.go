package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bruv/core/runtime/tools"
	chatsvc "bruv/core/services/chat"
	llmsvc "bruv/core/services/llm"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"

	"github.com/google/uuid"
)

// LoopConfig holds the per-call configuration for RunLoop.
type LoopConfig struct {
	ChatID       string
	SystemPrompt string
	Tools        []llm.ToolDef
	MaxIter      int

	// AllowDuplicateTool: tool names in this set bypass dedup (e.g. "create_card")
	AllowDuplicateTool map[string]bool

	// ExecuteTool runs a single tool call. Returns (result, action, pinSuggestion).
	// Project chat returns nil for pinSuggestion.
	ExecuteTool func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion)

	// StageTool is non-nil only for card chat in suggest mode.
	StageTool func(tc llm.ToolCall) (string, []model.PendingEdit)

	// AfterToolsRun is called after each iteration's tool calls (e.g. reload card, rebuild tools, nudge).
	// It receives the tool calls from the current iteration and the accumulated state,
	// and returns updated tools + optional system nudge messages to inject.
	AfterToolsRun func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message)

	// suggest mode: if true, ExecuteTool is ignored and StageTool is used instead.
	SuggestMode bool

	// MinConfidence filters pin suggestions below this threshold.
	MinConfidence string

	// FallbackContent is the assistant message if max iterations are reached.
	FallbackContent string

	// TokenBudget is the maximum total tokens allowed across all iterations (0 = unlimited).
	TokenBudget int
	// TotalTokensUsed is written back with the cumulative token count after the loop finishes.
	TotalTokensUsed *int
}

func (rt *Runtime) RunLoop(ctx context.Context, provider llm.Provider, modelName string, cf *model.ChatFile, lc LoopConfig) (*model.ChatFile, error) {
	// Convert chat history to LLM messages
	var llmMessages []llm.Message
	for _, m := range cf.Messages {
		if m.Role == model.RoleUser || m.Role == model.RoleAssistant {
			llmMessages = append(llmMessages, llm.Message{Role: m.Role, Content: m.Content})
		}
	}

	var allToolActions []model.ToolAction
	var pinSuggestion *model.PinSuggestion
	var allPendingEdits []model.PendingEdit
	var cumulativeTokens int
	toolDefs := lc.Tools

	for iteration := 0; iteration < lc.MaxIter; iteration++ {
		resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
			SystemPrompt: lc.SystemPrompt,
			Messages:     llmMessages,
			Model:        modelName,
			Tools:        toolDefs,
		})
		if err != nil {
			errMsg := model.ChatMessage{
				ID:        uuid.New().String(),
				Role:      model.RoleSystem,
				Content:   "Error: " + err.Error(),
				Timestamp: time.Now().UTC(),
			}
			cf, _ = config.AppendChatMessage(rt.deps.Repo().Manifest.ID,lc.ChatID, errMsg)
			if lc.TotalTokensUsed != nil {
				*lc.TotalTokensUsed = cumulativeTokens
			}
			return cf, nil
		}

		// Accumulate token usage
		if resp.Usage != nil {
			cumulativeTokens += resp.Usage.TotalTokens
		}

		// Check token budget
		if lc.TokenBudget > 0 && cumulativeTokens > lc.TokenBudget {
			budgetMsg := model.ChatMessage{
				ID:        uuid.New().String(),
				Role:      model.RoleSystem,
				Content:   fmt.Sprintf("Token budget exceeded (%d / %d). Stopping.", cumulativeTokens, lc.TokenBudget),
				Timestamp: time.Now().UTC(),
			}
			cf, _ = config.AppendChatMessage(rt.deps.Repo().Manifest.ID,lc.ChatID, budgetMsg)
			if lc.TotalTokensUsed != nil {
				*lc.TotalTokensUsed = cumulativeTokens
			}
			return cf, fmt.Errorf("token budget exceeded (%d / %d)", cumulativeTokens, lc.TokenBudget)
		}

		// No tool calls — final text response
		if len(resp.ToolCalls) == 0 {
			assistantMsg := model.ChatMessage{
				ID:            uuid.New().String(),
				Role:          model.RoleAssistant,
				Content:       resp.Content,
				Timestamp:     time.Now().UTC(),
				ToolActions:   allToolActions,
				PinSuggestion: pinSuggestion,
				PendingEdits:  allPendingEdits,
			}
			cf, _ = config.AppendChatMessage(rt.deps.Repo().Manifest.ID,lc.ChatID, assistantMsg)
			if lc.TotalTokensUsed != nil {
				*lc.TotalTokensUsed = cumulativeTokens
			}
			return cf, nil
		}

		// Add assistant message with tool calls to conversation
		llmMessages = append(llmMessages, llm.Message{
			Role:      model.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Deduplicate tool calls within same response
		seenCalls := make(map[string]bool)
		for _, tc := range resp.ToolCalls {
			if seenCalls[tc.Name] && !lc.AllowDuplicateTool[tc.Name] {
				llmMessages = append(llmMessages, llm.Message{
					Role:       "tool",
					Content:    "Skipped — duplicate call",
					ToolCallID: tc.ID,
				})
				continue
			}
			seenCalls[tc.Name] = true

			var result string
			if lc.SuggestMode && lc.StageTool != nil {
				var edits []model.PendingEdit
				result, edits = lc.StageTool(tc)
				allPendingEdits = append(allPendingEdits, edits...)
			} else {
				var action *model.ToolAction
				var ps *model.PinSuggestion
				result, action, ps = lc.ExecuteTool(tc)
				if action != nil {
					allToolActions = append(allToolActions, *action)
				}
				if ps != nil && config.ConfidenceMeetsThreshold(ps.Confidence, lc.MinConfidence) {
					pinSuggestion = ps
				}
			}

			llmMessages = append(llmMessages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// Post-iteration hook: rebuild tools, inject nudge messages
		if lc.AfterToolsRun != nil {
			var nudges []llm.Message
			toolDefs, nudges = lc.AfterToolsRun(resp.ToolCalls, toolDefs, pinSuggestion, allPendingEdits)
			llmMessages = append(llmMessages, nudges...)
		}
	}

	// Max iterations reached — save what we have
	assistantMsg := model.ChatMessage{
		ID:            uuid.New().String(),
		Role:          model.RoleAssistant,
		Content:       lc.FallbackContent,
		Timestamp:     time.Now().UTC(),
		ToolActions:   allToolActions,
		PinSuggestion: pinSuggestion,
		PendingEdits:  allPendingEdits,
	}
	cf, _ = config.AppendChatMessage(rt.deps.Repo().Manifest.ID,lc.ChatID, assistantMsg)
	if lc.TotalTokensUsed != nil {
		*lc.TotalTokensUsed = cumulativeTokens
	}
	return cf, nil
}

func (rt *Runtime) SendProject(brandSlug, streamSlug, projectSlug, userMessage, contextLevel string) (*model.ChatFile, error) {
	if rt.deps.Repo() == nil {
		return nil, fmt.Errorf("no repository open")
	}

	project, err := rt.deps.Repo().GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := chatsvc.ProjectChatID(project.ID)

	// Save user message
	cf, err := rt.saveUserMessage(chatID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := rt.deps.LLM().LoadProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Build system prompt with project context
	categories, _ := rt.deps.Repo().ListCategories(brandSlug, streamSlug, projectSlug)
	brand, _ := rt.deps.Repo().GetBrand(brandSlug)
	stream, _ := rt.deps.Repo().GetStream(brandSlug, streamSlug)
	systemPrompt := rt.deps.Prompts().Project(brandSlug, streamSlug, projectSlug, brand, stream, project, categories, cfg, model.ProjectChatContextLevel(contextLevel))

	// Build tool definitions
	var catMaps []map[string]string
	for _, cat := range categories {
		catMaps = append(catMaps, map[string]string{"id": cat.ID, "breadcrumb": cat.Name})
	}
	cardTypes := rt.deps.Registry().List()
	toolDefs := llm.ProjectTools(cardTypes, catMaps)

	// Build the per-call scope for tool callbacks. Both staging and execute
	// callbacks use the cardIDs set to reject IDs the LLM hallucinated or
	// copied from a different project; the slugs let project-metadata tools
	// (update_project, *_label, *_category) target the right project.
	scope := tools.ProjectChatScope{
		BrandSlug:   brandSlug,
		StreamSlug:  streamSlug,
		ProjectSlug: projectSlug,
		CardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := rt.deps.Repo().ListCardsInCategory(cat.ID)
		for _, p := range pins {
			scope.CardIDs[p.CardID] = true
		}
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = llmsvc.DefaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(rt.deps.Ctx(), 120*time.Second)
	defer cancel()

	suggestMode := cfg.AIMode == "suggest"
	return rt.RunLoop(ctx, provider, modelName, cf, LoopConfig{
		ChatID:             chatID,
		SystemPrompt:       systemPrompt,
		Tools:              toolDefs,
		MaxIter:            5,
		// Per-entity mutating tools must be callable multiple times in one
		// iteration so the LLM can act on several distinct targets in a single
		// turn (e.g. set an icon on every category, move several cards). The
		// bulk variant `update_cards` exists for the most common case but the
		// LLM doesn't always reach for it. Query/read tools are deliberately
		// NOT whitelisted — those should be deduped to stop runaway loops.
		AllowDuplicateTool: map[string]bool{
			"create_card":          true,
			"update_card":          true,
			"move_card":            true,
			"add_tags_to_cards":    true,
			"configure_agent":      true,
			"create_category":      true,
			"update_category":      true,
			"delete_category":      true,
			"create_project_tag":   true,
			"update_project_tag":   true,
			"delete_project_tag":   true,
		},
		ExecuteTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := rt.deps.Tools().ExecuteProject(tc, scope)
			return result, action, nil
		},
		StageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return rt.deps.Tools().StageProject(tc, scope)
		},
		SuggestMode:     suggestMode,
		FallbackContent: "I've made the requested changes to your project.",
	})
}

func (rt *Runtime) SendCard(cardID, userMessage string) (*model.ChatFile, error) {
	if rt.deps.Repo() == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Save user message
	cf, err := rt.saveUserMessage(cardID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := rt.deps.LLM().LoadProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Attribute all card edits in this chat turn to the LLM model
	rt.deps.LLMActors().Store(cardID, cfg.Model)
	defer rt.deps.LLMActors().Delete(cardID)

	// Load card for context
	card, err := rt.deps.Repo().GetCard(cardID)
	if err != nil {
		return cf, nil
	}

	systemPrompt := rt.deps.Prompts().Card(card, cfg)

	// Build tool definitions
	cardTypes := rt.deps.Registry().List()
	allCats, _ := rt.deps.Card().ListAllCategories()
	var catMaps []map[string]string
	for _, c := range allCats {
		catMaps = append(catMaps, map[string]string{"id": c.CategoryID, "breadcrumb": c.Breadcrumb})
	}

	buildToolDefs := func(c *model.Card) []llm.ToolDef {
		// Collect MCP tool IDs so configure_agent's allowed_tools enum
		// includes them — lets the LLM add MCP tools to agents via chat.
		var mcpToolIDs []string
		if rt.deps.MCPRegistry() != nil {
			for _, t := range rt.deps.MCPRegistry().Tools() {
				mcpToolIDs = append(mcpToolIDs, t.NamespaceID)
			}
		}
		tools := llm.CardTools(cardTypes, catMaps, mcpToolIDs)
		if c != nil && len(c.Blocks) > 0 {
			fieldProps := make(map[string]any)
			for _, b := range c.Blocks {
				if b.Key == "" {
					continue
				}
				var prop map[string]any
				switch b.Type {
				case model.BlockChecklist:
					prop = map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of checklist item texts. Each string becomes an unchecked item.",
					}
				case model.BlockCheckbox:
					prop = map[string]any{"type": "boolean"}
				case model.BlockNumber:
					prop = map[string]any{"type": "number"}
				default:
					prop = map[string]any{"type": "string"}
				}
				if b.Meta != nil {
					if desc, ok := b.Meta["description"].(string); ok && desc != "" {
						prop["description"] = desc
					}
					if opts, ok := b.Meta["options"].([]string); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
					if opts, ok := b.Meta["options"].([]any); ok && len(opts) > 0 {
						prop["enum"] = opts
					}
				}
				fieldProps[b.Key] = prop
			}
			if len(fieldProps) > 0 {
				for i, t := range tools {
					if t.Name == "set_fields" {
						tools[i] = llm.ToolDef{
							Name:        "set_fields",
							Description: "Fill in field values on the card. ALWAYS call this to write content into fields.",
							Parameters:  map[string]any{"type": "object", "properties": fieldProps},
						}
						break
					}
				}
			}
		}
		return tools
	}

	// Chat mode: the user has opted out of card-mutating tools, but
	// web research is still fair game (doesn't touch the card) — else
	// "can you look this up" is permanently broken in chat mode.
	// Edit/suggest modes get the full card-tool surface.
	var toolDefs []llm.ToolDef
	if cfg.AIMode == "chat" {
		toolDefs = llm.WebTools()
	} else {
		toolDefs = buildToolDefs(card)
	}
	slog.Info("card chat tools assembled",
		"cardID", cardID, "ai_mode", cfg.AIMode, "tool_count", len(toolDefs))

	modelName := cfg.Model
	if modelName == "" {
		modelName = llmsvc.DefaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(rt.deps.Ctx(), 120*time.Second)
	defer cancel()

	return rt.RunLoop(ctx, provider, modelName, cf, LoopConfig{
		ChatID:       cardID,
		SystemPrompt: systemPrompt,
		Tools:        toolDefs,
		// 6 iterations comfortably covers web_search → web_fetch →
		// summarise, or a couple of card-tool rounds plus a final
		// message. Previously 3, which was too tight for research
		// flows — the loop would exhaust before the model got to
		// speak, triggering the fallbackContent lie below.
		MaxIter: 6,
		SuggestMode:  cfg.AIMode == "suggest",
		StageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return rt.deps.Tools().StageCard(tc, allCats)
		},
		ExecuteTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			return rt.deps.Tools().ExecuteCard(cardID, card, tc, allCats)
		},
		MinConfidence: cfg.MinConfidence,
		AfterToolsRun: func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message) {
			var nudges []llm.Message

			// Reload card after tool execution (it may have changed)
			if cfg.AIMode != "suggest" {
				card, _ = rt.deps.Repo().GetCard(cardID)
				tools = buildToolDefs(card)
			}

			// Nudge to fill empty fields
			calledSetFields := false
			for _, tc := range calls {
				if tc.Name == "set_fields" || tc.Name == "update_blocks" {
					calledSetFields = true
					break
				}
			}
			if !calledSetFields && card != nil && len(card.Blocks) > 0 {
				var emptyKeys []string
				for _, b := range card.Blocks {
					if b.Key != "" {
						if v, ok := b.Value.(string); ok && v == "" {
							emptyKeys = append(emptyKeys, b.Key)
						}
					}
				}
				if len(emptyKeys) > 0 {
					nudges = append(nudges, llm.Message{
						Role:    model.RoleUser,
						Content: fmt.Sprintf("[System: The card has empty fields that need content: %s. Use set_fields to fill them based on the conversation.]", strings.Join(emptyKeys, ", ")),
					})
				}
			}

			// Nudge for pin
			calledPin := false
			for _, tc := range calls {
				if tc.Name == "suggest_pin" {
					calledPin = true
					break
				}
			}
			suggestPinStaged := false
			for _, e := range edits {
				if e.Tool == "suggest_pin" {
					suggestPinStaged = true
					break
				}
			}
			if !calledPin && pin == nil && !suggestPinStaged {
				existingPins, _ := rt.deps.Repo().GetCardPins(cardID)
				if len(existingPins) == 0 {
					nudges = append(nudges, llm.Message{
						Role:    model.RoleUser,
						Content: "[System: This card has no pin location yet. Use suggest_pin to pin it to the best-fit category.]",
					})
				}
			}

			return tools, nudges
		},
		// Fallback is only reached when the model keeps calling tools
		// without ever producing a final text reply. Previously this
		// said "I've made the changes to your card." — actively
		// misleading when the model was researching, not editing.
		// Honest + generic: the user can see the tool actions that
		// fired above this message and ask a follow-up if needed.
		FallbackContent: "I hit my tool-call limit before I could write a reply. The tools above show what ran — ask again or narrow the request if you'd like a summary.",
	})
}

func (rt *Runtime) saveUserMessage(chatID, userMessage string) (*model.ChatFile, error) {
	userMsg := model.ChatMessage{
		ID:        uuid.New().String(),
		Role:      model.RoleUser,
		Content:   repo.SanitizeText(userMessage),
		Timestamp: time.Now().UTC(),
	}
	return config.AppendChatMessage(rt.deps.Repo().Manifest.ID,chatID, userMsg)
}
