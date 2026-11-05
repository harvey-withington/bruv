package supervisor

// The UNCLI chat engine's Suggest mode (internal/mcpserver): a board
// write its session makes is staged, not applied, as a pending edit on
// the chat's own history, through the same staging (and so the same
// project scope and apply path) as BRUV's own chat in Suggest mode. When
// the turn ends the page asks about all of them on one UNCLI review card
// and applies the ticked ones with Apply(Project)PendingEdits
// (shared/uncliChat.reviewStagedChanges).

import (
	"fmt"
	"strings"
	"time"

	"bruv/core/boardtools"
	"bruv/core/runtime/tools"
	chatsvc "bruv/core/services/chat"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"

	"github.com/google/uuid"
)

// UncliStagedContent is the text of the history message holding a UNCLI
// chat's staged edits; the pages pick those messages out by it
// (shared/uncliChat UNCLI_STAGED_CONTENT) and show the edits, not this.
const UncliStagedContent = "Edits proposed in the UNCLI chat."

// StageUncliEdit stages a board-tool write for the chat chatID (a card
// id, or a project's chatsvc.ProjectChatID). It returns the edits staged,
// or why the staging refused the call (outside the chat's project, bad
// arguments), for the model.
func (r *Runtime) StageUncliEdit(chatID, tool string, args map[string]any) (staged []model.PendingEdit, refused string, err error) {
	if r.repo == nil {
		return nil, "", fmt.Errorf("no repository open")
	}
	tc := llm.ToolCall{Name: tool, Arguments: args}
	var result string
	if projectID, ok := strings.CutPrefix(chatID, chatsvc.ProjectChatID("")); ok {
		loc, err := r.GetProjectLocation(projectID)
		if err != nil {
			return nil, "", err
		}
		result, staged = r.tools.StageProject(tc, tools.ScopeForProject(r.repo, loc.BrandSlug, loc.StreamSlug, loc.ProjectSlug))
	} else {
		allCats, err := r.ListAllCategories()
		if err != nil {
			return nil, "", err
		}
		result, staged = r.tools.StageCard(chatID, tc, allCats)
	}
	if len(staged) == 0 {
		return nil, strings.TrimPrefix(result, "error: "), nil
	}
	// One call, one change: word it for the review card, naming the card.
	if len(staged) == 1 {
		staged[0].Label, staged[0].Detail = boardtools.DescribeCall(r, tool, args)
	}
	_, err = config.UpdateChat(r.repo.Manifest.ID, chatID, func(cf *model.ChatFile) error {
		cf.Messages = append(cf.Messages, model.ChatMessage{
			ID:           uuid.New().String(),
			Role:         "assistant",
			Content:      UncliStagedContent,
			Timestamp:    time.Now().UTC(),
			PendingEdits: staged,
		})
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return staged, "", nil
}
