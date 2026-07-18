// Mobile-side data model. Mirrors the Go structs in
// internal/model/model.go for the entities the browse + card surfaces
// touch.
//
// The desktop's shared/types.ts uses `Promise<any>` for these API
// returns (legacy reasons). Mobile defines the shapes properly so we
// stay CLAUDE.md-compliant and get type help in components.

export type Brand = {
  id: string
  name: string
  slug: string
  description?: string
  icon?: string
  logo?: string
  website?: string
  position: number
  created_at: string
  updated_at: string
}

export type Stream = {
  id: string
  brand_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  created_at: string
  updated_at: string
}

export type Project = {
  id: string
  stream_id: string
  brand_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  created_at: string
  updated_at: string
}

export type Category = {
  id: string
  project_id: string
  name: string
  slug: string
  description?: string
  icon?: string
  position: number
  accepted_types?: string[]
  created_at: string
  updated_at: string
}

// Lightweight card shape used by inbox + project listings. The full
// card with blocks lives in `Card` (defined when card-detail lands in
// step 7); this is the minimum to render a tile.
export type CardSummary = {
  id: string
  type: string
  title: string
  tags: string[]
  due_date?: string
  updated_at: string
}

// Per-project tag definition. The Go side calls these "Label"
// internally — we present them as tags everywhere user-facing.
// Returned by GetProjectLabels(brand, stream, project).
export type ProjectTag = {
  id: string
  name: string
  color: string
  icon?: string
}

// Result of PromoteCardToProject — the newly created project (with its
// default category) that the card was pinned into.
export type PromotedProject = {
  project: Project
  category: Category
}
