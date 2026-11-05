package prompts

// Briefs for a chat BRUV doesn't run itself (UNCLI, BRUV's other chat
// engine): what the model is told once, when the chat's CLI connects to
// BRUV's board tools (the MCP initialize reply's instructions). They
// carry the parts of Card's and Project's prompts that hold for the whole
// chat — what it's about, the user's context, the card types, the
// project's descriptions, instructions and tags — and leave out what the
// tools and their descriptions already say, and what changes turn to
// turn (the page sends that with each turn; get_card has the rest).

import (
	"fmt"
	"strings"

	"bruv/internal/config"
	"bruv/internal/model"
)

// CardChatBrief is the brief for the chat about card.
func (b *Builder) CardChatBrief(card *model.Card, cfg config.LLMConfig) string {
	parts := []string{fmt.Sprintf("This is the user's BRUV chat about the card %q (card_id %s), which they have open beside it. "+
		"Read it with get_card before changing it.", card.Title, card.ID)}
	loc, err := b.deps.Card().GetLocation(card.ID)
	switch {
	case err != nil:
		parts = append(parts, "The card isn't filed on a board yet (it's in the inbox): when you know what it is, suggest a place with pin_card.")
	case card.ContextLevel != model.ContextIsolated:
		parts = append(parts, b.projectBrief(loc.BrandSlug, loc.StreamSlug, loc.ProjectSlug, card.ContextLevel == model.ContextBrand || card.ContextLevel == model.ContextGlobal)...)
	}
	return strings.Join(append(parts, b.boardBrief(cfg)...), "\n\n")
}

// ProjectChatBrief is the brief for the chat about a project.
func (b *Builder) ProjectChatBrief(brandSlug, streamSlug, projectSlug string, cfg config.LLMConfig) string {
	parts := []string{"This is the user's BRUV chat about one project, which they have open beside it. " +
		"Its board tools default to this project: leave brand, stream and project out. list_cards shows its cards by category."}
	parts = append(parts, b.projectBrief(brandSlug, streamSlug, projectSlug, true)...)
	if r := b.deps.Repo(); r != nil {
		if ws, err := r.GetWorkspace(brandSlug, streamSlug, projectSlug); err == nil {
			line := fmt.Sprintf("Workspace: the project has one attached (origin: %s, adapter: %s); workspace_index lists its files.", ws.Origin.Kind, ws.Adapter)
			if idx, err := r.GetWorkspaceIndex(brandSlug, streamSlug, projectSlug); err == nil && idx.Summary != "" {
				line += "\n" + idx.Summary
			}
			parts = append(parts, line)
		}
	}
	return strings.Join(append(parts, b.boardBrief(cfg)...), "\n\n")
}

// projectBrief is a project's place, descriptions, tag vocabulary and,
// when withBrand, its brand's instructions.
func (b *Builder) projectBrief(brandSlug, streamSlug, projectSlug string, withBrand bool) []string {
	r := b.deps.Repo()
	if r == nil {
		return nil
	}
	brand, errB := r.GetBrand(brandSlug)
	stream, errS := r.GetStream(brandSlug, streamSlug)
	project, errP := r.GetProject(brandSlug, streamSlug, projectSlug)
	if errB != nil || errS != nil || errP != nil {
		return nil
	}
	var parts []string
	where := fmt.Sprintf("Project: %s (in %s › %s).", project.Name, brand.Name, stream.Name)
	for _, d := range []struct{ what, text string }{{"Brand", brand.Description}, {"Stream", stream.Description}, {"Project", project.Description}} {
		if d.text != "" {
			where += "\n" + d.what + ": " + d.text
		}
	}
	parts = append(parts, where)
	if withBrand && brand.SystemPrompt != "" {
		parts = append(parts, "Brand instructions:\n"+brand.SystemPrompt)
	}
	if labels, err := r.GetProjectLabels(brandSlug, streamSlug, projectSlug); err == nil && len(labels) > 0 {
		names := make([]string, len(labels))
		for i, l := range labels {
			names[i] = l.Name
		}
		parts = append(parts, "The project's tags (prefer these; add short new ones only if none fit): "+strings.Join(names, ", "))
	}
	return parts
}

// boardBrief is what holds board-wide: the user's own context and the
// card types with their hints.
func (b *Builder) boardBrief(cfg config.LLMConfig) []string {
	var parts []string
	if cfg.Context != "" {
		parts = append(parts, "About the user (their own words, from BRUV's settings):\n"+cfg.Context)
	}
	if roster := b.cardTypeRoster(); roster != "" {
		parts = append(parts, roster)
	}
	return parts
}
