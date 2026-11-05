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
	inotify "bruv/internal/notify"

	"github.com/google/uuid"
)

// UncliChatID is the chat (a card id, or a project's chatsvc.ProjectChatID)
// a UNCLI session is: from the meta the page made it with
// (shared/uncliChat.chatMeta: "bruv/<repo>/card/<card>" or
// "bruv/<repo>/project/<brand>/<stream>/<project>"), else from BRUV's
// record of the session. repoID is this repo's id in the registry (the
// one pages and URLs use); false when it's no chat of this repo's.
func (r *Runtime) UncliChatID(repoID, meta, session string) (string, bool) {
	if r.repo == nil {
		return "", false
	}
	parts := strings.Split(meta, "/")
	if len(parts) >= 4 && parts[0] == "bruv" && parts[1] == repoID {
		switch {
		case parts[2] == "card" && len(parts) == 4 && parts[3] != "":
			return parts[3], true
		case parts[2] == "project" && len(parts) == 6:
			if p, err := r.repo.GetProject(parts[3], parts[4], parts[5]); err == nil {
				return chatsvc.ProjectChatID(p.ID), true
			}
		}
	}
	if chat, ok := config.UncliSessionChat(session); ok && chat.RepoID == repoID {
		return chat.ChatID, true
	}
	return "", false
}

// UncliChatBrief is what a UNCLI chat's model is told about its chat when
// it connects (prompts.CardChatBrief / ProjectChatBrief); "" when the
// chat's card or project can't be read.
func (r *Runtime) UncliChatBrief(chatID string) string {
	cfg, err := r.LLM.GetConfig()
	if err != nil {
		cfg = config.LLMConfig{}
	}
	if projectID, ok := strings.CutPrefix(chatID, chatsvc.ProjectChatID("")); ok {
		loc, err := r.GetProjectLocation(projectID)
		if err != nil {
			return ""
		}
		return r.prompts.ProjectChatBrief(loc.BrandSlug, loc.StreamSlug, loc.ProjectSlug, cfg)
	}
	card, err := r.GetCard(chatID)
	if err != nil {
		return ""
	}
	return r.prompts.CardChatBrief(card, cfg)
}

// NotifyUncliApproval tells the user a UNCLI chat (chatID, as UncliChatID)
// is waiting for them: an in-app notification, which a server mirrors to
// the phones that take push. A card's chat opens on a tap, and its alerts
// replace each other rather than stack. what is the approval's own title.
func (r *Runtime) NotifyUncliApproval(chatID, what string) {
	n := inotify.Request{Source: "uncli_chat", Body: what, Channels: inotify.ParseChannels("in-app")}
	if projectID, ok := strings.CutPrefix(chatID, chatsvc.ProjectChatID("")); ok {
		name := "a project"
		if loc, err := r.GetProjectLocation(projectID); err == nil {
			if p, err := r.repo.GetProject(loc.BrandSlug, loc.StreamSlug, loc.ProjectSlug); err == nil {
				name = "“" + p.Name + "”"
			}
		}
		n.Title = "Your chat on " + name + " is waiting for you"
	} else {
		n.CardID, n.Title = chatID, "Your chat on a card is waiting for you"
		if card, err := r.GetCard(chatID); err == nil {
			n.CardTitle, n.Title = card.Title, "Your chat on “"+card.Title+"” is waiting for you"
		}
	}
	cfg, err := config.LoadNotifyConfig()
	if err != nil {
		cfg = config.DefaultNotifyConfig()
	}
	inotify.NewDispatcher(cfg, func(name string, data any) { r.bus.Publish(name, data) }).Send(n)
}

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
