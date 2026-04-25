package model

import "time"

// ActivityEntry records a single user or LLM action on a card.
//
// ActorID is the stable identifier (UserProfile.UserID for human
// actors, the model name for LLM actors). It picks the per-actor
// shard file the entry is appended to so concurrent writers from
// different machines never touch the same file. Actor stays as a
// snapshot of the display name at write time — historic entries
// keep their author label even if the user later renames themself.
type ActivityEntry struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	ActorID      string    `json:"actor_id,omitempty"` // stable identity (UserID for users, model name for LLMs)
	Actor        string    `json:"actor"`              // display name snapshot at write time
	ActorType    string    `json:"actor_type"`         // "user" | "llm"
	Action       string    `json:"action"`             // see action constants below
	Field        string    `json:"field,omitempty"`    // human label of the field changed (for updated_field)
	CardID       string    `json:"card_id"`
	CardTitle    string    `json:"card_title"`
	BrandSlug    string    `json:"brand_slug,omitempty"`
	StreamSlug   string    `json:"stream_slug,omitempty"`
	ProjectSlug  string    `json:"project_slug,omitempty"`
	BrandName    string    `json:"brand_name,omitempty"`
	StreamName   string    `json:"stream_name,omitempty"`
	ProjectName  string    `json:"project_name,omitempty"`
	CategoryName string    `json:"category_name,omitempty"`
}

// Activity action constants.
const (
	ActivityCreated      = "created"
	ActivityDeleted      = "deleted"
	ActivityUpdatedTitle = "updated_title"
	ActivityUpdatedType  = "updated_type"
	ActivityUpdatedField = "updated_field"
	ActivityUpdatedTags  = "updated_tags"
	ActivityUpdatedDate  = "updated_due_date"
	ActivityPinned       = "pinned"
	ActivityUnpinned     = "unpinned"
)
