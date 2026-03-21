package model

import "time"

// Manifest holds repository-level metadata stored in .bruv/manifest.json
type Manifest struct {
	Version   string    `json:"version"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Brand is the top-level container representing a coherent identity or organisation.
type Brand struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Logo         string    `json:"logo,omitempty"`
	Website      string    `json:"website,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Stream is an ongoing series or work track within a Brand.
type Stream struct {
	ID        string    `json:"id"`
	BrandID   string    `json:"brand_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Project is a discrete body of work within a Stream, analogous to a Trello board.
type Project struct {
	ID        string    `json:"id"`
	StreamID  string    `json:"stream_id"`
	BrandID   string    `json:"brand_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Category is a workflow stage within a Project, analogous to a Trello list.
type Category struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Type     string         `json:"type"`               // "text", "checklist", "checkbox", "radio", "select", "number", "date", "image", "video", "url", "divider"
	Label    string         `json:"label"`              // display label (e.g. "Description", "Recording Status")
	Key      string         `json:"key,omitempty"`      // schema field key (e.g. "recording_status"); empty for user-added blocks
	Value    any            `json:"value"`              // type-specific value
	Required bool           `json:"required,omitempty"` // from schema — advisory only
	Meta     map[string]any `json:"meta,omitempty"`     // type-specific config (enum options, format hints, etc.)
}

// Block type constants.
const (
	BlockText      = "text"
	BlockChecklist = "checklist"
	BlockCheckbox  = "checkbox"
	BlockRadio     = "radio"
	BlockSelect    = "select"
	BlockNumber    = "number"
	BlockDate      = "date"
	BlockImage     = "image"
	BlockVideo     = "video"
	BlockURL       = "url"
	BlockDivider   = "divider"
)

// Card is the atomic unit of work. Exists once in the repository, can be pinned
// to multiple Projects via Pins.
type Card struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Title        string          `json:"title"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ContextLevel ContextLevel    `json:"context_level"`
	Fields       map[string]any  `json:"fields"`      // Deprecated: migrated to Blocks on read
	Checklist    []ChecklistItem `json:"checklist"`   // Deprecated: migrated to Blocks on read
	Attachments  []string        `json:"attachments"` // Deprecated: migrated to Blocks on read
	DueDate      *time.Time      `json:"due_date"`
	Tags         []string        `json:"tags"`
	Blocks       []Block         `json:"blocks"`
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
