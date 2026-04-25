// Reactive app state using Svelte 5 module-level $state
import { ListCategories, GetCard, ListCardIDsInCategory, GetProjectLabels, ListCardTypes, GetTagColors, ListAgentCardStates } from './api'
import { onEvent } from './events'
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
    description?: string
    icon?: string
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
  // cardID → enabled. Present for every card with an agent
  // configuration on disk; absent for cards that have never been
  // configured. Lets the UI distinguish "no agent" from "agent
  // configured but disabled".
  agentCardStates: {} as Record<string, boolean>,
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
export const projectTags = $state<{ list: Array<{ id: string; name: string; color: string; icon?: string }> }>({ list: [] })

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

// Resolve a tag name to its icon (project tag icon only — no global fallback).
// Returns '' when the tag has no icon assigned.
export function getTagIcon(tagName: string): string {
  const lower = tagName.toLowerCase()
  const pt = projectTags.list.find(t => t.name.toLowerCase() === lower)
  return pt?.icon || ''
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

// loadBoard fetches a project's categories and cards and replaces
// board.categories atomically. Pass { silent: true } for refreshes
// triggered by in-place edits (card mutation, tag change) so the
// existing cards stay visible while new data loads — otherwise the
// user sees a black/"Loading…" flash over whatever was just edited,
// which is jarring for something as minor as a checklist toggle.
// The loading state is still used on genuine project switches and
// the first load where there's nothing to show yet.
export async function loadBoard(brandSlug: string, streamSlug: string, projectSlug: string, opts: { silent?: boolean } = {}) {
  if (!opts.silent) {
    board.loading = true
  }
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
        icon: cat.icon || '',
        position: cat.position,
        accepted_types: cat.accepted_types?.length ? [...cat.accepted_types] : undefined,
        cards: cards.filter((c): c is NonNullable<typeof c> => c !== null),
      }
    }))
    // Load agent card states before setting categories so indicators are ready when cards render
    try {
      board.agentCardStates = (await ListAgentCardStates()) || {}
    } catch { board.agentCardStates = {} }
    board.categories = populated
  } catch {
    board.categories = []
  }
  if (!opts.silent) {
    board.loading = false
  }
}

// Set up Wails event listeners for agent running state.
// Returns cleanup function. Call once from App.svelte onMount.
export function setupAgentEventListeners(): () => void {
  const unsub1 = onEvent<{ cardID?: string }>('agent:started', (data) => {
    if (data?.cardID) {
      board.runningAgentIds = { ...board.runningAgentIds, [data.cardID]: true }
    }
  })
  const unsub2 = onEvent<{ cardID?: string }>('agent:completed', (data) => {
    if (data?.cardID) {
      const { [data.cardID]: _, ...rest } = board.runningAgentIds
      board.runningAgentIds = rest
    }
  })
  const unsub3 = onEvent<{ cardID?: string }>('agent:failed', (data) => {
    if (data?.cardID) {
      const { [data.cardID]: _, ...rest } = board.runningAgentIds
      board.runningAgentIds = rest
    }
  })
  return () => { unsub1(); unsub2(); unsub3() }
}
