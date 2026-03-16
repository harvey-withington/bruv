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
type ChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Card is the atomic unit of work. Exists once in the repository, can be pinned
// to multiple Projects via Pins.
type Card struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Title        string          `json:"title"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ContextLevel ContextLevel    `json:"context_level"`
	Fields       map[string]any  `json:"fields"`
	Checklist    []ChecklistItem `json:"checklist"`
	Attachments  []string        `json:"attachments"`
	DueDate      *time.Time      `json:"due_date"`
	Tags         []string        `json:"tags"`
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
