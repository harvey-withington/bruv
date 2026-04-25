package prompts

import (
	"bruv/core/runtime/promptfmt"
	"bruv/internal/config"
	"bruv/internal/model"
	"fmt"
	"runtime"
	"strings"
	"time"
)

func (b *Builder) Project(brandSlug, streamSlug, projectSlug string, brand *model.Brand, stream *model.Stream, project *model.Project, categories []model.Category, cfg config.LLMConfig, level model.ProjectChatContextLevel) string {
	// Default to full context if empty/unrecognised.
	if level != model.ProjectChatContextMetadata && level != model.ProjectChatContextNone {
		level = model.ProjectChatContextAll
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are BRUV AI, a project assistant. Today is %s.\n\n", time.Now().Format("2006-01-02 (Monday)")))

	// Hierarchy context (always included — this is small and gives the LLM its bearings)
	if b.deps.Repo() != nil && b.deps.Repo().Manifest.Description != "" {
		sb.WriteString(fmt.Sprintf("## Repository: %s\n%s\n\n", b.deps.Repo().Manifest.Name, b.deps.Repo().Manifest.Description))
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
	if b.deps.Repo() != nil {
		labels, _ := b.deps.Repo().GetProjectLabels(brandSlug, streamSlug, projectSlug)
		if len(labels) > 0 {
			sb.WriteString("## Project tags (the tag vocabulary)\n")
			sb.WriteString("These are the tags defined for this project. Each line shows usage count.\n")
			for _, l := range labels {
				count := 0
				if ids, err := b.deps.Search().ListCardIDsByTag(l.Name); err == nil {
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
				sb.WriteString(promptfmt.RenderCategoryHeader(cat))
				pins, _ := b.deps.Repo().ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := b.deps.Repo().GetCard(pin.CardID)
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
				sb.WriteString(promptfmt.RenderCategoryHeader(cat))
				pins, _ := b.deps.Repo().ListCardsInCategory(cat.ID, cat.ID)
				if len(pins) == 0 {
					sb.WriteString("  (empty)\n\n")
					continue
				}
				for _, pin := range pins {
					if seenCards[pin.CardID] {
						continue
					}
					seenCards[pin.CardID] = true
					card, err := b.deps.Repo().GetCard(pin.CardID)
					if err != nil {
						continue
					}
					totalCards++
					sb.WriteString(b.serializeCardForProjectPrompt(card))
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
	sb.WriteString(promptfmt.AvailableIconList())
	sb.WriteString("\n\n")
	sb.WriteString("When creating cards, always pin them to the most appropriate category.\n")
	return sb.String()
}

func (b *Builder) serializeCardForProjectPrompt( card *model.Card) string {
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
	if b.deps.Repo() != nil {
		af, err := b.deps.Repo().GetAgentConfig(card.ID)
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

func (b *Builder) Card(card *model.Card, cfg config.LLMConfig) string {
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
	if b.deps.Registry() != nil {
		typeNames := b.deps.Registry().List()
		if len(typeNames) > 0 {
			var typeDescs []string
			for _, tn := range typeNames {
				s := b.deps.Registry().Get(tn)
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
	if b.deps.Repo() != nil {
		af, err := b.deps.Repo().GetAgentConfig(card.ID)
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
	if level == model.ContextIsolated || b.deps.Repo() == nil {
		return strings.Join(parts, "\n\n")
	}

	// Repository description
	if b.deps.Repo().Manifest.Description != "" {
		parts = append(parts, "Repository: "+b.deps.Repo().Manifest.Name+"\n"+b.deps.Repo().Manifest.Description)
	}

	// Build hierarchy descriptions (deduplicated — each entity listed once)
	allCats, _ := b.deps.Card().ListAllCategories()
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
	projectTags := b.collectProjectTags(allCats)
	if len(projectTags) > 0 {
		parts = append(parts, "Existing tags (prefer these, or create short new ones if none fit):\n"+strings.Join(projectTags, "\n"))
	}

	// Brand instructions for pinned cards
	pins, _ := b.deps.Repo().GetCardPins(card.ID)
	if len(pins) > 0 && (level == model.ContextBrand || level == model.ContextGlobal) {
		loc, err := b.deps.Card().GetLocation(card.ID)
		if err == nil {
			brand, err := b.deps.Repo().GetBrand(loc.BrandSlug)
			if err == nil && brand.SystemPrompt != "" {
				parts = append(parts, "Brand instructions:\n"+brand.SystemPrompt)
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

func (b *Builder) collectProjectTags(allCats []CategoryPath) []string {
	if b.deps.Repo() == nil {
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
		labels, err := b.deps.Repo().GetProjectLabels(c.BrandSlug, c.StreamSlug, c.ProjectSlug)
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

func (b *Builder) Agent(card *model.Card, agentCfg model.AgentConfig, llmCfg config.LLMConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are BRUV Agent, an autonomous AI assistant running on a schedule. Today is %s.

## Your Goal

%s

## How to Read Your Goal

Your Goal is authoritative. Identify every deliverable it names — things
to check, save, update, notify, produce — and make sure each one has
been addressed with the right tool before you finish.

- If the Goal says "notify", "alert", "tell me", "let me know", or
  anything similar, you MUST call the `+"`notify`"+` tool. Describing the
  notification in your response is not enough; you have to actually
  call the tool.
- If the Goal says to change the card title, due date, or tags, use
  the top-level `+"`title`"+`, `+"`due_date`"+`, or `+"`tags`"+` parameters on
  `+"`update_self`"+`. Do NOT put these in the `+"`updates`"+` array — that
  only works for content blocks, not intrinsic card fields.
- If the Goal says to save, record, write, update, or populate a
  specific block on this card (e.g. "save to the X list", "update Y",
  "record findings in Z"), you MUST call `+"`update_self`"+` targeting
  that block by name or key. Check "Current Card State" below to find
  the exact block and its type; match the value format to the type per
  the `+"`update_self`"+` tool description.
- If a Goal references a block that does NOT exist in "Current Card
  State", skip that deliverable rather than creating a new block under
  a wrong name — creating unnamed text blocks will not render where
  the user expected them.
- Pay attention to quantity and format requirements in the Goal:
  "one per list item", "as a checklist", "in the findings block".
  These are instructions, not suggestions.

## You Are Running Autonomously

This is a scheduled, non-interactive run. There is no user available to
answer questions. Do not ask "would you like me to...", do not wait for
confirmation, do not end your turn with an open question. If you lack
information, use your tools to find it; if you cannot find it, record
what you did discover and finish the run.

## Operational Rules

- Be concise in your text responses — the user reads tool results, not
  your narration.
- If a web search or fetch fails, try alternative approaches: different
  search terms, different sources, fallback aggregators. Do not give
  up after one failure.
- Every deliverable the Goal names is either completed (tool called)
  or explicitly noted as unreachable. Silent omission is a failure.
`, time.Now().Format("2006-01-02 (Monday)"), agentCfg.Goal))

	// Agent card type guidance — but only list the fields that actually
	// exist on this card. A previous version of this prompt listed all
	// standard agent fields unconditionally, which wasted context on
	// fields the card didn't have AND used soft "if they exist" language
	// the LLM routinely ignored. Now we scan the card once, collect the
	// real agent-style fields, and emit a hard MUST-update list scoped
	// to what's present.
	if card.Type == "agent" {
		agentFields := promptfmt.CollectAgentFields(card)
		if len(agentFields) > 0 {
			sb.WriteString(`
## MANDATORY: Update These Card Fields Before Finishing

This card has agent-tracking fields that MUST be updated at the end of
every run — this is not optional housekeeping, it is part of the card's
contract. Before you finish, you MUST call update_self to set each of
the following. A run that completes without updating these fields is a
failed run.

`)
			for _, f := range agentFields {
				sb.WriteString(fmt.Sprintf("- **`%s`** (type: `%s`) — %s\n", f.Key, f.BlockType, f.Guidance))
			}
			sb.WriteString(`
You may batch all of these into a single update_self call with multiple
updates in the "updates" array. Do this as the final step of your run,
after completing the Goal's other deliverables.
`)
		}
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
		// Render each block with its type and key so the LLM knows how
		// to target it via update_self. Previously we only showed label
		// and value, which left the LLM guessing at block types — a
		// major reason agents wrote strings into list blocks.
		sb.WriteString("\n### Card Content\n")
		sb.WriteString("(Use the `key` column as the `key` argument to update_self. Match value format to `type`.)\n\n")
		for _, b := range card.Blocks {
			label := b.Label
			if label == "" {
				label = b.Key
			}
			if label == "" {
				label = b.Type
			}
			key := b.Key
			if key == "" {
				key = "(none — match by label)"
			}
			sb.WriteString(fmt.Sprintf("- **%s** (type: `%s`, key: `%s`)\n", label, b.Type, key))
			valueStr := promptfmt.FormatBlockValueForPrompt(b)
			if valueStr != "" {
				sb.WriteString(fmt.Sprintf("    current value: %s\n", valueStr))
			}
		}
	}

	// Repository context
	if b.deps.Repo() != nil && b.deps.Repo().Manifest.Description != "" {
		sb.WriteString(fmt.Sprintf("\n## Repository: %s\n%s\n", b.deps.Repo().Manifest.Name, b.deps.Repo().Manifest.Description))
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
