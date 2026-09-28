// Package boardtools is BRUV's native tool set: the one registry of
// board tools (cards, hierarchy, attachments, comments, agents) that
// every LLM surface calls in-process — the MCP server, card chat,
// project chat and agents. The MCP server is just one transport over
// it, so a tool or fix added here reaches every surface at once.
package boardtools

import (
	"bruv/core/services/agentsvc"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	projectsvc "bruv/core/services/project"
	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
)

// Board is the slice of an open repo the tools act on. Satisfied by
// *supervisor.Runtime; tests can supply their own.
type Board interface {
	Repo() *repo.Repository
	CardService() *card.Service
	ProjectService() *projectsvc.Service
	CatalogService() *catalog.Service
	AgentService() *agentsvc.Service

	GetCard(id string) (*model.Card, error)
	CreateCard(cardType, title string) (*model.Card, error)
	UpdateCardTitle(id, title string) (*model.Card, error)
	UpdateCardDescription(id, description string) (*model.Card, error)
	UpdateCardType(id, cardType string) (*model.Card, error)
	UpdateCardDueDate(id, dueDate string) (*model.Card, error)
	UpdateCardTags(id string, tags []string) (*model.Card, error)
	UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error)
	PinCard(cardID, categoryID string) error
	UnpinCard(cardID, categoryID string) error
	SearchCards(query string, limit int) ([]index.SearchResult, error)
	RecentCards(limit int) ([]index.SearchResult, error)

	AddCardAttachment(cardID, name, data string) (*model.Card, error)
	ReadCardAttachment(cardID, attachmentID string) ([]byte, *model.FileAttachment, error)
	SignAttachmentURL(cardID, attachmentID string) (string, error)
	AddCardComment(cardID, author, text string) (*model.Comment, error)
	ListCardComments(cardID string) ([]model.Comment, error)

	ListBrands() ([]model.Brand, error)
	ListStreams(brandSlug string) ([]model.Stream, error)
	ListProjects(brandSlug, streamSlug string) ([]model.Project, error)
	ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error)
	CreateBrand(name string) (*model.Brand, error)
	CreateStream(brandSlug, name string) (*model.Stream, error)
	CreateProject(brandSlug, streamSlug, name string) (*model.Project, error)
	CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error)
	UpdateBrandDescription(slug, description string) (*model.Brand, error)
	UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error)
	UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error)

	ListCardTypes() []catalog.CardTypeInfo
	ResolveOrCreateCardType(input string) (id string, created bool, err error)

	GetAgentConfig(cardID string) (*model.AgentFile, error)
	TriggerAgent(cardID string) error
}
