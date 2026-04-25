// Package chat is the ChatService — chat history I/O for both per-card
// and per-project conversations. Both kinds share one persistence
// backend (internal/config's LoadChatFor / SaveChatFor); project chats
// use a synthetic "__project__<id>" chat ID so card + project histories
// don't collide.
//
// The LLM chat loop (runChatLoop + tool dispatch) stays on App for
// now — it's deeply intertwined with the ~2500-line agent tools file
// and belongs in a future extraction that groups chat + agent + tool
// execution under a shared LLM-runtime package.
package chat

import (
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
)

// Deps is the narrow host contract for ChatService.
type Deps interface {
	Repo() *repo.Repository
}

// Service exposes chat history I/O.
type Service struct{ deps Deps }

// New constructs a ChatService.
func New(deps Deps) *Service { return &Service{deps: deps} }

// ProjectChatID returns the synthetic chat ID used to store project
// chat messages. Exported because the LLM runtime on App needs to
// build the same ID when saving mid-conversation.
func ProjectChatID(projectID string) string {
	return "__project__" + projectID
}

// LoadCardHistory returns the card's chat file.
func (s *Service) LoadCardHistory(cardID string) (*model.ChatFile, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	return config.LoadChatFor(r.Manifest.ID, cardID)
}

// LoadProjectHistory returns the project's chat file, resolving the
// project's UUID from its slug path.
func (s *Service) LoadProjectHistory(brandSlug, streamSlug, projectSlug string) (*model.ChatFile, error) {
	r := s.deps.Repo()
	if r == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	return config.LoadChatFor(r.Manifest.ID, ProjectChatID(project.ID))
}

// ClearProjectHistory wipes the project's chat to a fresh empty file.
func (s *Service) ClearProjectHistory(brandSlug, streamSlug, projectSlug string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	project, err := r.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return err
	}
	chatID := ProjectChatID(project.ID)
	return config.SaveChatFor(r.Manifest.ID, &model.ChatFile{CardID: chatID, Messages: []model.ChatMessage{}})
}

// ClearCardHistory wipes a card's chat to a fresh empty file.
func (s *Service) ClearCardHistory(cardID string) error {
	r := s.deps.Repo()
	if r == nil {
		return fmt.Errorf("no repository open")
	}
	return config.SaveChatFor(r.Manifest.ID, &model.ChatFile{CardID: cardID, Messages: []model.ChatMessage{}})
}
