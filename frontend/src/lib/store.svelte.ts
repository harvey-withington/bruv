// Reactive app state using Svelte 5 module-level $state
import { ListCategories, GetCard, ListCardIDsInCategory, GetProjectLabels, ListCardTypes, GetTagColors, ListAgentCardIDs } from './api'
import { EventsOn } from '../../wailsjs/runtime/runtime'
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

  // Agents page mode
  agentsMode: false,

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
  agentCardIds: {} as Record<string, boolean>,
  runningAgentIds: {} as Record<string, boolean>,
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

// Global tag colors — repo-wide map loaded on repo open, used as fallback when no project is active
export const globalTagColors = $state<{ map: Record<string, string> }>({ map: {} })

// Resolve a tag name to its color.
// Prefers the active project's label color; falls back to the global tags.json map.
export function getTagColor(tagName: string): string {
  const lower = tagName.toLowerCase()
  const pt = projectTags.list.find(t => t.name.toLowerCase() === lower)
  if (pt?.color) return pt.color
  return globalTagColors.map[tagName] || globalTagColors.map[lower] || 'var(--border)'
}

// Load (or refresh) the global tag color map from the backend.
export async function loadGlobalTagColors() {
  try {
    globalTagColors.map = await GetTagColors() || {}
  } catch { /* ignore — map stays as-is */ }
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

// Inbox search filter fields — which fields the search query is matched against
export const inboxSearchFilters = $state({
  title: true,
  type: true,
  tags: true,
  actor: true,
  project: true,
})

// Board search filter fields — client-side filtering on loaded board cards
export const boardSearchFilters = $state({
  title: true,
  type: true,
  tags: true,
})

// Inbox search state (activity feed, orphaned cards)
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

// Board search state — independent from inbox, client-side filtering only
export const boardSearch = $state({
  query: '',
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
        description: cat.description || '',
        position: cat.position,
        accepted_types: cat.accepted_types?.length ? [...cat.accepted_types] : undefined,
        cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
      }
    }))
    // Load agent card IDs before setting categories so indicators are ready when cards render
    try {
      const ids = await ListAgentCardIDs() || []
      const map: Record<string, boolean> = {}
      for (const id of ids) map[id] = true
      board.agentCardIds = map
    } catch { board.agentCardIds = {} }
    board.categories = populated
  } catch {
    board.categories = []
  }
  board.loading = false
}

// Set up Wails event listeners for agent running state.
// Returns cleanup function. Call once from App.svelte onMount.
export function setupAgentEventListeners(): () => void {
  const unsub1 = EventsOn('agent:started', (data: any) => {
    if (data?.cardID) {
      board.runningAgentIds = { ...board.runningAgentIds, [data.cardID]: true }
    }
  })
  const unsub2 = EventsOn('agent:completed', (data: any) => {
    if (data?.cardID) {
      const { [data.cardID]: _, ...rest } = board.runningAgentIds
      board.runningAgentIds = rest
    }
  })
  const unsub3 = EventsOn('agent:failed', (data: any) => {
    if (data?.cardID) {
      const { [data.cardID]: _, ...rest } = board.runningAgentIds
      board.runningAgentIds = rest
    }
  })
  return () => { unsub1(); unsub2(); unsub3() }
}
