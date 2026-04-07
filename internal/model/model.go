package model

import "time"

// Manifest holds repository-level metadata stored in .bruv/manifest.json
type Manifest struct {
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
}

// ContextLevel controls how much repository context the LLM receives for a card.
type ContextLevel string

const (
	ContextIsolated ContextLevel = "isolated"
	ContextProject  ContextLevel = "project"
	ContextBrand    ContextLevel = "brand"
	ContextGlobal   ContextLevel = "global"
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
	BlockText      = "text"
	BlockChecklist = "checklist"
	BlockList      = "list"
	BlockMedia     = "media"
	BlockURL       = "url"
	BlockDivider   = "divider"

	// Legacy block types — kept for migration compatibility.
	BlockCheckbox = "checkbox"
	BlockRadio    = "radio"
	BlockSelect   = "select"
	BlockNumber   = "number"
	BlockDate     = "date"
	BlockImage    = "image"
	BlockVideo    = "video"
)

// FileAttachment is a file attached to a card (card-level, not per-block).
type FileAttachment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Mime    string `json:"mime"`
	Size    int64  `json:"size"`
	AddedAt string `json:"added_at"`
}

// Card is the atomic unit of work. Exists once in the repository, can be pinned
// to multiple Projects via Pins.
type Card struct {
	ID              string           `json:"id"`
	Type            string           `json:"type"`
	Title           string           `json:"title"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	ContextLevel    ContextLevel     `json:"context_level"`
	Fields          map[string]any   `json:"fields"`            // Deprecated: migrated to Blocks on read
	Checklist       []ChecklistItem  `json:"checklist"`         // Deprecated: migrated to Blocks on read
	Attachments     []string         `json:"attachments"`       // Deprecated: migrated to Blocks on read
	DueDate         *time.Time       `json:"due_date"`
	Tags            []string         `json:"tags"`
	Labels          []string         `json:"labels,omitempty"`  // label IDs from project's labels.json
	Blocks          []Block          `json:"blocks"`
	FileAttachments []FileAttachment `json:"file_attachments,omitempty"`
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
	Status string         `json:"status"` // "pending", "accepted", "rejected"
}

// ChatMessage is a single message in a card's chat history.
type ChatMessage struct {
	ID            string          `json:"id"`
	Role          string          `json:"role"`
	Content       string          `json:"content"`
	Timestamp     time.Time       `json:"timestamp"`
	ToolActions   []ToolAction    `json:"tool_actions,omitempty"`
	PinSuggestion *PinSuggestion  `json:"pin_suggestion,omitempty"`
	PendingEdits  []PendingEdit   `json:"pending_edits,omitempty"`
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
	LastRunAt       *time.Time  `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time  `json:"next_run_at,omitempty"`
	MaxTokensBudget int         `json:"max_tokens_budget,omitempty"` // 0 = default (50000)
}

// AgentRun records a single execution of a card's agent.
type AgentRun struct {
	ID         string       `json:"id"`
	CardID     string       `json:"card_id"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	Status     string       `json:"status"`
	Summary    string       `json:"summary,omitempty"`
	ToolCalls  []ToolAction `json:"tool_calls,omitempty"`
	Error      string       `json:"error,omitempty"`
	TokensUsed int          `json:"tokens_used,omitempty"`
}

// AgentFile is the on-disk format for cards/<card-uuid>.agent.json.
type AgentFile struct {
	CardID string      `json:"card_id"`
	Config AgentConfig `json:"config"`
	Runs   []AgentRun  `json:"runs,omitempty"`
}
