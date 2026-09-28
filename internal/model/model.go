package model

import "time"

// Manifest holds repository-level metadata stored in .bruv/manifest.json.
//
// ID is a stable UUID generated at repo creation time. It is used to key
// per-user data (chat history, window state, etc.) in the OS config folder
// so that personal state stays separate from the repo itself. The ID
// survives zipping, cloning, and sharing — if Alice shares her repo with
// Bob, both machines see the same ID and keep their chats keyed
// independently in their own config folders.
//
// Existing repos created before this field existed get an ID backfilled
// on first open via repo.Open().
type Manifest struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Brand is the top-level container representing a coherent identity or organisation.
type Brand struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description,omitempty"`
	Icon         string    `json:"icon,omitempty"`
	Logo         string    `json:"logo,omitempty"`
	Website      string    `json:"website,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Stream is an ongoing series or work track within a Brand.
type Stream struct {
	ID          string    `json:"id"`
	BrandID     string    `json:"brand_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Project is a discrete body of work within a Stream, analogous to a Trello board.
type Project struct {
	ID          string    `json:"id"`
	StreamID    string    `json:"stream_id"`
	BrandID     string    `json:"brand_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Category is a workflow stage within a Project, analogous to a Trello list.
type Category struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Description   string    `json:"description,omitempty"`
	Icon          string    `json:"icon,omitempty"`
	Position      int       `json:"position"`
	AcceptedTypes []string  `json:"accepted_types,omitempty"` // nil/empty = all card types accepted
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Label is a project-scoped label that can be assigned to cards.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon,omitempty"`
}

// ProjectMember represents a user who is a member of a project.
type ProjectMember struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Username string `json:"username"`
}

// ContextLevel controls how much repository context the LLM receives for a card.
type ContextLevel string

const (
	ContextIsolated ContextLevel = "isolated"
	ContextProject  ContextLevel = "project"
	ContextBrand    ContextLevel = "brand"
	ContextGlobal   ContextLevel = "global"
)

// ProjectChatContextLevel controls how much card detail is serialised into the
// Project Chat system prompt. Unlike ContextLevel (which is per-card for Card
// chat), this is a session-level setting for the project chat panel.
type ProjectChatContextLevel string

const (
	// ProjectChatContextAll includes every pinned card in every category with
	// title, type, tags, due date, and description snippet. This is the default.
	ProjectChatContextAll ProjectChatContextLevel = "all"
	// ProjectChatContextMetadata includes category names and card titles/types
	// only — no tags, descriptions, or due dates. Keeps the prompt small while
	// still giving the LLM a rough map of the board.
	ProjectChatContextMetadata ProjectChatContextLevel = "metadata"
	// ProjectChatContextNone omits card enumeration entirely. The LLM sees only
	// brand/stream/project metadata. Useful for pure-generation tasks where
	// board state is irrelevant and you want to minimise token usage.
	ProjectChatContextNone ProjectChatContextLevel = "none"
)

// ChecklistItem is a single item within a card's checklist.
// Deprecated: Use Block with Type="checklist" instead. Kept for migration compatibility.
type ChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Block is an ordered content element within a card.
// Template fields and user content live in the same list.
type Block struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`               // "text", "checklist", "list", "media", "url", "divider"
	Label    string         `json:"label"`              // display label (e.g. "Description", "Recording Status")
	Key      string         `json:"key,omitempty"`      // schema field key (e.g. "recording_status"); empty for user-added blocks
	Value    any            `json:"value"`              // type-specific value
	Required bool           `json:"required,omitempty"` // from schema — advisory only
	Meta     map[string]any `json:"meta,omitempty"`     // type-specific config (enum options, collapsed state, etc.)
}

// Block type constants.
const (
	BlockText          = "text"
	BlockChecklist     = "checklist"
	BlockList          = "list"
	BlockMedia         = "media"
	BlockURL           = "url"
	BlockDivider       = "divider"
	BlockSelect        = "select"
	BlockNumber        = "number"
	BlockDate          = "date"
	BlockRating        = "rating"
	BlockCheckbox      = "checkbox"
	BlockRadio         = "radio"
	BlockCheckboxGroup = "checkbox_group"
	BlockImage         = "image"
	BlockProgress      = "progress"
	BlockAlarm         = "alarm"
	BlockSurvey        = "survey"
	BlockSlideDeck     = "slide_deck"
	// BlockWorkspaceFiles lists workspace files/folders the card is about;
	// value is []WorkspaceFileEntry, meta.display is "tree" or "flat".
	BlockWorkspaceFiles = "workspace_files"

	// Legacy block types — kept for migration compatibility.
	BlockVideo = "video"
)

// Comment is a user- or import-authored comment on a card.
// Stored separately from the card itself in cards/<cardID>.comments.json,
// matching the on-disk layout used by chat history.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Text      string    `json:"text"`
}

// CommentFile is the on-disk format for cards/<card-uuid>.comments.json.
type CommentFile struct {
	CardID   string    `json:"card_id"`
	Comments []Comment `json:"comments"`
}

// FileAttachment is a file attached to a card (card-level, not per-block).
//
// Bytes live at <repo>/attachments/<cardID>/<id>. There is intentionally
// no on-disk Path field — paths are server-machine-local and would
// break the moment the repo is shared. Clients fetch attachment bytes
// via signed HTTP URLs (see transport/http/attachments.go and
// app.SignAttachmentURL); the server resolves <cardID>+<id> to a path
// at request time.
type FileAttachment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mime    string `json:"mime"`
	Size    int64  `json:"size"`
	AddedAt string `json:"added_at"`
}

// Card is the atomic unit of work. Exists once in the repository, can be pinned
// to multiple Projects via Pins.
//
// Description is the card's primary rich-text body (markdown, mentions,
// the lot). Intrinsic — every card has one, even if empty. Reads and
// writes go through dedicated paths (UpdateCardDescription); never
// merge it into the Block list.
//
// Blocks are the structured user-facing fields a card carries —
// checklists, ratings, dates, the per-card-type template entries.
// One Block in the data is one Field in the UI; templates pre-define
// these and switching Card Type merges them into the card.
type Card struct {
	ID              string           `json:"id"`
	Type            string           `json:"type"`
	Title           string           `json:"title"`
	Description     string           `json:"description,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	ContextLevel    ContextLevel     `json:"context_level"`
	DueDate         *time.Time       `json:"due_date"`
	Tags            []string         `json:"tags"`
	Labels          []string         `json:"labels,omitempty"`  // label IDs from project's tags.json
	Members         []string         `json:"members,omitempty"` // member IDs/usernames
	Blocks          []Block          `json:"blocks"`
	FileAttachments []FileAttachment `json:"file_attachments,omitempty"`
}

// WorkspaceFileEntry is one item of a BlockWorkspaceFiles value: a file or
// folder in a project Workspace that this card is about. Path is
// slash-relative to the workspace root and chokepoint-resolved on every
// use; the entry carries its own workspace id so the block renders (and
// opens the editor) wherever the card renders, Inbox included.
// Design: plan/2026-09-17 workspace files block.md.
type WorkspaceFileEntry struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path"`
	IsDir       bool   `json:"is_dir,omitempty"`
}

// Pin represents a card's membership in a specific Project/Category.
type Pin struct {
	CardID     string    `json:"card_id"`
	ProjectID  string    `json:"project_id"`
	CategoryID string    `json:"category_id"`
	Position   int       `json:"position"`
	PinnedAt   time.Time `json:"pinned_at"`
}

// PinFile is the on-disk format for pins/<card-uuid>/pins.json.
type PinFile struct {
	CardID string `json:"card_id"`
	Pins   []Pin  `json:"pins"`
}

// Chat role constants.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// ToolAction records a tool call the AI made and what happened.
type ToolAction struct {
	Tool   string `json:"tool"`             // tool name (set_card_type, update_blocks, add_tags, suggest_pin)
	Input  any    `json:"input"`            // arguments the AI passed
	Result string `json:"result,omitempty"` // brief outcome description
}

// PinSuggestion is a pending suggestion to pin the card to a category.
type PinSuggestion struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	Breadcrumb   string `json:"breadcrumb"`
	Reason       string `json:"reason"`
	Confidence   string `json:"confidence,omitempty"` // "high", "medium", "low"
	Status       string `json:"status"`               // "pending", "accepted", "rejected"
}

// PendingEdit is a staged LLM-proposed change awaiting user approval in Suggest mode.
type PendingEdit struct {
	ID     string         `json:"id"`
	Tool   string         `json:"tool"`
	Input  map[string]any `json:"input"`
	Label  string         `json:"label"`  // short human-readable summary
	Detail string         `json:"detail"` // longer description for hover tooltip
	Status string         `json:"status"` // "pending", "accepted", "rejected", "failed"
	// Error is why an accepted edit could not be applied (Status ==
	// "failed"). Kept apart from Detail so the row still shows what
	// was proposed and the tooltip can say why it did not happen.
	Error string `json:"error,omitempty"`
}

// PendingEditFailed is the status of an accepted edit whose tool call
// returned an error at apply time. It is terminal: the user fixes the
// cause (a card type the category refuses, say) and asks the AI again.
const PendingEditFailed = "failed"

// ChatMessage is a single message in a card's chat history.
type ChatMessage struct {
	ID            string         `json:"id"`
	Role          string         `json:"role"`
	Content       string         `json:"content"`
	Timestamp     time.Time      `json:"timestamp"`
	ToolActions   []ToolAction   `json:"tool_actions,omitempty"`
	PinSuggestion *PinSuggestion `json:"pin_suggestion,omitempty"`
	PendingEdits  []PendingEdit  `json:"pending_edits,omitempty"`
	// Bookmarked marks a message the user wants to jump back to via the
	// chat panel's bookmark navigation. Persisted with the chat file.
	Bookmarked bool `json:"bookmarked,omitempty"`
	// Route records which model answered and why, on assistant replies
	// and on provider-error messages.
	Route *RouteDecision `json:"route,omitempty"`
}

// RouteDecision records how one AI turn chose its model: the model that
// served it, the settings level the choice came from, and — when a
// router picked — how. Structured so each surface localizes the
// explanation (plan/2026-09-25 multiple models and model routing.md).
type RouteDecision struct {
	ModelID       string `json:"model_id,omitempty"` // registry id; "" for legacy / ad-hoc
	Model         string `json:"model"`              // provider model id sent on the wire
	ModelLabel    string `json:"model_label,omitempty"`
	Provider      string `json:"provider"` // provider kind
	ProviderLabel string `json:"provider_label,omitempty"`
	// Source is the settings level the choice came from: "override"
	// (this chat / this agent), "task", "default", "first" (first
	// enabled model) or "legacy" (pre-routing config).
	Source string `json:"source"`
	// Router fields, set when a router picked the model.
	RouterID   string `json:"router_id,omitempty"`
	RouterName string `json:"router_name,omitempty"`
	Via        string `json:"via,omitempty"`        // "rule" | "fallback" | "first"
	RuleIndex  int    `json:"rule_index,omitempty"` // 1-based
	RuleName   string `json:"rule_name,omitempty"`
	Band       string `json:"band,omitempty"`
	Score      *int   `json:"score,omitempty"`
	// Skipped lists higher-precedence choices that could not be used,
	// so "why isn't my chat using X" has an answer.
	Skipped []SkippedChoice `json:"skipped,omitempty"`
}

// SkippedChoice is a model choice passed over during resolution.
type SkippedChoice struct {
	Source string `json:"source"`
	Ref    string `json:"ref"`
	// Why: "missing" (deleted model/router), "disabled", "no_account",
	// "no_eligible" (router had no usable model), "router_error".
	Why string `json:"why"`
}

// ChatFile is the on-disk format for cards/<card-uuid>.messages.json.
type ChatFile struct {
	CardID   string        `json:"card_id"`
	Messages []ChatMessage `json:"messages"`
}

// AgentStatus represents the current state of a card's agent.
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusFailed   AgentStatus = "failed"
	AgentStatusDisabled AgentStatus = "disabled"
)

// AgentConfig holds the agent configuration for a card.
// Persisted separately as cards/<card-uuid>.agent.json.
type AgentConfig struct {
	Enabled       bool        `json:"enabled"`
	Goal          string      `json:"goal"`
	Schedule      string      `json:"schedule"`
	AllowedTools  []string    `json:"allowed_tools"`
	Status        AgentStatus `json:"status"`
	NotifyOn      []string    `json:"notify_on,omitempty"`
	NotifyChannel string      `json:"notify_channel,omitempty"`
	// LLM is the agent's model choice (a config.ModelRef string:
	// "model:<id>" / "router:<id>"); empty = the agent_run task's
	// assignment. LLMAccountID / LLMModel are the pre-routing pair, still
	// honoured when LLM is empty and cleared by the agent editor on save.
	LLM               string     `json:"llm,omitempty"`
	LLMAccountID      string     `json:"llm_account_id,omitempty"`
	LLMModel          string     `json:"llm_model,omitempty"`
	LastRunAt         *time.Time `json:"last_run_at,omitempty"`
	NextRunAt         *time.Time `json:"next_run_at,omitempty"`
	MaxTokensBudget   int        `json:"max_tokens_budget,omitempty"`     // 0 = default (50000)
	MaxTurns          int        `json:"max_turns,omitempty"`             // 0 = default (DefaultAgentMaxTurns); model↔tool rounds per run
	RunStartedAt      *time.Time `json:"run_started_at,omitempty"`        // set when entering running state; used for stuck detection
	MinIntervalMins   int        `json:"min_interval_minutes,omitempty"`  // 0 = default (5); minimum minutes between runs
	MaxRetries        int        `json:"max_retries,omitempty"`           // 0 = no retry
	RetryCount        int        `json:"retry_count,omitempty"`           // current consecutive failure count
	RetryBackoffMins  int        `json:"retry_backoff_minutes,omitempty"` // 0 = default (5)
	CostBudgetUSD     float64    `json:"cost_budget_usd,omitempty"`
	CostSpentUSD      float64    `json:"cost_spent_usd,omitempty"`
	StartDate         *time.Time `json:"start_date,omitempty"`
	EndDate           *time.Time `json:"end_date,omitempty"`
	ActiveWindowStart string     `json:"active_window_start,omitempty"` // "09:00" format
	ActiveWindowEnd   string     `json:"active_window_end,omitempty"`   // "17:00" format
	OneShot           bool       `json:"one_shot,omitempty"`
	Timezone          string     `json:"timezone,omitempty"` // IANA timezone, empty = local
}

// Agent run turn limits: each turn is one model response plus the tool
// calls it made. The old fixed limit of 10 ended multi-step goals
// before their report step.
const (
	DefaultAgentMaxTurns = 25
	MaxAgentMaxTurns     = 100
)

// AgentRun records a single execution of a card's agent.
type AgentRun struct {
	ID           string       `json:"id"`
	CardID       string       `json:"card_id"`
	StartedAt    time.Time    `json:"started_at"`
	FinishedAt   *time.Time   `json:"finished_at,omitempty"`
	Status       string       `json:"status"`
	Summary      string       `json:"summary,omitempty"`
	ToolCalls    []ToolAction `json:"tool_calls,omitempty"`
	Error        string       `json:"error,omitempty"`
	TokensUsed   int          `json:"tokens_used,omitempty"`
	ModelUsed    string       `json:"model_used,omitempty"`
	ProviderUsed string       `json:"provider_used,omitempty"`
	// Route is the full model decision; ModelUsed/ProviderUsed stay for
	// cost roll-ups and runs recorded before routing existed.
	Route *RouteDecision `json:"route,omitempty"`
}

// AgentFile is the on-disk format for cards/<card-uuid>.agent.json.
type AgentFile struct {
	CardID string      `json:"card_id"`
	Config AgentConfig `json:"config"`
	Runs   []AgentRun  `json:"runs,omitempty"`
}
