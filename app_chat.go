package main

// LLM chat loops and system prompt assembly.
//
// Two chats share this file: per-card chat (SendChatMessage) and
// project-level chat (SendProjectChatMessage). Both hit the shared
// runChatLoop which runs the LLM tool-calling loop iteration by
// iteration until the model settles or hits maxIter. The difference is
// in the tool catalogue and scope-validation: card chat operates on a
// single card, project chat operates on a bag of cards with
// cardIDs-in-scope filtering so the LLM can't hallucinate mutations
// against cards outside the project.
//
// System prompts are assembled here too — they're long, they're
// specific to the mode (card vs project, edit vs suggest vs chat-only),
// and they move in lockstep with the tool catalogue. Keeping them
// next to the loop that uses them makes prompt tuning readable.
//
// Extracted from app.go so prompt edits don't require scrolling through
// the card CRUD surface to find them.

import (
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- Chat ---

func (a *App) LoadChatHistory(cardID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return config.LoadChatFor(a.repo.Manifest.ID,cardID)
}

// projectChatID returns the virtual chat ID used to store project-level chat messages.
func projectChatID(projectID string) string {
	return "__project__" + projectID
}

// LoadProjectChatHistory returns the chat history for a project.
func (a *App) LoadProjectChatHistory(brandSlug, streamSlug, projectSlug string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	return config.LoadChatFor(a.repo.Manifest.ID,projectChatID(project.ID))
}

// ClearProjectChatHistory deletes all messages in a project's AI chat.
func (a *App) ClearProjectChatHistory(brandSlug, streamSlug, projectSlug string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return err
	}
	chatID := projectChatID(project.ID)
	return config.SaveChatFor(a.repo.Manifest.ID,&model.ChatFile{CardID: chatID, Messages: []model.ChatMessage{}})
}

// ClearCardChatHistory deletes all messages in a card's AI chat.
func (a *App) ClearCardChatHistory(cardID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	return config.SaveChatFor(a.repo.Manifest.ID,&model.ChatFile{CardID: cardID, Messages: []model.ChatMessage{}})
}

// SendProjectChatMessage sends a message in the project-level AI chat.
// Context is assembled from all cards pinned to the project, grouped by category.
// The LLM has tools to create cards, bulk-tag, move cards, and update cards.
// --- Chat loop infrastructure ---

// chatLoopConfig holds the per-call configuration for runChatLoop.
type chatLoopConfig struct {
	chatID       string
	systemPrompt string
	tools        []llm.ToolDef
	maxIter      int

	// allowDuplicateTool: tool names in this set bypass dedup (e.g. "create_card")
	allowDuplicateTool map[string]bool

	// executeTool runs a single tool call. Returns (result, action, pinSuggestion).
	// Project chat returns nil for pinSuggestion.
	executeTool func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion)

	// stageTool is non-nil only for card chat in suggest mode.
	stageTool func(tc llm.ToolCall) (string, []model.PendingEdit)

	// afterToolsRun is called after each iteration's tool calls (e.g. reload card, rebuild tools, nudge).
	// It receives the tool calls from the current iteration and the accumulated state,
	// and returns updated tools + optional system nudge messages to inject.
	afterToolsRun func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message)

	// suggest mode: if true, executeTool is ignored and stageTool is used instead.
	suggestMode bool

	// minConfidence filters pin suggestions below this threshold.
	minConfidence string

	// fallbackContent is the assistant message if max iterations are reached.
	fallbackContent string

	// tokenBudget is the maximum total tokens allowed across all iterations (0 = unlimited).
	tokenBudget int
	// totalTokensUsed is written back with the cumulative token count after the loop finishes.
	totalTokensUsed *int
}

// runChatLoop runs the shared LLM tool-calling loop for both card and project chat.
func (a *App) runChatLoop(ctx context.Context, provider llm.Provider, modelName string, cf *model.ChatFile, lc chatLoopConfig) (*model.ChatFile, error) {
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
	toolDefs := lc.tools

	for iteration := 0; iteration < lc.maxIter; iteration++ {
		resp, err := provider.ChatCompletion(ctx, llm.ChatRequest{
			SystemPrompt: lc.systemPrompt,
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
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, errMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
			}
			return cf, nil
		}

		// Accumulate token usage
		if resp.Usage != nil {
			cumulativeTokens += resp.Usage.TotalTokens
		}

		// Check token budget
		if lc.tokenBudget > 0 && cumulativeTokens > lc.tokenBudget {
			budgetMsg := model.ChatMessage{
				ID:        uuid.New().String(),
				Role:      model.RoleSystem,
				Content:   fmt.Sprintf("Token budget exceeded (%d / %d). Stopping.", cumulativeTokens, lc.tokenBudget),
				Timestamp: time.Now().UTC(),
			}
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, budgetMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
			}
			return cf, fmt.Errorf("token budget exceeded (%d / %d)", cumulativeTokens, lc.tokenBudget)
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
			cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, assistantMsg)
			if lc.totalTokensUsed != nil {
				*lc.totalTokensUsed = cumulativeTokens
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
			if seenCalls[tc.Name] && !lc.allowDuplicateTool[tc.Name] {
				llmMessages = append(llmMessages, llm.Message{
					Role:       "tool",
					Content:    "Skipped — duplicate call",
					ToolCallID: tc.ID,
				})
				continue
			}
			seenCalls[tc.Name] = true

			var result string
			if lc.suggestMode && lc.stageTool != nil {
				var edits []model.PendingEdit
				result, edits = lc.stageTool(tc)
				allPendingEdits = append(allPendingEdits, edits...)
			} else {
				var action *model.ToolAction
				var ps *model.PinSuggestion
				result, action, ps = lc.executeTool(tc)
				if action != nil {
					allToolActions = append(allToolActions, *action)
				}
				if ps != nil && config.ConfidenceMeetsThreshold(ps.Confidence, lc.minConfidence) {
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
		if lc.afterToolsRun != nil {
			var nudges []llm.Message
			toolDefs, nudges = lc.afterToolsRun(resp.ToolCalls, toolDefs, pinSuggestion, allPendingEdits)
			llmMessages = append(llmMessages, nudges...)
		}
	}

	// Max iterations reached — save what we have
	assistantMsg := model.ChatMessage{
		ID:            uuid.New().String(),
		Role:          model.RoleAssistant,
		Content:       lc.fallbackContent,
		Timestamp:     time.Now().UTC(),
		ToolActions:   allToolActions,
		PinSuggestion: pinSuggestion,
		PendingEdits:  allPendingEdits,
	}
	cf, _ = config.AppendChatMessage(a.repo.Manifest.ID,lc.chatID, assistantMsg)
	if lc.totalTokensUsed != nil {
		*lc.totalTokensUsed = cumulativeTokens
	}
	return cf, nil
}

// --- Project chat ---

func (a *App) SendProjectChatMessage(brandSlug, streamSlug, projectSlug, userMessage, contextLevel string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := projectChatID(project.ID)

	// Save user message
	cf, err := a.saveUserMessage(chatID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := a.loadLLMProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Build system prompt with project context
	categories, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	brand, _ := a.repo.GetBrand(brandSlug)
	stream, _ := a.repo.GetStream(brandSlug, streamSlug)
	systemPrompt := a.buildProjectSystemPrompt(brandSlug, streamSlug, projectSlug, brand, stream, project, categories, cfg, model.ProjectChatContextLevel(contextLevel))

	// Build tool definitions
	var catMaps []map[string]string
	for _, cat := range categories {
		catMaps = append(catMaps, map[string]string{"id": cat.ID, "breadcrumb": cat.Name})
	}
	cardTypes := a.listCardTypeIDs()
	toolDefs := llm.ProjectTools(cardTypes, catMaps)

	// Build the per-call scope for tool callbacks. Both staging and execute
	// callbacks use the cardIDs set to reject IDs the LLM hallucinated or
	// copied from a different project; the slugs let project-metadata tools
	// (update_project, *_label, *_category) target the right project.
	scope := projectChatScope{
		brandSlug:   brandSlug,
		streamSlug:  streamSlug,
		projectSlug: projectSlug,
		cardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			scope.cardIDs[p.CardID] = true
		}
	}

	modelName := cfg.Model
	if modelName == "" {
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	suggestMode := cfg.AIMode == "suggest"
	return a.runChatLoop(ctx, provider, modelName, cf, chatLoopConfig{
		chatID:             chatID,
		systemPrompt:       systemPrompt,
		tools:              toolDefs,
		maxIter:            5,
		// Per-entity mutating tools must be callable multiple times in one
		// iteration so the LLM can act on several distinct targets in a single
		// turn (e.g. set an icon on every category, move several cards). The
		// bulk variant `update_cards` exists for the most common case but the
		// LLM doesn't always reach for it. Query/read tools are deliberately
		// NOT whitelisted — those should be deduped to stop runaway loops.
		allowDuplicateTool: map[string]bool{
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
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			result, action := a.executeProjectToolCall(tc, scope)
			return result, action, nil
		},
		stageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return a.stageProjectToolCall(tc, scope)
		},
		suggestMode:     suggestMode,
		fallbackContent: "I've made the requested changes to your project.",
	})
}

// buildProjectSystemPrompt builds the system prompt for project-level chat.
// Slugs are passed so the prompt can fetch project-scoped data (labels) and
// reference the project unambiguously to the LLM.
func (a *App) buildProjectSystemPrompt(brandSlug, streamSlug, projectSlug string, brand *model.Brand, stream *model.Stream, project *model.Project, categories []model.Category, cfg config.LLMConfig, level model.ProjectChatContextLevel) string {
	// Default to full context if empty/unrecognised.
	if level != model.ProjectChatContextMetadata && level != model.ProjectChatContextNone {
		level = model.ProjectChatContextAll
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are BRUV AI, a project assistant. Today is %s.\n\n", time.Now().Format("2006-01-02 (Monday)")))

	// Hierarchy context (always included — this is small and gives the LLM its bearings)
	if a.repo != nil && a.repo.Manifest.Description != "" {
		sb.WriteString(fmt.Sprintf("## Repository: %s\n%s\n\n", a.repo.Manifest.Name, a.repo.Manifest.Description))
	}
	if brand != nil {
		sb.WriteString(fmt.Sprintf("## Brand: %s\n", brand.Name))
		if brand.Description != "" {
			sb.WriteString(brand.Description + "\n")
		}
		if brand.SystemPrompt != "" {
			sb.WriteString("\nBrand instructions:\n" + brand.SystemPrompt + "\n")
		}
		sb.WriteString("\n")
	}
	if stream != nil {
		sb.WriteString(fmt.Sprintf("## Stream: %s\n", stream.Name))
		if stream.Description != "" {
			sb.WriteString(stream.Description + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("## Project: %s\n", project.Name))
	if project.Description != "" {
		sb.WriteString(project.Description + "\n")
	}
	if project.Icon != "" {
		sb.WriteString(fmt.Sprintf("Icon: `%s`\n", project.Icon))
	}
	sb.WriteString(fmt.Sprintf("project_id: `%s`\n", project.ID))
	sb.WriteString("\n")

	if cfg.Context != "" {
		sb.WriteString("## User context\n" + cfg.Context + "\n\n")
	}

	// Project tag vocabulary. Each line includes the tag's name, current
	// color, optional icon, ID, and a usage count computed by walking
	// ListCardIDsByTag — this is what lets the LLM answer "find unused tags"
	// without needing a dedicated tool. (The underlying Go type is named
	// `model.Label` for historical reasons; user-facing terminology is "tag".)
	if a.repo != nil {
		labels, _ := a.repo.GetProjectLabels(brandSlug, streamSlug, projectSlug)
		if len(labels) > 0 {
			sb.WriteString("## Project tags (the tag vocabulary)\n")
			sb.WriteString("These are the tags defined for this project. Each line shows usage count.\n")
			for _, l := range labels {
				count := 0
				if ids, err := a.ListCardIDsByTag(l.Name); err == nil {
					count = len(ids)
				}
				line := fmt.Sprintf("- `%s` (id: `%s`, color: %s", l.Name, l.ID, l.Color)
				if l.Icon != "" {
					line += ", icon: `" + l.Icon + "`"
				}
				line += fmt.Sprintf(", used by %d card", count)
				if count != 1 {
					line += "s"
				}
				if count == 0 {
					line += " — UNUSED"
				}
				line += ")"
				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Card enumeration is gated by the context level.
	switch level {
	case model.ProjectChatContextNone:
		sb.WriteString("_Card details are hidden for this conversation. Use tools to query cards if needed._\n\n")

	case model.ProjectChatContextMetadata:
		// Titles + types only — no tags, descriptions, or due dates.
		totalCards := 0
		seenCards := make(map[string]bool)
		if len(categories) > 0 {
			sb.WriteString("## Categories and cards (titles only)\n\n")
			for _, cat := range categories {
				sb.WriteString(renderCategoryHeader(cat))
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := a.repo.GetCard(pin.CardID)
					if err != nil {
						continue
					}
					totalCards++
					sb.WriteString(fmt.Sprintf("- **%s** (id: `%s`, type: %s)\n", card.Title, card.ID, card.Type))
				}
				sb.WriteString("\n")
			}
			if totalCards == 0 {
				sb.WriteString("All categories are empty — no cards yet.\n\n")
			}
		} else {
			sb.WriteString("This project has no categories yet.\n\n")
		}

	case model.ProjectChatContextAll:
		fallthrough
	default:
		// Full enumeration: titles, types, tags, due dates, content snippet, agent config.
		totalCards := 0
		seenCards := make(map[string]bool)
		if len(categories) > 0 {
			sb.WriteString("## Categories and cards\n\n")
			for _, cat := range categories {
				sb.WriteString(renderCategoryHeader(cat))
				pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := a.repo.GetCard(pin.CardID)
					if err != nil {
						continue
					}
					totalCards++
					sb.WriteString(serializeCardForProjectPrompt(a, card))
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("This project has no categories yet.\n\n")
		}
		if totalCards == 0 && len(categories) > 0 {
			sb.WriteString("All categories are empty — no cards yet.\n\n")
		}
	}

	sb.WriteString("## Your capabilities\n")
	sb.WriteString("You can read and modify any property of this project, its cards, its categories, and its tag vocabulary. ")
	sb.WriteString("USE THE TOOLS to make changes — do not just describe what would be done.\n\n")
	sb.WriteString("Cards:\n")
	sb.WriteString("- `create_card` — create a new card and optionally pin it to a category.\n")
	sb.WriteString("- `update_card` / `update_cards` — change title, type, tags, due date, description, blocks. Prefer the plural for bulk edits.\n")
	sb.WriteString("- For tags: use `tags_to_add` to append, `tags_to_remove` to drop specific tags, and `tags` only when the user explicitly wants to replace the whole tag list.\n")
	sb.WriteString("- `add_tags_to_cards` — bulk-tag many cards in one call.\n")
	sb.WriteString("- `move_card` — move a card between categories.\n")
	sb.WriteString("- `configure_agent` — set a card's agent schedule, goal, enabled state, or tool whitelist.\n\n")
	sb.WriteString("Web:\n")
	sb.WriteString("- `web_fetch` — fetch a URL and read its text. Use for known links.\n")
	sb.WriteString("- `web_search` — search the web via DuckDuckGo. Use for open-ended lookups; follow up with `web_fetch` on the best result if you need full content.\n")
	sb.WriteString("- YOU HAVE WEB ACCESS. If the user asks about current events, prices, news, or anything external to the project, CALL `web_search` — do NOT tell the user to look it up themselves. Cite source URLs in your reply.\n\n")
	sb.WriteString("Project metadata:\n")
	sb.WriteString("- `update_project` — change the project's name, description, or icon.\n\n")
	sb.WriteString("Project tags (the tag vocabulary, listed above):\n")
	sb.WriteString("- `create_project_tag` — define a new tag.\n")
	sb.WriteString("- `update_project_tag` — rename, recolor, or set an icon for an existing tag. Identify by `tag_id` or `tag_name`.\n")
	sb.WriteString("- `delete_project_tag` — remove a tag from the project's vocabulary. The tag list above shows usage counts; tags marked UNUSED can usually be deleted directly.\n\n")
	sb.WriteString("Categories (columns):\n")
	sb.WriteString("- `create_category` / `update_category` / `delete_category` — manage the columns. update_category can set name, description, icon, and accepted_types.\n")
	sb.WriteString("- `update_category` and `delete_category` accept either `category_id` (preferred) or `category_name`. Use the name when referring to a category you just created in the same conversation, since its ID won't be known yet.\n")
	sb.WriteString("- When chaining `create_category` with `move_card` or `create_card` in the same turn, use the destination's NAME (`to_category_name` / `category_name`) — the apply phase resolves the name after the create runs.\n")
	sb.WriteString("- `move_card` only requires `card_id` and the destination. The source (`from_category_id`) is auto-detected from the card's current pin in this project, so you usually don't need to supply it.\n\n")
	sb.WriteString("Icon names (for `icon` parameters on `update_project`, `update_category`, `create_project_tag`, `update_project_tag`):\n")
	sb.WriteString("Use ONLY these names — anything else will display as a placeholder. Names use kebab-case.\n")
	sb.WriteString(availableIconList())
	sb.WriteString("\n\n")
	sb.WriteString("When creating cards, always pin them to the most appropriate category.\n")
	return sb.String()
}

// availableIconList returns the icon name list embedded in the system prompt.
// MUST be kept in sync with `ICON_MAP` in `frontend/src/lib/icons.ts`. The
// frontend's DynamicIcon component renders an unknown-icon placeholder for
// any name not in that map, so picking one from outside this list silently
// breaks the display.
//
// When you add an icon to ICON_MAP, add it here too. (And vice versa.)
func availableIconList() string {
	icons := []string{
		// General
		"folder", "folder-open", "star", "heart", "zap", "rocket", "globe", "home", "flag", "bookmark",
		"tag", "tags", "lightbulb", "puzzle", "circle-dot", "crown", "trophy", "diamond", "gem", "sparkles",
		// People & reactions
		"user", "users", "hand-metal", "thumbs-up", "thumbs-down", "smile", "frown", "meh", "angry", "laugh",
		// Communication
		"bell", "mail", "message-square", "message-circle", "megaphone", "phone", "smartphone", "send", "inbox",
		// Media & entertainment
		"image", "video", "music", "music-2", "camera", "mic", "tv", "tv-2", "radio", "podcast", "headphones",
		"gamepad2", "film", "clapperboard", "popcorn", "drama", "newspaper",
		"disc", "monitor-play", "play-circle", "pause-circle", "stop-circle", "volume-2", "volume-x",
		// Files & writing
		"file-text", "file", "book", "book-open", "library", "pen", "pen-tool", "brush", "scissors", "palette",
		"edit", "copy", "save", "archive", "box", "package", "boxes",
		// Dev
		"code", "terminal", "database", "server", "cloud", "hash", "at-sign", "binary", "github", "gitlab", "layers",
		// Business / money
		"briefcase", "building", "shopping-cart", "dollar-sign", "credit-card", "bar-chart", "pie-chart",
		"award", "target", "crosshair", "circle-dollar-sign", "wallet", "piggy-bank", "banknote", "receipt",
		"calculator", "medal",
		// Time
		"calendar", "calendar-days", "calendar-clock", "calendar-check", "clock", "timer", "alarm-clock", "hourglass", "history",
		// Nature & weather
		"sun", "moon", "flame", "leaf", "tree-pine", "tree-deciduous", "mountain", "cloud-rain", "cloud-snow",
		"cloud-lightning", "snowflake", "wind", "rainbow", "sunrise", "sunset",
		// Animals
		"dog", "cat", "bird", "fish", "rabbit", "squirrel", "bug", "turtle",
		// Food & drink
		"coffee", "pizza", "utensils", "wine", "apple", "cookie", "ice-cream", "soup", "beer",
		// Transport
		"plane", "car", "bus", "train", "bike", "ship", "truck", "fuel",
		// Science & education
		"microscope", "atom", "flask-conical", "beaker", "dna", "brain", "brain-circuit", "graduation-cap", "school",
		// Health & fitness
		"activity", "heart-pulse", "stethoscope", "pill", "syringe", "bandage", "dumbbell",
		// Tools
		"wrench", "hammer", "drill", "pickaxe", "ruler", "hardhat", "plug",
		// Navigation
		"map", "map-pin", "compass", "navigation", "arrow-right", "arrow-up", "link", "external-link",
		// Security
		"lock", "unlock", "key", "shield", "shield-check", "shield-alert", "eye",
		// Devices
		"monitor", "laptop", "settings",
		// Status & alerts
		"alert-circle", "check-circle", "alert-triangle", "info", "help-circle", "badge-check", "badge-alert",
		// Lists & filter
		"list-checks", "list-todo", "list-filter", "filter",
		// Search
		"search", "zoom-in", "zoom-out",
		// Social
		"twitter", "youtube", "twitch", "linkedin", "facebook", "instagram", "slack",
		// Shapes
		"circle", "triangle", "octagon", "hexagon", "pentagon",
		// Editing
		"refresh-cw", "check", "plus", "minus",
		// Fun
		"gift", "party-popper", "percent",
	}
	return strings.Join(icons, ", ")
}

// renderCategoryHeader produces the markdown header line(s) for a category in
// the project chat system prompt. Includes name + ID, optional description,
// optional icon, and accepted_types restriction (if any).
func renderCategoryHeader(cat model.Category) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s (category_id: %s)\n", cat.Name, cat.ID))
	if cat.Icon != "" {
		sb.WriteString(fmt.Sprintf("Icon: `%s`\n", cat.Icon))
	}
	if cat.Description != "" {
		sb.WriteString(cat.Description + "\n")
	}
	if len(cat.AcceptedTypes) > 0 {
		sb.WriteString("Accepted card types: " + strings.Join(cat.AcceptedTypes, ", ") + "\n")
	}
	return sb.String()
}

// serializeCardForProjectPrompt renders one card into the markdown form used by
// the project chat system prompt. Includes title, id, type, tags, due date,
// description / block content snippets, and agent config when present.
//
// Total content per card is bounded so a project with hundreds of cards
// doesn't blow the context window:
//   - Description / block snippets are capped at ~500 chars combined.
//   - Agent goal is capped at ~200 chars.
//   - Each text/markdown block is included in order, separated by spaces.
func serializeCardForProjectPrompt(a *App, card *model.Card) string {
	const maxContentChars = 500
	const maxAgentGoalChars = 200

	var sb strings.Builder
	line := fmt.Sprintf("- **%s** (id: `%s`, type: %s", card.Title, card.ID, card.Type)
	if len(card.Tags) > 0 {
		line += ", tags: " + strings.Join(card.Tags, ", ")
	}
	if card.DueDate != nil {
		line += ", due: " + card.DueDate.Format("2006-01-02")
	}
	line += ")"
	sb.WriteString(line + "\n")

	// Aggregate text content from description field + every text/markdown block,
	// in document order, capped at maxContentChars overall.
	var content strings.Builder
	if desc, ok := card.Fields["description"].(string); ok && desc != "" {
		content.WriteString(desc)
	}
	for _, b := range card.Blocks {
		if b.Type != "text" && b.Type != "markdown" {
			continue
		}
		s, ok := b.Value.(string)
		if !ok || s == "" {
			continue
		}
		if content.Len() > 0 {
			content.WriteString(" \n")
		}
		content.WriteString(s)
		if content.Len() >= maxContentChars {
			break
		}
	}
	if content.Len() > 0 {
		snippet := content.String()
		if len(snippet) > maxContentChars {
			snippet = snippet[:maxContentChars] + "…"
		}
		sb.WriteString("  > " + strings.ReplaceAll(snippet, "\n", "\n  > ") + "\n")
	}

	// Agent config — only show if there's anything meaningful configured.
	if a.repo != nil {
		af, err := a.repo.GetAgentConfig(card.ID)
		if err == nil && af != nil {
			cfg := af.Config
			hasConfig := cfg.Enabled || cfg.Schedule != "" || cfg.Goal != "" || cfg.Status != "" || cfg.LastRunAt != nil || cfg.NextRunAt != nil
			if hasConfig {
				sb.WriteString("  agent:")
				sb.WriteString(fmt.Sprintf(" enabled=%t", cfg.Enabled))
				if cfg.Schedule != "" {
					sb.WriteString(", schedule=`" + cfg.Schedule + "`")
				}
				if cfg.Status != "" {
					sb.WriteString(", status=" + string(cfg.Status))
				}
				if cfg.LastRunAt != nil {
					sb.WriteString(", last_run=" + cfg.LastRunAt.Format("2006-01-02 15:04"))
				}
				if cfg.NextRunAt != nil {
					sb.WriteString(", next_run=" + cfg.NextRunAt.Format("2006-01-02 15:04"))
				}
				sb.WriteString("\n")
				if cfg.Goal != "" {
					goal := cfg.Goal
					if len(goal) > maxAgentGoalChars {
						goal = goal[:maxAgentGoalChars] + "…"
					}
					sb.WriteString("  agent goal: " + strings.ReplaceAll(goal, "\n", " ") + "\n")
				}
				// Surface the most recent failed run's error so the LLM can help debug.
				if len(af.Runs) > 0 {
					last := af.Runs[len(af.Runs)-1]
					if last.Status == "failed" && last.Error != "" {
						errMsg := last.Error
						if len(errMsg) > 200 {
							errMsg = errMsg[:200] + "…"
						}
						sb.WriteString("  agent last error: " + strings.ReplaceAll(errMsg, "\n", " ") + "\n")
					}
				}
			}
		}
	}
	return sb.String()
}

// --- Card chat ---

func (a *App) SendChatMessage(cardID, userMessage string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	// Save user message
	cf, err := a.saveUserMessage(cardID, userMessage)
	if err != nil {
		return nil, err
	}

	// Load LLM config + provider
	cfg, provider, err := a.loadLLMProvider()
	if err != nil || provider == nil {
		return cf, nil
	}

	// Attribute all card edits in this chat turn to the LLM model
	a.llmActors.Store(cardID, cfg.Model)
	defer a.llmActors.Delete(cardID)

	// Load card for context
	card, err := a.repo.GetCard(cardID)
	if err != nil {
		return cf, nil
	}

	systemPrompt := a.buildSystemPrompt(card, cfg)

	// Build tool definitions
	cardTypes := a.listCardTypeIDs()
	allCats, _ := a.ListAllCategories()
	var catMaps []map[string]string
	for _, c := range allCats {
		catMaps = append(catMaps, map[string]string{"id": c.CategoryID, "breadcrumb": c.Breadcrumb})
	}

	buildToolDefs := func(c *model.Card) []llm.ToolDef {
		// Collect MCP tool IDs so configure_agent's allowed_tools enum
		// includes them — lets the LLM add MCP tools to agents via chat.
		var mcpToolIDs []string
		if a.mcpRegistry != nil {
			for _, t := range a.mcpRegistry.Tools() {
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
		modelName = defaultModelForProvider(cfg.Provider)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	defer cancel()

	return a.runChatLoop(ctx, provider, modelName, cf, chatLoopConfig{
		chatID:       cardID,
		systemPrompt: systemPrompt,
		tools:        toolDefs,
		// 6 iterations comfortably covers web_search → web_fetch →
		// summarise, or a couple of card-tool rounds plus a final
		// message. Previously 3, which was too tight for research
		// flows — the loop would exhaust before the model got to
		// speak, triggering the fallbackContent lie below.
		maxIter: 6,
		suggestMode:  cfg.AIMode == "suggest",
		stageTool: func(tc llm.ToolCall) (string, []model.PendingEdit) {
			return a.stageToolCall(tc, allCats)
		},
		executeTool: func(tc llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			return a.executeToolCall(cardID, card, tc, allCats)
		},
		minConfidence: cfg.MinConfidence,
		afterToolsRun: func(calls []llm.ToolCall, tools []llm.ToolDef, pin *model.PinSuggestion, edits []model.PendingEdit) ([]llm.ToolDef, []llm.Message) {
			var nudges []llm.Message

			// Reload card after tool execution (it may have changed)
			if cfg.AIMode != "suggest" {
				card, _ = a.repo.GetCard(cardID)
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
				existingPins, _ := a.repo.GetCardPins(cardID)
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
		fallbackContent: "I hit my tool-call limit before I could write a reply. The tools above show what ran — ask again or narrow the request if you'd like a summary.",
	})
}

// --- Chat helpers ---

// saveUserMessage saves a user message to a chat and returns the updated ChatFile.
func (a *App) saveUserMessage(chatID, userMessage string) (*model.ChatFile, error) {
	userMsg := model.ChatMessage{
		ID:        uuid.New().String(),
		Role:      model.RoleUser,
		Content:   repo.SanitizeText(userMessage),
		Timestamp: time.Now().UTC(),
	}
	return config.AppendChatMessage(a.repo.Manifest.ID,chatID, userMsg)
}
// projectChatScope bundles the per-call context that project chat tool
// callbacks need: which project we're scoped to, plus the set of card IDs that
// legitimately belong to it. The struct lets executeProjectToolCall and
// stageProjectToolCall share a single parameter without growing a long
// argument list each time we add a tool that needs project context.
type projectChatScope struct {
	brandSlug, streamSlug, projectSlug string
	cardIDs                            map[string]bool
}
func (a *App) buildSystemPrompt(card *model.Card, cfg config.LLMConfig) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(`You are BRUV AI. You help the user both ORGANISE this card (edit title, tags, fields, pin location, agent config) AND RESEARCH anything relevant to it (look up live information on the web). Today is %s.

FIRST, decide which mode the user's message is in:
  - ORGANISE: they're describing the card's content or asking you to edit it ("this is about X", "add Y", "set due date…", "tag it Z"). In ORGANISE mode, call all applicable card tools at once: type, title, fields, tags, AND pin.
  - RESEARCH: they're asking about something external — current events, prices, news, looking up a URL, explaining why something happened. In RESEARCH mode, call web_search and/or web_fetch, then reply with the answer. Do NOT also call card-organising tools unless the user explicitly asks you to save the findings to the card.
  - CHAT: they're asking a general question that needs no tool. Just reply.
When in doubt between ORGANISE and RESEARCH, prefer RESEARCH — it's less disruptive to call the wrong web tool than to rewrite the card's title/tags.

RULES:
- NEVER call a tool if the value is already correct (check current card state below).
- NEVER call the same tool twice in one response.
- After using tools, briefly describe what you changed or what you found.
- When researching, ALWAYS cite the source URLs returned by web_search in your final reply.
- YOU HAVE WEB ACCESS via web_search and web_fetch. If the user asks about anything you don't already know, CALL those tools. Do NOT say "I can't search the web" or "use a search engine yourself" — those responses are wrong.

TOOLS:
- set_card_type — Pick the best type. Only if type is not set or wrong.
- set_fields — Fill field values with real content from the user's message. ALWAYS call this when fields are empty.
- set_title — Write a clear, specific title. Only if title is "New Card" or generic.
- set_due_date — YYYY-MM-DD format. Resolve relative dates from today (%s).
- suggest_pin — ALWAYS pin the card. STRONGLY prefer an existing category_id from the list below. The hierarchy is: Brand > Stream > Project > Category (e.g. "Big Ideas / YouTube Channels / Channel Brainstorm / Ideas"). Do NOT use the card title as a brand name. Only create new names if NOTHING existing fits.
- add_tags — Add relevant tags. Prefer existing project tags listed below, but you may create new short, descriptive tags if none fit.
- add_field — Add a NEW field to the card (e.g. a checklist, extra notes, a checkbox). Use when the user asks for a field that does not already exist. ALWAYS pass the 'value' parameter in the same call when the user described what should go in the field — do NOT split into add_field followed by set_fields, that pattern frequently leaves the field empty. Only use set_fields afterward to update an EXISTING field.
- configure_agent — Set up or modify the card's autonomous agent. Provide enabled, goal, schedule, and allowed_tools. The agent runs in the background and can fetch web pages, search, notify the user, and update this card. Use this when the user asks to "set up an agent", "run this on a schedule", "check daily", etc.
- web_fetch — Fetch a specific URL and read its text content. Use when the user gives you a link or when you need up-to-date info from a known page.
- web_search — Search the web via DuckDuckGo. Use for "look up…", "find the latest…", "what's happening with…" style asks. Returns titles, URLs, and snippets; follow up with web_fetch on the most relevant result if you need the full content.`, time.Now().Format("2006-01-02 (Monday)"), time.Now().Format("2006-01-02")))

	if cfg.Context != "" {
		parts = append(parts, "User context:\n"+cfg.Context)
	}

	// Available card types with their schemas
	if a.registry != nil {
		typeNames := a.registry.List()
		if len(typeNames) > 0 {
			var typeDescs []string
			for _, tn := range typeNames {
				s := a.registry.Get(tn)
				if s == nil {
					continue
				}
				desc := fmt.Sprintf("- %s: %s", tn, s.Description)
				var fields []string
				for key, prop := range s.Properties {
					f := key
					if prop.Description != "" {
						f += " (" + prop.Description + ")"
					}
					if len(prop.Enum) > 0 {
						f += " [" + strings.Join(prop.Enum, "/") + "]"
					}
					fields = append(fields, f)
				}
				if len(fields) > 0 {
					desc += "\n  Fields: " + strings.Join(fields, ", ")
				}
				typeDescs = append(typeDescs, desc)
			}
			parts = append(parts, "Available card types:\n"+strings.Join(typeDescs, "\n"))
		}
	}

	// Card context (always included)
	var cardParts []string
	cardParts = append(cardParts, fmt.Sprintf("Current card: %q", card.Title))
	if card.Type != "" {
		cardParts = append(cardParts, fmt.Sprintf("Type: %s", card.Type))
	} else {
		cardParts = append(cardParts, "Type: (not set)")
	}
	if len(card.Tags) > 0 {
		cardParts = append(cardParts, fmt.Sprintf("Tags: %s", strings.Join(card.Tags, ", ")))
	}
	if card.DueDate != nil {
		cardParts = append(cardParts, fmt.Sprintf("Due: %s", card.DueDate.Format("2006-01-02")))
	}
	// Include ALL fields — show empty ones so the LLM knows to fill them
	var emptyFields []string
	for _, b := range card.Blocks {
		label := b.Label
		if label == "" {
			label = b.Key
		}
		if label == "" {
			continue
		}
		if v, ok := b.Value.(string); ok && v != "" {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: %s", b.Key, v))
		} else if v, ok := b.Value.(string); ok && v == "" {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: (EMPTY — needs content)", b.Key))
			emptyFields = append(emptyFields, b.Key)
		} else if b.Value != nil {
			cardParts = append(cardParts, fmt.Sprintf("Field [%s]: %v", b.Key, b.Value))
		}
	}
	if len(emptyFields) > 0 {
		cardParts = append(cardParts, fmt.Sprintf(">>> EMPTY FIELDS that need content: %s — use set_fields to fill them!", strings.Join(emptyFields, ", ")))
	}
	parts = append(parts, strings.Join(cardParts, "\n"))

	// Agent context — show current agent config if it exists
	if a.repo != nil {
		af, err := a.repo.GetAgentConfig(card.ID)
		if err == nil && af.Config.Enabled {
			agentParts := []string{"Agent: ENABLED"}
			agentParts = append(agentParts, fmt.Sprintf("  Goal: %s", af.Config.Goal))
			agentParts = append(agentParts, fmt.Sprintf("  Schedule: %s", af.Config.Schedule))
			agentParts = append(agentParts, fmt.Sprintf("  Status: %s", af.Config.Status))
			agentParts = append(agentParts, fmt.Sprintf("  Tools: %s", strings.Join(af.Config.AllowedTools, ", ")))
			if af.Config.LastRunAt != nil {
				agentParts = append(agentParts, fmt.Sprintf("  Last run: %s", af.Config.LastRunAt.Format("2006-01-02 15:04")))
			}
			parts = append(parts, strings.Join(agentParts, "\n"))
		} else {
			parts = append(parts, "Agent: not configured. Use configure_agent to set up an autonomous agent on this card.")
		}
	}

	// Hierarchy context based on ContextLevel
	level := card.ContextLevel
	if level == "" {
		level = model.ContextProject
	}
	if level == model.ContextIsolated || a.repo == nil {
		return strings.Join(parts, "\n\n")
	}

	// Repository description
	if a.repo.Manifest.Description != "" {
		parts = append(parts, "Repository: "+a.repo.Manifest.Name+"\n"+a.repo.Manifest.Description)
	}

	// Build hierarchy descriptions (deduplicated — each entity listed once)
	allCats, _ := a.ListAllCategories()
	if len(allCats) > 0 {
		seenBrands := map[string]bool{}
		seenStreams := map[string]bool{}
		seenProjects := map[string]bool{}
		var hierarchy []string
		for _, c := range allCats {
			if !seenBrands[c.BrandName] {
				seenBrands[c.BrandName] = true
				if c.BrandDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("Brand %q — %s", c.BrandName, c.BrandDescription))
				}
			}
			streamKey := c.BrandName + "/" + c.StreamName
			if !seenStreams[streamKey] {
				seenStreams[streamKey] = true
				if c.StreamDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("  Stream %q — %s", c.StreamName, c.StreamDescription))
				}
			}
			projectKey := streamKey + "/" + c.ProjectName
			if !seenProjects[projectKey] {
				seenProjects[projectKey] = true
				if c.ProjectDescription != "" {
					hierarchy = append(hierarchy, fmt.Sprintf("    Project %q — %s", c.ProjectName, c.ProjectDescription))
				}
			}
			if c.CategoryDescription != "" {
				hierarchy = append(hierarchy, fmt.Sprintf("      Category %q — %s", c.CategoryName, c.CategoryDescription))
			}
		}
		if len(hierarchy) > 0 {
			parts = append(parts, "Hierarchy descriptions:\n"+strings.Join(hierarchy, "\n"))
		}

		// Category listing for pin suggestions (compact — no repeated descriptions)
		var catDescs []string
		for _, c := range allCats {
			desc := fmt.Sprintf("- %s (id: %s)", c.Breadcrumb, c.CategoryID)
			if len(c.AcceptedTypes) > 0 {
				desc += " [accepts: " + strings.Join(c.AcceptedTypes, ", ") + "]"
			}
			catDescs = append(catDescs, desc)
		}
		parts = append(parts, "Available categories for pinning (PREFER these — only create new if none fit):\n"+strings.Join(catDescs, "\n"))
	} else {
		parts = append(parts, "No categories exist yet. Use suggest_pin with brand/stream/project/category names to create a new location.")
	}

	// Collect existing project tags so the AI prefers them over inventing new ones
	projectTags := a.collectProjectTags(allCats)
	if len(projectTags) > 0 {
		parts = append(parts, "Existing tags (prefer these, or create short new ones if none fit):\n"+strings.Join(projectTags, "\n"))
	}

	// Brand instructions for pinned cards
	pins, _ := a.repo.GetCardPins(card.ID)
	if len(pins) > 0 && (level == model.ContextBrand || level == model.ContextGlobal) {
		loc, err := a.GetCardLocation(card.ID)
		if err == nil {
			brand, err := a.repo.GetBrand(loc.BrandSlug)
			if err == nil && brand.SystemPrompt != "" {
				parts = append(parts, "Brand instructions:\n"+brand.SystemPrompt)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// collectProjectTags gathers all unique tags from all projects, grouped by project.
func (a *App) collectProjectTags(allCats []CategoryPath) []string {
	if a.repo == nil {
		return nil
	}
	// Deduplicate projects (multiple categories share the same project)
	type projectKey struct{ brand, stream, project string }
	seen := make(map[projectKey]bool)
	var results []string

	for _, c := range allCats {
		pk := projectKey{c.BrandSlug, c.StreamSlug, c.ProjectSlug}
		if seen[pk] {
			continue
		}
		seen[pk] = true
		labels, err := a.repo.GetProjectLabels(c.BrandSlug, c.StreamSlug, c.ProjectSlug)
		if err != nil || len(labels) == 0 {
			continue
		}
		var tagNames []string
		for _, l := range labels {
			tagNames = append(tagNames, l.Name)
		}
		results = append(results, fmt.Sprintf("- %s / %s / %s: %s", c.BrandName, c.StreamName, c.ProjectName, strings.Join(tagNames, ", ")))
	}
	return results
}


