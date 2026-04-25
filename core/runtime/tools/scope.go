package tools

import "bruv/core/services/card"

// CategoryPath is the tool-layer alias for card.CategoryPath. Every
// tool call that references the card hierarchy uses this shape; the
// alias lets the dispatcher code read naturally while the canonical
// type definition stays in the card service package.
type CategoryPath = card.CategoryPath

// ProjectChatScope bundles the identity of the project a project-chat
// call is operating inside. Used by ExecuteProjectTool and
// StageProjectTool to validate that every card_id the LLM mentions
// actually belongs to the active project — this is the defence
// against LLM hallucinations that touch cards outside scope.
//
// Moved here from the main package so the tools package owns its
// scope type; app_chat.go references it through this package.
type ProjectChatScope struct {
	BrandSlug   string
	StreamSlug  string
	ProjectSlug string
	CardIDs     map[string]bool
}
