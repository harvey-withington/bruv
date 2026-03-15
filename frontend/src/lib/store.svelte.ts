// Reactive app state using Svelte 5 module-level $state

// Navigation state
export const nav = $state({
  repoOpen: false,
  repoPath: '',

  // Currently selected location in the hierarchy
  brandSlug: null as string | null,
  streamSlug: null as string | null,
  projectSlug: null as string | null,

  // Sidebar collapsed state
  sidebarCollapsed: false,
})

// Board data — populated when a project is selected
export const board = $state({
  categories: [] as Array<{
    id: string
    name: string
    slug: string
    position: number
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
})
