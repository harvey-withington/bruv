// Package promptfmt holds the pure formatting helpers used by the LLM
// system-prompt builders (buildSystemPrompt / buildProjectSystemPrompt /
// buildAgentSystemPrompt). Every function here is a pure data→string
// transform: no App state, no services, no I/O.
//
// Extracted from app_chat.go and app_agent.go as the first step of the
// wider LLM-runtime extraction (see plan/llm-runtime-extraction-*.md).
// The stateful builders themselves — the ones that query repo + tags +
// categories + registry — remain in main for now and await a dedicated
// session. These helpers alone don't unblock headless agents, but they
// reduce the surface the bigger refactor has to touch and prove the
// package layout works.
package promptfmt

import (
	"fmt"
	"strings"

	"bruv/internal/model"
)

// AvailableIconList returns the canonical comma-separated icon name
// list embedded in LLM system prompts. MUST stay in sync with
// ICON_MAP in frontend/src/lib/icons.ts — the frontend's DynamicIcon
// renders an unknown-icon placeholder for names outside that map, so
// picking a name from outside this list silently breaks display.
//
// When you add an icon to ICON_MAP, add it here too. (And vice versa.)
func AvailableIconList() string {
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

// RenderCategoryHeader produces the markdown header line(s) for a
// category in the project chat system prompt. Includes name + ID,
// optional description, optional icon, and accepted_types restriction
// (if any).
func RenderCategoryHeader(cat model.Category) string {
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

// FormatCardContent formats a card's content as readable text.
// Used in agent system prompts to show the LLM what the card currently
// contains before it decides what to change.
func FormatCardContent(card *model.Card) string {
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

// FormatBlockValueForPrompt renders a block's current value for
// inclusion in the agent system prompt. Different block types have
// different value shapes ([]map[string]any for lists/checklists,
// string for text, etc.) and a raw %v dump produces garbage for the
// complex ones — so we format each type cleanly so the LLM can see
// what's already on the card and decide whether to append or replace.
func FormatBlockValueForPrompt(b model.Block) string {
	if b.Value == nil {
		return ""
	}
	switch b.Type {
	case model.BlockList:
		items, ok := b.Value.([]any)
		if !ok {
			return fmt.Sprintf("(unexpected format) %v", b.Value)
		}
		if len(items) == 0 {
			return "(empty list)"
		}
		var texts []string
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					texts = append(texts, t)
				}
			}
		}
		return "[" + strings.Join(texts, " | ") + "]"

	case model.BlockChecklist:
		items, ok := b.Value.([]any)
		if !ok {
			return fmt.Sprintf("(unexpected format) %v", b.Value)
		}
		if len(items) == 0 {
			return "(empty checklist)"
		}
		var parts []string
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				text, _ := m["text"].(string)
				done, _ := m["done"].(bool)
				mark := "[ ]"
				if done {
					mark = "[x]"
				}
				parts = append(parts, fmt.Sprintf("%s %s", mark, text))
			}
		}
		return strings.Join(parts, " | ")

	case model.BlockText:
		s, _ := b.Value.(string)
		// Truncate long text so the prompt doesn't balloon. The LLM
		// doesn't need to see 5000 characters of existing description
		// to decide what to update.
		if len(s) > 200 {
			return s[:200] + "…"
		}
		return s

	default:
		// Single-value types: number, bool, string, date, etc.
		return fmt.Sprintf("%v", b.Value)
	}
}

// AgentField describes a card block that an agent's system prompt
// flags as mandatory-update-before-finishing. Exported so builder
// code can iterate the returned list.
type AgentField struct {
	Key       string
	BlockType string
	Guidance  string
}

// AgentFieldGuidance is the guidance text the agent receives for each
// well-known tracking field. Custom tracking fields fall back to a
// generic guidance string.
var AgentFieldGuidance = map[string]string{
	"status":      "Set to one of: \"success\", \"failed\", \"idle\". Use \"success\" if the Goal was completed, \"failed\" if something blocked you.",
	"last_run":    "Write a 1–2 sentence summary of what you actually did this run — tools called, findings, or errors encountered.",
	"last_run_at": "Set to the CURRENT timestamp as an ISO 8601 string (see System Context below for the exact value).",
	"findings":    "Append new findings to the existing value. Do not overwrite prior findings — the value should accumulate across runs. If there's nothing new this run, restate the latest.",
	"description": "Only update if the description is empty or materially out of date; otherwise leave it alone.",
	"next_check":  "If the Goal involves ongoing monitoring, set this to the ISO 8601 timestamp of when you should next run.",
	"error":       "Set to a brief error message if the run failed, or clear it (empty string) on success.",
}

const customAgentFieldGuidance = "This is a custom tracking field — update it with whatever value this run produces for it. If this run didn't produce a new value, leave it alone."

// CollectAgentFields returns the tracking fields on an agent card that
// must be updated before the run finishes. Known fields come first in
// a deterministic order (aids prompt caching + consistency across
// runs); custom fields follow.
func CollectAgentFields(card *model.Card) []AgentField {
	orderedKnown := []string{"status", "last_run", "last_run_at", "findings", "description", "next_check", "error"}
	seen := make(map[string]bool)
	var fields []AgentField

	for _, knownKey := range orderedKnown {
		for _, b := range card.Blocks {
			if b.Key == knownKey {
				fields = append(fields, AgentField{
					Key:       b.Key,
					BlockType: b.Type,
					Guidance:  AgentFieldGuidance[knownKey],
				})
				seen[knownKey] = true
				break
			}
		}
	}

	// Custom agent fields: any other block whose key isn't a known
	// one but whose block-type suggests it's a tracking field (rating,
	// select, date, etc., with a non-empty key). We err on the side
	// of inclusion — the LLM can decide whether a custom field is
	// genuinely relevant this run.
	for _, b := range card.Blocks {
		if b.Key == "" || seen[b.Key] {
			continue
		}
		if !isLikelyAgentField(b) {
			continue
		}
		fields = append(fields, AgentField{
			Key:       b.Key,
			BlockType: b.Type,
			Guidance:  customAgentFieldGuidance,
		})
	}

	return fields
}

// isLikelyAgentField is a heuristic for custom template-defined
// agent fields. We match on block types typically used for status
// tracking (number, rating, select, date, progress, checkbox) AND
// require a non-empty key — freeform text blocks don't count because
// they're the Goal's working surface, not tracking state.
func isLikelyAgentField(b model.Block) bool {
	switch b.Type {
	case model.BlockNumber, model.BlockRating, model.BlockSelect,
		model.BlockDate, model.BlockProgress, model.BlockCheckbox:
		return b.Key != ""
	}
	return false
}
