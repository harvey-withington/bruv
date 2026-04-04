// Reactive app state using Svelte 5 module-level $state
import { ListCategories, GetCard, ListCardIDsInCategory, GetProjectLabels, ListCardTypes } from './api'
import type { CardTypeInfo } from './types'

// Navigation state
export const nav = $state({
  repoOpen: false,
  repoId: '',

  // Currently selected location in the hierarchy
  brandSlug: null as string | null,
  streamSlug: null as string | null,
  projectSlug: null as string | null,
  brandName: '' as string,
  streamName: '' as string,
  projectName: '' as string,

  // Inbox mode (showing orphaned cards)
  inboxMode: false,

  // Sidebar collapsed state
  sidebarCollapsed: false,
  sidebarWidth: 260,
})

// Board data — populated when a project is selected
export const board = $state({
  categories: [] as Array<{
    id: string
    name: string
    slug: string
    position: number
    accepted_types?: string[]
    cards: Array<{
      id: string
      type: string
      title: string
      tags: string[]
      due_date: string | null
      checklist_total: number
      checklist_done: number
    }>
  }>,
  loading: false,
})

// Global card types (built-in + user-defined), loaded once at startup
export const cardTypes = $state<{ list: CardTypeInfo[] }>({ list: [] })

export async function loadCardTypes() {
  try {
    cardTypes.list = await ListCardTypes() || []
  } catch {
    cardTypes.list = []
  }
}

// User display preferences
export const prefs = $state({
  typeBadgeDisplay: 'color' as 'text' | 'color' | 'hidden',
})

// Project tags — per-project tag definitions (source of truth for tag colors)
export const projectTags = $state<{ list: Array<{ id: string; name: string; color: string }> }>({ list: [] })

// Resolve a tag name to its project-level color
export function getTagColor(tagName: string): string {
  const pt = projectTags.list.find(t => t.name.toLowerCase() === tagName.toLowerCase())
  return pt?.color || 'var(--border)'
}

// Drag and drop state
export const dnd = $state<{
  dragging: null | { type: 'card'; cardId: string; fromCategoryId: string; cardType: string } | { type: 'column'; categoryId: string }
  overCategoryId: string | null
  overCardIndex: number | null
  overColumnIndex: number | null
  copyMode: boolean
}>({
  dragging: null,
  overCategoryId: null,
  overCardIndex: null,
  overColumnIndex: null,
  copyMode: false,
})

// Column settings — only one popover open at a time
export const columnSettings = $state({ openCategoryId: null as string | null })

// Global card-open request (set by bruv: link clicks, consumed by App.svelte)
export const navigate = $state<{ openCardId: string | null }>({ openCardId: null })

// Search state
export const search = $state({
  query: '',
  results: [] as Array<{
    CardID: string
    Title: string
    Type: string
    Rank: number
  }>,
  open: false,
  matchingIds: new Set<string>(),
})

// --- Board loading (single source of truth) ---

export async function loadBoard(brandSlug: string, streamSlug: string, projectSlug: string) {
  board.loading = true
  try {
    try { projectTags.list = await GetProjectLabels(brandSlug, streamSlug, projectSlug) || [] } catch { projectTags.list = [] }

    const cats = await ListCategories(brandSlug, streamSlug, projectSlug) || []
    const populated = await Promise.all(cats.map(async (cat: any) => {
      let cardIds: string[] = []
      try {
        cardIds = await ListCardIDsInCategory(cat.id, cat.id) || []
      } catch { /* no cards pinned yet */ }

      const cards = await Promise.all(cardIds.map(async (id: string) => {
        try {
          const card = await GetCard(id)
          return {
            id: card.id,
            type: card.type,
            title: card.title,
            tags: card.tags || [],
            due_date: card.due_date,
            checklist_total: card.checklist?.length || 0,
            checklist_done: card.checklist?.filter((c: any) => c.done).length || 0,
          }
        } catch { return null }
      }))

      return {
        id: cat.id,
        name: cat.name,
        slug: cat.slug,
        position: cat.position,
        accepted_types: cat.accepted_types?.length ? [...cat.accepted_types] : undefined,
        cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
      }
    }))
    board.categories = populated
  } catch {
    board.categories = []
  }
  board.loading = false
}
