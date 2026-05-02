package supervisor

// Per-repo RPC surface for the headless runtime.
//
// Mirrors the desktop App's Wails-bound per-repo methods so the
// JSON-RPC reflection dispatcher exposes the same ~140-method API
// to the frontend whether it's talking to a remote bruv-server or
// to the desktop's local loopback. The original definitions live
// in app_*.go on the desktop side; this file is the (currently
// hand-mirrored) copy. Future cleanup: extract a shared Ops struct
// embedded by both App and Runtime so the methods are
// defined once.

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bruv/core/runtime/promptfmt"
	"bruv/core/runtime/tools"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	chatsvc "bruv/core/services/chat"
	llmsvc "bruv/core/services/llm"
	"bruv/core/services/mcpsvc"
	"bruv/internal/config"
	"bruv/internal/importer"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/notify"
	"bruv/internal/repo"
)

// Type aliases for stable wire shapes (mirror app_card.go +
// app_tools.go).
type CardLocation = card.CardLocation
type CategoryPath = card.CategoryPath
type projectChatScope = tools.ProjectChatScope


// Wails-bound forwarders for card CRUD, mutations, pins, moves,
// checklist, and comments. Domain logic lives in core/services/card.


// Type aliases preserve stable Wails TS bindings.

// --- CRUD ---

func (r *Runtime) CreateCard(cardType, title string) (*model.Card, error) {
	return r.Card.Create(cardType, title)
}
func (r *Runtime) GetCard(id string) (*model.Card, error) { return r.Card.Get(id) }
func (r *Runtime) ListCards() ([]model.Card, error)       { return r.Card.List() }

func (r *Runtime) DuplicateCard(cardID, categoryID string) (*model.Card, error) {
	return r.Card.Duplicate(cardID, categoryID)
}
func (r *Runtime) CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug string) (*model.Category, error) {
	return r.Card.CopyCategory(brandSlug, streamSlug, projectSlug, categorySlug)
}
func (r *Runtime) DeleteCard(id string) error { return r.Card.Delete(id) }

// --- Mutations ---

func (r *Runtime) UpdateCardTitle(id, title string) (*model.Card, error) {
	return r.Card.UpdateTitle(id, title)
}
func (r *Runtime) UpdateCardType(id, cardType string) (*model.Card, error) {
	return r.Card.UpdateType(id, cardType)
}
// UpdateCardDescription replaces the card's intrinsic description.
// The legacy UpdateCardFields RPC was deleted in favour of this —
// the previous "fields map with a magic 'description' key" model
// caused silent desync with blocks; description is now first-class.
func (r *Runtime) UpdateCardDescription(id, description string) (*model.Card, error) {
	return r.Card.UpdateDescription(id, description)
}
func (r *Runtime) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return r.Card.UpdateBlocks(id, blocks)
}
func (r *Runtime) AddCardAttachment(cardID, name, data string) (*model.Card, error) {
	return r.Card.AddAttachment(cardID, name, data)
}
func (r *Runtime) RemoveCardAttachment(cardID, attachmentID string) (*model.Card, error) {
	return r.Card.RemoveAttachment(cardID, attachmentID)
}
func (r *Runtime) UpdateCardTags(id string, tags []string) (*model.Card, error) {
	return r.Card.UpdateTags(id, tags)
}
func (r *Runtime) UpdateCardDueDate(id, dueDate string) (*model.Card, error) {
	return r.Card.UpdateDueDate(id, dueDate)
}

// --- Category helpers / pins / moves ---

func (r *Runtime) GetCategoryAcceptedTypes(categoryID string) ([]string, error) {
	return r.Card.GetCategoryAcceptedTypes(categoryID)
}
func (r *Runtime) PinCard(cardID, projectID, categoryID string) error {
	return r.Card.Pin(cardID, projectID, categoryID)
}
func (r *Runtime) UnpinCard(cardID, projectID, categoryID string) error {
	return r.Card.Unpin(cardID, projectID, categoryID)
}
func (r *Runtime) GetCardPins(cardID string) ([]model.Pin, error) {
	return r.Card.GetPins(cardID)
}
func (r *Runtime) GetCardLocation(cardID string) (*CardLocation, error) {
	return r.Card.GetLocation(cardID)
}
func (r *Runtime) GetProjectLocation(projectID string) (*CardLocation, error) {
	return r.Card.GetProjectLocation(projectID)
}
func (r *Runtime) ListAllCategories() ([]CategoryPath, error) {
	return r.Card.ListAllCategories()
}
func (r *Runtime) GetCardPinBreadcrumbs(cardID string) ([]CategoryPath, error) {
	return r.Card.GetPinBreadcrumbs(cardID)
}
func (r *Runtime) MoveCardInCategory(cardID, projectID, categoryID string, newPosition int) error {
	return r.Card.MoveInCategory(cardID, projectID, categoryID, newPosition)
}
func (r *Runtime) MoveCardToCategory(cardID, projectID, fromCategoryID, toCategoryID string, newPosition int) error {
	return r.Card.MoveToCategory(cardID, projectID, fromCategoryID, toCategoryID, newPosition)
}

// --- Comments ---

func (r *Runtime) ListCardComments(cardID string) ([]model.Comment, error) {
	return r.Card.ListComments(cardID)
}
func (r *Runtime) AddCardComment(cardID, author, text string) (*model.Comment, error) {
	return r.Card.AddComment(cardID, author, text)
}
func (r *Runtime) UpdateCardComment(cardID, commentID, text string) (*model.Comment, error) {
	return r.Card.UpdateComment(cardID, commentID, text)
}
func (r *Runtime) DeleteCardComment(cardID, commentID string) error {
	return r.Card.DeleteComment(cardID, commentID)
}

// Wails-bound forwarders for brand CRUD. Domain logic lives in
// core/services/project.


func (r *Runtime) CreateBrand(name string) (*model.Brand, error) { return r.Project.CreateBrand(name) }
func (r *Runtime) GetBrand(slug string) (*model.Brand, error)    { return r.Project.GetBrand(slug) }
func (r *Runtime) ListBrands() ([]model.Brand, error)            { return r.Project.ListBrands() }
func (r *Runtime) RenameBrand(slug, newName string) (*model.Brand, error) {
	return r.Project.RenameBrand(slug, newName)
}
func (r *Runtime) UpdateBrandDescription(slug, description string) (*model.Brand, error) {
	return r.Project.UpdateBrandDescription(slug, description)
}
func (r *Runtime) UpdateBrandIcon(slug, icon string) (*model.Brand, error) {
	return r.Project.UpdateBrandIcon(slug, icon)
}
func (r *Runtime) DeleteBrand(slug string) error { return r.Project.DeleteBrand(slug) }

// Wails-bound forwarders for stream CRUD. Domain logic lives in
// core/services/project.


func (r *Runtime) CreateStream(brandSlug, name string) (*model.Stream, error) {
	return r.Project.CreateStream(brandSlug, name)
}
func (r *Runtime) ListStreams(brandSlug string) ([]model.Stream, error) {
	return r.Project.ListStreams(brandSlug)
}
func (r *Runtime) RenameStream(brandSlug, streamSlug, newName string) (*model.Stream, error) {
	return r.Project.RenameStream(brandSlug, streamSlug, newName)
}
func (r *Runtime) UpdateStreamDescription(brandSlug, streamSlug, description string) (*model.Stream, error) {
	return r.Project.UpdateStreamDescription(brandSlug, streamSlug, description)
}
func (r *Runtime) UpdateStreamIcon(brandSlug, streamSlug, icon string) (*model.Stream, error) {
	return r.Project.UpdateStreamIcon(brandSlug, streamSlug, icon)
}
func (r *Runtime) DeleteStream(brandSlug, streamSlug string) error {
	return r.Project.DeleteStream(brandSlug, streamSlug)
}

// Wails-bound forwarders for project CRUD. Domain logic lives in
// core/services/project.


func (r *Runtime) CreateProject(brandSlug, streamSlug, name string) (*model.Project, error) {
	return r.Project.CreateProject(brandSlug, streamSlug, name)
}
func (r *Runtime) ListProjects(brandSlug, streamSlug string) ([]model.Project, error) {
	return r.Project.ListProjects(brandSlug, streamSlug)
}
func (r *Runtime) RenameProject(brandSlug, streamSlug, projectSlug, newName string) (*model.Project, error) {
	return r.Project.RenameProject(brandSlug, streamSlug, projectSlug, newName)
}
func (r *Runtime) UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description string) (*model.Project, error) {
	return r.Project.UpdateProjectDescription(brandSlug, streamSlug, projectSlug, description)
}
func (r *Runtime) UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon string) (*model.Project, error) {
	return r.Project.UpdateProjectIcon(brandSlug, streamSlug, projectSlug, icon)
}
func (r *Runtime) DeleteProject(brandSlug, streamSlug, projectSlug string) error {
	return r.Project.DeleteProject(brandSlug, streamSlug, projectSlug)
}

// Wails-bound forwarders for category CRUD. Domain logic lives in
// core/services/project.


func (r *Runtime) CreateCategory(brandSlug, streamSlug, projectSlug, name string, position int) (*model.Category, error) {
	return r.Project.CreateCategory(brandSlug, streamSlug, projectSlug, name, position)
}
func (r *Runtime) ListCategories(brandSlug, streamSlug, projectSlug string) ([]model.Category, error) {
	return r.Project.ListCategories(brandSlug, streamSlug, projectSlug)
}
func (r *Runtime) RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName string) (*model.Category, error) {
	return r.Project.RenameCategory(brandSlug, streamSlug, projectSlug, categorySlug, newName)
}
func (r *Runtime) UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description string) (*model.Category, error) {
	return r.Project.UpdateCategoryDescription(brandSlug, streamSlug, projectSlug, categorySlug, description)
}
func (r *Runtime) UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon string) (*model.Category, error) {
	return r.Project.UpdateCategoryIcon(brandSlug, streamSlug, projectSlug, categorySlug, icon)
}
func (r *Runtime) DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug string) error {
	return r.Project.DeleteCategory(brandSlug, streamSlug, projectSlug, categorySlug)
}
func (r *Runtime) UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug string, acceptedTypes []string) (*model.Category, error) {
	return r.Project.UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, categorySlug, acceptedTypes)
}
func (r *Runtime) MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID string) error {
	return r.Project.MoveCategoryCards(brandSlug, streamSlug, projectSlug, fromCategoryID, toCategoryID)
}

// Wails-bound forwarders for card types + templates + import/export.
// Domain logic lives in core/services/catalog.


// Exported type aliases preserve stable Wails TS bindings.
type CardTypeInfo = catalog.CardTypeInfo
type CardTypesExport = catalog.CardTypesExport
type CardTypesImportResult = catalog.CardTypesImportResult

// ListCardTypes returns all card types (built-in first, then user).
func (r *Runtime) ListCardTypes() []CardTypeInfo { return r.Catalog.ListCardTypes() }

func (r *Runtime) ValidateCardFields(cardType string, fields map[string]any) []string {
	return r.Catalog.ValidateCardFields(cardType, fields)
}

func (r *Runtime) CreateUserCardType(label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	return r.Catalog.CreateUserCardType(label, color, description, aiHint, templateID)
}

func (r *Runtime) UpdateUserCardType(id, label, color, description, aiHint, templateID string) (config.UserCardType, error) {
	return r.Catalog.UpdateUserCardType(id, label, color, description, aiHint, templateID)
}

func (r *Runtime) UpdateUserCardTypeIcon(id, icon string) (config.UserCardType, error) {
	return r.Catalog.UpdateUserCardTypeIcon(id, icon)
}

func (r *Runtime) DeleteUserCardType(id string) error { return r.Catalog.DeleteUserCardType(id) }

func (r *Runtime) UpdateBuiltinCardType(id, color, templateID string) error {
	return r.Catalog.UpdateBuiltinCardType(id, color, templateID)
}

func (r *Runtime) ListCardTemplates() ([]config.CardTemplate, error) {
	return r.Catalog.ListCardTemplates()
}

func (r *Runtime) CreateCardTemplate(name string, blocks []model.Block) (config.CardTemplate, error) {
	return r.Catalog.CreateCardTemplate(name, blocks)
}

func (r *Runtime) UpdateCardTemplate(id, name string, blocks []model.Block) (config.CardTemplate, error) {
	return r.Catalog.UpdateCardTemplate(id, name, blocks)
}

func (r *Runtime) DeleteCardTemplate(id string) error { return r.Catalog.DeleteCardTemplate(id) }

func (r *Runtime) RefreshTypeBlocks(cardID string) (*model.Card, error) {
	return r.Catalog.RefreshTypeBlocks(cardID)
}

func (r *Runtime) ExportCardTypesToFile(filePath string) error {
	return r.Catalog.ExportCardTypesToFile(filePath)
}

func (r *Runtime) ImportCardTypesFromFile(filePath, mode string) (CardTypesImportResult, error) {
	return r.Catalog.ImportCardTypesFromFile(filePath, mode)
}

func (r *Runtime) ImportCardTypesFromRepo(otherRepoPath, mode string) (CardTypesImportResult, error) {
	return r.Catalog.ImportCardTypesFromRepo(otherRepoPath, mode)
}

// Internal helpers — App forwarders so app_card.go's creation flow
// doesn't need to import the catalog package directly. When the card
// service is extracted these go away.
func (r *Runtime) applyTypeBlocks(cardID, cardType string) {
	r.Catalog.ApplyTypeBlocks(cardID, cardType)
}

func (r *Runtime) resolveTemplateBlocks(cardType string) []model.Block {
	return r.Catalog.ResolveTemplateBlocks(cardType)
}

// LLM chat — Wails-bound surface + thin forwarders.
//
// The chat runtime (runChatLoop, SendCard/SendProject cores,
// saveUserMessage helper, chatLoopConfig struct) lives in
// core/runtime/chat. The system-prompt builders live in
// core/runtime/prompts. Value-coercion + tool dispatch live in
// core/runtime/tools. This file keeps only the Wails-bound method
// forwarders that the generated ShellAPI-era bindings used to expose,
// plus chat-history shims and local wrappers around the pure
// formatting helpers in core/runtime/promptfmt.


// --- Chat history (forwarders to core/services/chat) ---

func (r *Runtime) LoadChatHistory(cardID string) (*model.ChatFile, error) {
	return r.Chat.LoadCardHistory(cardID)
}

// projectChatID returns the synthetic chat ID used to store project
// chat messages. Kept as a package-level helper so code outside the
// chat runtime (notifications, debugging) can compute it.
func projectChatID(projectID string) string { return chatsvc.ProjectChatID(projectID) }

func (r *Runtime) LoadProjectChatHistory(brandSlug, streamSlug, projectSlug string) (*model.ChatFile, error) {
	return r.Chat.LoadProjectHistory(brandSlug, streamSlug, projectSlug)
}

func (r *Runtime) ClearProjectChatHistory(brandSlug, streamSlug, projectSlug string) error {
	return r.Chat.ClearProjectHistory(brandSlug, streamSlug, projectSlug)
}

func (r *Runtime) ClearCardChatHistory(cardID string) error {
	return r.Chat.ClearCardHistory(cardID)
}

// --- Chat runtime forwarders (core/runtime/chat) ---

// SendChatMessage is the Wails-bound entry point for per-card chat.
// Forwards to the chat runtime.
func (r *Runtime) SendChatMessage(cardID, userMessage string) (*model.ChatFile, error) {
	return r.chatRT.SendCard(cardID, userMessage)
}

// SendProjectChatMessage is the Wails-bound entry point for
// project-level chat. Forwards to the chat runtime.
func (r *Runtime) SendProjectChatMessage(brandSlug, streamSlug, projectSlug, userMessage, contextLevel string) (*model.ChatFile, error) {
	return r.chatRT.SendProject(brandSlug, streamSlug, projectSlug, userMessage, contextLevel)
}

// --- Promptfmt wrappers ---
//
// availableIconList + renderCategoryHeader are one-line wrappers so
// other main-package files (app_agent.go in particular) don't each
// need to import core/runtime/promptfmt. The compiler inlines these.

func availableIconList() string { return promptfmt.AvailableIconList() }
func renderCategoryHeader(cat model.Category) string {
	return promptfmt.RenderCategoryHeader(cat)
}

// Wails-bound surface for search, index lifecycle, and two cross-cutting
// queries (ListActivityLog, ListRecentlyUpdatedCards) that read directly
// from repo + enrich with category context for the Inbox and activity
// feed views.
//
// Most methods here are thin forwarders to core/services/search — see
// that package for the domain logic. openIndex, ListActivityLog, and
// ListRecentlyUpdatedCards remain on App until their neighbouring
// services are extracted (repository lifecycle + inbox/activity,
// respectively).


// --- Index lifecycle (stays on App until repository-service extraction) ---

func (r *Runtime) openIndex(repoPath string) error {
	if r.idx != nil {
		r.idx.Close()
	}
	dbPath := filepath.Join(repoPath, ".bruv", "index.db")
	idx, err := index.Open(dbPath)
	if err != nil {
		return err
	}
	r.idx = idx
	return nil
}

// --- Search / index-backed lookups (forwarders to core/services/search) ---

// GetCardProjectContext returns the stored project hierarchy path for a card (e.g. "Brand > Stream > Project").
func (r *Runtime) GetCardProjectContext(cardID string) string {
	return r.Search.GetCardProjectContext(cardID)
}

// SearchCards performs a full-text search across all indexed cards.
func (r *Runtime) SearchCards(query string, limit int) ([]index.SearchResult, error) {
	return r.Search.SearchCards(query, limit)
}

// SearchOrphanedCards performs a full-text search limited to orphaned (inbox) cards.
func (r *Runtime) SearchOrphanedCards(query string, limit int) ([]index.SearchResult, error) {
	return r.Search.SearchOrphanedCards(query, limit)
}

// RebuildIndex drops and rebuilds the entire SQLite index from disk.
func (r *Runtime) RebuildIndex() (*index.RebuildStats, error) {
	return r.Search.RebuildIndex()
}

// RefreshIndex incrementally updates the index for changed/new/deleted cards.
func (r *Runtime) RefreshIndex() (*index.RebuildStats, error) {
	return r.Search.RefreshIndex()
}

// ListCardIDsInCategory returns card IDs pinned to a project/category via the index.
func (r *Runtime) ListCardIDsInCategory(projectID, categoryID string) ([]string, error) {
	return r.Search.ListCardIDsInCategory(projectID, categoryID)
}

// ListOrphanedCardIDs returns IDs of cards that have no pins (Inbox cards).
func (r *Runtime) ListOrphanedCardIDs() ([]string, error) {
	return r.Search.ListOrphanedCardIDs()
}

// ListCardIDsByTag returns card IDs with a given tag via the index.
func (r *Runtime) ListCardIDsByTag(tag string) ([]string, error) {
	return r.Search.ListCardIDsByTag(tag)
}

// ListActivityLog returns the most-recent limit activity entries, newest first.
func (r *Runtime) ListActivityLog(limit int) ([]model.ActivityEntry, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 50
	}
	return r.repo.ListActivity(limit)
}

// RecentCard is a card summary enriched with its first-pin path, used by the inbox.
type RecentCard struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	UpdatedAt    time.Time `json:"updated_at"`
	Tags         []string  `json:"tags"`
	DueDate      string    `json:"due_date,omitempty"`
	BrandSlug    string    `json:"brand_slug,omitempty"`
	StreamSlug   string    `json:"stream_slug,omitempty"`
	ProjectSlug  string    `json:"project_slug,omitempty"`
	BrandName    string    `json:"brand_name,omitempty"`
	StreamName   string    `json:"stream_name,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	CategoryName string    `json:"category_name,omitempty"`
	Breadcrumb   string    `json:"breadcrumb,omitempty"`
}

// ListRecentlyUpdatedCards returns up to limit cards sorted by UpdatedAt descending.
// Orphaned cards (no pins) are excluded so every result has a navigable path.
func (r *Runtime) ListRecentlyUpdatedCards(limit int) ([]RecentCard, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	if limit <= 0 {
		limit = 21
	}

	all, err := r.repo.ListCards()
	if err != nil {
		return nil, err
	}

	// Pre-build a breadcrumb lookup by categoryID for fast resolution
	allCats, _ := r.ListAllCategories()
	catByID := make(map[string]CategoryPath, len(allCats))
	for _, cp := range allCats {
		catByID[cp.CategoryID] = cp
	}

	// Sort all cards newest-first by UpdatedAt
	sorted := make([]model.Card, len(all))
	copy(sorted, all)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
	})

	result := make([]RecentCard, 0, limit)
	for _, card := range sorted {
		if len(result) >= limit {
			break
		}

		// Resolve pins; skip orphaned cards
		pins, err := r.repo.GetCardPins(card.ID)
		if err != nil || len(pins) == 0 {
			continue
		}

		rc := RecentCard{
			ID:        card.ID,
			Title:     card.Title,
			Type:      card.Type,
			UpdatedAt: card.UpdatedAt,
			Tags:      card.Tags,
		}
		if card.DueDate != nil {
			rc.DueDate = card.DueDate.Format("2006-01-02")
		}

		// Enrich with first-pin path
		if cp, ok := catByID[pins[0].CategoryID]; ok {
			rc.BrandSlug = cp.BrandSlug
			rc.StreamSlug = cp.StreamSlug
			rc.ProjectSlug = cp.ProjectSlug
			rc.BrandName = cp.BrandName
			rc.StreamName = cp.StreamName
			rc.ProjectName = cp.ProjectName
			rc.CategoryName = cp.CategoryName
			rc.Breadcrumb = cp.Breadcrumb
		}

		result = append(result, rc)
	}
	return result, nil
}

// Wails-bound forwarders for LLM config, accounts, token pricing, and
// health probes. Domain logic lives in core/services/llm.
//
// loadLLMProvider / loadLLMProviderForAccount remain as App-level
// helpers because app_chat.go and app_agent.go still call them
// directly; they'll migrate to receiving the llm.Service via their
// Deps when chat and agent are extracted into services.


// --- Wails-bound forwarders ---

func (r *Runtime) GetLLMConfig() (config.LLMConfig, error)       { return r.LLM.GetConfig() }
func (r *Runtime) SetLLMConfig(c config.LLMConfig) error         { return r.LLM.SetConfig(c) }
func (r *Runtime) GetLLMAccounts() ([]config.LLMAccount, error)  { return r.LLM.GetAccounts() }
func (r *Runtime) SaveLLMAccounts(x []config.LLMAccount) error   { return r.LLM.SaveAccounts(x) }
func (r *Runtime) TestLLMAccountConnection(id string) (string, error) {
	return r.LLM.TestAccountConnection(id)
}
func (r *Runtime) IsLLMConfigured() bool               { return r.LLM.IsConfigured() }
func (r *Runtime) TestLLMConnection() (string, error)  { return r.LLM.TestConnection() }
func (r *Runtime) GetTokenPricing() (map[string]config.ModelPricing, error) {
	return r.LLM.GetPricing()
}
func (r *Runtime) SaveTokenPricing(p map[string]config.ModelPricing) error {
	return r.LLM.SavePricing(p)
}

// TestSystemNotification is wired here because the button that calls
// it sits in the LLM settings panel. It belongs in NotifyService
// conceptually but the internal/notify package already exposes it
// directly.
func (r *Runtime) TestSystemNotification() error {
	return notify.TestSystemNotification()
}

// --- Internal helpers used by chat and agent execution paths ---

// loadLLMProvider resolves the default provider.
func (r *Runtime) loadLLMProvider() (config.LLMConfig, llm.Provider, error) {
	return r.LLM.LoadProvider()
}

// loadLLMProviderForAccount resolves a provider with optional account
// ID and model override. Precedence documented on the service method.
func (r *Runtime) loadLLMProviderForAccount(accountID, modelOverride string) (config.LLMConfig, llm.Provider, error) {
	return r.LLM.LoadProviderForAccount(accountID, modelOverride)
}

// defaultModelForProvider is kept as a package-level helper because
// app_chat.go and app_agent.go reference it directly. The canonical
// implementation lives in core/services/llm.
func defaultModelForProvider(provider string) string {
	return llmsvc.DefaultModelForProvider(provider)
}

// listCardTypeIDs stays on App because it's not an LLM concern — it
// returns the schema registry IDs used by agent tool builders. When
// the catalog service is extracted it will own this.
func (r *Runtime) listCardTypeIDs() []string {
	if r.registry != nil {
		return r.registry.List()
	}
	return nil
}

// Wails-bound forwarders for MCP server management. Domain logic lives
// in core/services/mcpsvc. Registry lifecycle (reloadMCPRegistry) stays
// on App because it depends on the OS keychain secret resolver and
// repo open state — the service triggers reloads via the Deps callback.


// MCPServerView is the Wails-bound response shape. Aliased to the
// service type so frontend TS bindings remain stable.
type MCPServerView = mcpsvc.ServerView

// MCPServerViewTool is the Wails-bound tool shape. Aliased to the
// service type.
type MCPServerViewTool = mcpsvc.ServerViewTool

// ListMCPServers returns every configured server for the current repo.
func (r *Runtime) ListMCPServers() ([]MCPServerView, error) {
	return r.MCP.List()
}

// AddMCPServer appends a new server and reloads the registry.
func (r *Runtime) AddMCPServer(spec mcp.ServerSpec) error {
	return r.MCP.Add(spec)
}

// UpdateMCPServer replaces an existing server's spec in place.
func (r *Runtime) UpdateMCPServer(spec mcp.ServerSpec) error {
	return r.MCP.Update(spec)
}

// DeleteMCPServer removes a server and purges its keychain secrets.
func (r *Runtime) DeleteMCPServer(name string) error {
	return r.MCP.Delete(name, func(server, env string, err error) {
		slog.Warn("mcp delete secret failed", "server", server, "env", env, "err", err)
	})
}

// SetMCPServerSecret stores a single env var value in the OS keychain.
func (r *Runtime) SetMCPServerSecret(serverName, envVarName, value string) error {
	return r.MCP.SetSecret(serverName, envVarName, value)
}

// GetMCPServerSecretStatus reports presence of each declared secret.
func (r *Runtime) GetMCPServerSecretStatus(serverName string) (map[string]bool, error) {
	return r.MCP.SecretStatus(serverName)
}

// RestartMCPServer tears down and re-starts a single server.
func (r *Runtime) RestartMCPServer(name string) error {
	return r.MCP.Restart(name)
}

// Wails-bound forwarders for tags + labels. Domain logic lives in
// core/services/catalog.


func (r *Runtime) GetTagColors() (map[string]string, error) { return r.Catalog.GetTagColors() }

func (r *Runtime) SetTagColor(tag, color string) (map[string]string, error) {
	return r.Catalog.SetTagColor(tag, color)
}

func (r *Runtime) AssignTagColor(tag string) (map[string]string, error) {
	return r.Catalog.AssignTagColor(tag)
}

func (r *Runtime) GetProjectLabels(brandSlug, streamSlug, projectSlug string) ([]model.Label, error) {
	return r.Catalog.GetProjectLabels(brandSlug, streamSlug, projectSlug)
}

func (r *Runtime) AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color string) ([]model.Label, error) {
	return r.Catalog.AddProjectLabel(brandSlug, streamSlug, projectSlug, name, color)
}

func (r *Runtime) RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID string) ([]model.Label, error) {
	return r.Catalog.RemoveProjectLabel(brandSlug, streamSlug, projectSlug, labelID)
}

func (r *Runtime) UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color string) ([]model.Label, error) {
	return r.Catalog.UpdateProjectLabel(brandSlug, streamSlug, projectSlug, labelID, name, color)
}

func (r *Runtime) SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon string) ([]model.Label, error) {
	return r.Catalog.SetProjectLabelIcon(brandSlug, streamSlug, projectSlug, labelID, icon)
}

func (r *Runtime) UpdateCardLabels(id string, labelIDs []string) (*model.Card, error) {
	return r.Catalog.UpdateCardLabels(id, labelIDs)
}

// healTagColors forwards to the service so the repo-open hook in
// app.go doesn't need to know the service package.
func (r *Runtime) healTagColors() { r.Catalog.HealTagColors() }

// Forwarders for the LLM tool execution surface. Domain logic lives
// in core/runtime/tools after the LLM-runtime-extraction stage-1
// pass — see plan/llm-runtime-extraction-2026-04-24.md.
//
// App callers (app_chat.go, app_agent.go, app_pending.go) still reach
// the tool dispatcher through r.tools, and the entry-point names kept
// their lowercase forms locally for source-diff minimalism. The
// canonical dispatcher lives on *tools.Dispatcher.


// projectChatScope is the main-package alias for tools.ProjectChatScope
// so existing construction sites and parameter names don't churn.

// executeToolCall dispatches an LLM tool call into the card-scope
// executor and returns (result, action, pin suggestion).
func (r *Runtime) executeToolCall(cardID string, card *model.Card, tc llm.ToolCall, allCats []CategoryPath) (string, *model.ToolAction, *model.PinSuggestion) {
	return r.tools.ExecuteCard(cardID, card, tc, allCats)
}

// executeProjectToolCall dispatches a project-chat tool call.
func (r *Runtime) executeProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, *model.ToolAction) {
	return r.tools.ExecuteProject(tc, scope)
}

// stageToolCall stages a card-chat tool call as PendingEdits (suggest mode).
func (r *Runtime) stageToolCall(tc llm.ToolCall, allCats []CategoryPath) (string, []model.PendingEdit) {
	return r.tools.StageCard(tc, allCats)
}

// stageProjectToolCall stages a project-chat tool call as PendingEdits.
func (r *Runtime) stageProjectToolCall(tc llm.ToolCall, scope projectChatScope) (string, []model.PendingEdit) {
	return r.tools.StageProject(tc, scope)
}

// coerceBlockValueForBlock is the package-level coercion helper.
// Called from app_agent.go during agent tool execution; mirrored here
// as a forwarder so that file doesn't need to import tools directly.
func coerceBlockValueForBlock(b *model.Block, val any) (any, error) {
	return tools.CoerceBlockValueForBlock(b, val)
}

// Wails-bound forwarders for Trello import and project export.
// Domain logic lives in core/services/repository.


func (r *Runtime) ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode string) (*importer.Result, error) {
	return r.Repository.ImportTrelloBoard(brandSlug, streamSlug, filePath, archiveMode)
}

func (r *Runtime) ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode string) (*importer.Result, error) {
	return r.Repository.ImportTrelloBoardFromJSON(brandSlug, streamSlug, jsonContent, archiveMode)
}

// ExportProjectToFile writes a project export to the given absolute path.
// Returns the byte count written. Distinct from ExportCardTypesToFile
// which lives on the catalog service.
func (r *Runtime) ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath string) (int, error) {
	return r.Repository.ExportProjectToFile(brandSlug, streamSlug, projectSlug, filePath)
}

// Pending edits (Suggest mode) — accept/reject/apply workflow.
//
// Suggest mode stages tool calls as PendingEdits on the chat message
// rather than executing them immediately. This file owns the user-driven
// side of that flow: a user ticks which edits to apply, clicks Apply,
// and the staged tool calls get replayed through executeToolCall.
//
// The load-bearing invariant — "accepted IDs get applied in message
// order; everything else gets rejected" — is tested via
// internal/repo/pending_test.go through the SplitPendingEdits helper.
// See that package for the partition semantics in detail.
//
// Pin suggestions live alongside pending edits because a user can
// stage a pin via suggest_pin in Suggest mode; Accept/RejectPinSuggestion
// are the per-message resolutions of those.


// ApplyProjectPendingEdits applies the subset of accepted edits and rejects
// the rest, for a project chat message. Mirrors ApplyPendingEdits but uses the
// project executor so tool calls resolve against the project scope at apply
// time (which can differ from staging time if cards were moved/deleted).
func (r *Runtime) ApplyProjectPendingEdits(brandSlug, streamSlug, projectSlug, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := r.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := projectChatID(project.ID)

	acceptSet := make(map[string]bool, len(acceptIDs))
	for _, id := range acceptIDs {
		acceptSet[id] = true
	}

	cf, err := config.LoadChatFor(r.repo.Manifest.ID, chatID)
	if err != nil {
		return nil, err
	}

	// Recompute project scope at apply time. This catches cases where the
	// chat session staged edits referencing cards that no longer belong to the
	// project (moved or deleted between staging and apply), in addition to
	// the original LLM-hallucination defence.
	categories, _ := r.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	applyScope := tools.ProjectChatScope{
		BrandSlug:   brandSlug,
		StreamSlug:  streamSlug,
		ProjectSlug: projectSlug,
		CardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := r.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			applyScope.CardIDs[p.CardID] = true
		}
	}

	// Walk the target message, applying accepted edits in order and marking
	// the rest rejected. Edits run synchronously through the project executor.
	// Failures are stamped into the edit's Detail so the user can hover to see
	// them, and we collect a count to surface as a returned error after save —
	// the frontend uses that to fire a toast.
	var failures int
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.Status != "pending" {
				continue
			}
			if acceptSet[edit.ID] {
				tc := llm.ToolCall{ID: edit.ID, Name: edit.Tool, Arguments: edit.Input}
				result, _ := r.executeProjectToolCall(tc, applyScope)
				if strings.HasPrefix(result, "error:") {
					// Leave it pending so the user can retry; record the error in detail.
					cf.Messages[i].PendingEdits[j].Detail = result
					failures++
					continue
				}
				cf.Messages[i].PendingEdits[j].Status = "accepted"
			} else {
				cf.Messages[i].PendingEdits[j].Status = "rejected"
			}
		}
		break
	}

	if err := config.SaveChatFor(r.repo.Manifest.ID, cf); err != nil {
		return nil, err
	}
	// Failures are surfaced via the per-edit Detail field (which starts with
	// "error:" for failed rows). The frontend scans for those after a refresh
	// and toasts the user. We don't return a Go error here because Wails would
	// drop the cf value, and the user needs to see the updated rows so they
	// can retry the failed ones.
	_ = failures
	return cf, nil
}

// AcceptPendingEdit applies a single pending edit from Suggest mode and marks it accepted.
func (r *Runtime) AcceptPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	card, _ := r.repo.GetCard(cardID)
	allCats, _ := r.ListAllCategories()
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			tc := llm.ToolCall{ID: editID, Name: edit.Tool, Arguments: edit.Input}
			result, _, _ := r.executeToolCall(cardID, card, tc, allCats)
			if strings.HasPrefix(result, "error:") {
				return nil, fmt.Errorf("could not apply edit: %s", result)
			}
			card, _ = r.repo.GetCard(cardID) // refresh for subsequent edits in same batch
			cf.Messages[i].PendingEdits[j].Status = "accepted"
			if err := config.SaveChatFor(r.repo.Manifest.ID, cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// RejectPendingEdit dismisses a single pending edit without applying it.
func (r *Runtime) RejectPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			cf.Messages[i].PendingEdits[j].Status = "rejected"
			if err := config.SaveChatFor(r.repo.Manifest.ID, cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// ApplyPendingEdits accepts the specified edits (in order) and rejects the rest.
// This is the primary batch action for the Suggest mode UI.
func (r *Runtime) ApplyPendingEdits(cardID, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}

	// Find the message and split its pending edits via the shared
	// helper so the accept/reject partition is covered by tests in
	// internal/repo/pending_test.go.
	var toAccept, toReject []string
	for _, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		toAccept, toReject = repo.SplitPendingEdits(m.PendingEdits, acceptIDs)
		break
	}

	var firstErr error
	for _, eid := range toAccept {
		if updated, err2 := r.AcceptPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		} else if firstErr == nil {
			firstErr = err2
		}
	}
	for _, eid := range toReject {
		if updated, err2 := r.RejectPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		}
	}
	return cf, firstErr
}

// AcceptAllPendingEdits applies all pending edits on a message in order.
func (r *Runtime) AcceptAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	// Collect IDs first so we iterate a stable snapshot
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	var pendingIDs []string
	for _, m := range cf.Messages {
		if m.ID == msgID {
			for _, e := range m.PendingEdits {
				if e.Status == "pending" {
					pendingIDs = append(pendingIDs, e.ID)
				}
			}
			break
		}
	}
	for _, eid := range pendingIDs {
		if updated, err := r.AcceptPendingEdit(cardID, msgID, eid); err == nil {
			cf = updated
		}
	}
	return cf, nil
}

// RejectAllPendingEdits dismisses all pending edits on a message.
func (r *Runtime) RejectAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID == msgID {
			for j, e := range m.PendingEdits {
				if e.Status == "pending" {
					cf.Messages[i].PendingEdits[j].Status = "rejected"
				}
			}
			return cf, config.SaveChatFor(r.repo.Manifest.ID, cf)
		}
	}
	return cf, nil
}

// --- Pin suggestions ---

// AcceptPinSuggestion accepts a pending pin suggestion on a chat message and performs the pin.
func (r *Runtime) AcceptPinSuggestion(cardID, messageID string) error {
	if r.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			// Pin convention: projectID == categoryID
			if err := r.PinCard(cardID, m.PinSuggestion.CategoryID, m.PinSuggestion.CategoryID); err != nil {
				return err
			}
			cf.Messages[i].PinSuggestion.Status = "accepted"
			return config.SaveChatFor(r.repo.Manifest.ID, cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}

// RejectPinSuggestion dismisses a pending pin suggestion on a chat message.
func (r *Runtime) RejectPinSuggestion(cardID, messageID string) error {
	if r.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(r.repo.Manifest.ID, cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			cf.Messages[i].PinSuggestion.Status = "rejected"
			return config.SaveChatFor(r.repo.Manifest.ID, cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}
