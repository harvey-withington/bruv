package prompts

import (
	"bruv/core/services/card"
	"bruv/core/services/search"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// Deps is the narrow host contract for Builder. Prompt construction
// needs: repo access (agent config lookups, brand system prompts,
// project labels), schema registry (available card-type descriptions
// for the card-chat prompt), card service (location + all-categories
// enumeration), and search service (tag usage counts for project
// chat).
type Deps interface {
	Repo() *repo.Repository
	Registry() *schema.Registry
	Card() *card.Service
	Search() *search.Service
}

// CategoryPath is the prompt-layer alias for card.CategoryPath.
// Mirrors the alias pattern established in core/runtime/tools so the
// extracted builder code reads naturally while the canonical type
// definition stays in the card service package.
type CategoryPath = card.CategoryPath

// Builder constructs LLM system prompts. One instance per App / runtime.
type Builder struct {
	deps Deps
}

// New constructs a prompt Builder.
func New(deps Deps) *Builder {
	return &Builder{deps: deps}
}
