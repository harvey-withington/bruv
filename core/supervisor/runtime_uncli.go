package supervisor

// The UNCLI chat engine's Suggest mode (internal/mcpserver): a board
// write its session makes is staged, not applied, as a pending edit on
// the chat's own history, through the same staging (and so the same
// project scope and apply path) as BRUV's own chat in Suggest mode. The
// user reviews it from the UNCLI chat's banner with Apply(Project)
// PendingEdits.

import (
	"fmt"
	"strings"
	"time"

	"bruv/core/runtime/tools"
	chatsvc "bruv/core/services/chat"
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"

	"github.com/google/uuid"
)

// UncliStagedContent is the text of the history message holding a UNCLI
// chat's staged edits (the surfaces show the edits, not this).
const UncliStagedContent = "Edits proposed in the UNCLI chat."

// StageUncliEdit stages a board-tool write for the chat chatID (a card
// id, or a project's chatsvc.ProjectChatID) and returns what the model
// is told: what was staged, or why it was refused. A read runs as it
// would in Suggest mode.
func (r *Runtime) StageUncliEdit(chatID, tool string, args map[string]any) (string, error) {
	if r.repo == nil {
		return "", fmt.Errorf("no repository open")
	}
	tc := llm.ToolCall{Name: tool, Arguments: args}
	var result string
	var edits []model.PendingEdit
	if projectID, ok := strings.CutPrefix(chatID, chatsvc.ProjectChatID("")); ok {
		loc, err := r.GetProjectLocation(projectID)
		if err != nil {
			return "", err
		}
		result, edits = r.tools.StageProject(tc, tools.ScopeForProject(r.repo, loc.BrandSlug, loc.StreamSlug, loc.ProjectSlug))
	} else {
		allCats, err := r.ListAllCategories()
		if err != nil {
			return "", err
		}
		result, edits = r.tools.StageCard(chatID, tc, allCats)
	}
	if len(edits) == 0 {
		return result, nil
	}
	_, err := config.UpdateChat(r.repo.Manifest.ID, chatID, func(cf *model.ChatFile) error {
		cf.Messages = append(cf.Messages, model.ChatMessage{
			ID:           uuid.New().String(),
			Role:         "assistant",
			Content:      UncliStagedContent,
			Timestamp:    time.Now().UTC(),
			PendingEdits: edits,
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}
