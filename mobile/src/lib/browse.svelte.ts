// Lazy cache for the browse hierarchy. Brands load eagerly when the
// browse page mounts; streams and projects load on demand when the
// user expands a row, then stick around so navigating away and back
// is instant.
//
// Mobile's browse model is intentionally lighter than the desktop's
// store.svelte.ts — no derived board, no card-type registry, no
// search indexes, no drag state. Just three keyed maps and a couple
// of small helpers.

import { repoRPC } from './auth'
import { onEvent } from './events.svelte'
import type { Brand, Stream, Project, Category } from './model'

type LoadState = 'idle' | 'loading' | 'loaded' | 'error'

const _brands = $state<{ items: Brand[]; state: LoadState; error: string | null }>({
  items: [],
  state: 'idle',
  error: null,
})

// Streams keyed by brand slug. One entry per brand the user has expanded.
const _streamsByBrand = $state<Record<string, { items: Stream[]; state: LoadState; error: string | null }>>({})

// Projects keyed by `${brandSlug}/${streamSlug}`. One entry per stream
// the user has expanded.
const _projectsByStream = $state<Record<string, { items: Project[]; state: LoadState; error: string | null }>>({})

// Categories keyed by `${brandSlug}/${streamSlug}/${projectSlug}`.
// Loaded on demand by the pin picker (and any future view that needs
// project structure without loading every card).
const _categoriesByProject = $state<Record<string, { items: Category[]; state: LoadState; error: string | null }>>({})

// Expanded brand / stream rows in the BrowsePage tree. Lifted out of
// the page component so navigating into a project and back doesn't
// collapse the user's expansions. Keyed the same way the load caches
// are: brand by slug, stream by `${brandSlug}/${streamSlug}`.
const _expandedBrands = $state<Record<string, boolean>>({})
const _expandedStreams = $state<Record<string, boolean>>({})

// Per-project category expansion for the ProjectPage accordion. Outer
// key = project ID, inner key = category ID, value = boolean.
//   - Single-expand mode: opening a category sweeps all others to
//     false in the same project, then sets the tapped one to true (or
//     toggles to false if it was already open).
//   - Multi-expand mode: tapping a category just toggles its own
//     boolean — others are unaffected.
// Persistence is in-memory only for now (matches the brand/stream tree).
// Promote to localStorage if it turns out users notice losing this on
// app reload.
const _expandedCategories = $state<Record<string, Record<string, boolean>>>({})

// Accordion mode preference. Global, persisted to localStorage —
// "I prefer single-expand" is a personal taste, not a per-project
// setting. Default 'single' because the whole point of the accordion
// on mobile is small-screen-fit.
export type AccordionMode = 'single' | 'multi'
const ACCORDION_MODE_KEY = 'bruv:accordionMode'
function readAccordionMode(): AccordionMode {
  const v = typeof localStorage !== 'undefined' ? localStorage.getItem(ACCORDION_MODE_KEY) : null
  return v === 'multi' ? 'multi' : 'single'
}
let _accordionMode = $state<AccordionMode>(readAccordionMode())

export const browse = {
  get brands() {
    return _brands
  },
  streamsFor(brandSlug: string) {
    return _streamsByBrand[brandSlug]
  },
  projectsFor(brandSlug: string, streamSlug: string) {
    return _projectsByStream[streamKey(brandSlug, streamSlug)]
  },
  categoriesFor(brandSlug: string, streamSlug: string, projectSlug: string) {
    return _categoriesByProject[projectKeyStr(brandSlug, streamSlug, projectSlug)]
  },
  get expandedBrands() {
    return _expandedBrands
  },
  get expandedStreams() {
    return _expandedStreams
  },
  get accordionMode(): AccordionMode {
    return _accordionMode
  },
  /** Returns the per-category expansion map for a project, creating
   *  one if absent. Mutating the returned object updates the store. */
  categoryExpansionFor(projectID: string): Record<string, boolean> {
    if (!_expandedCategories[projectID]) {
      _expandedCategories[projectID] = {}
    }
    return _expandedCategories[projectID]
  },
}

/** Persist the accordion mode preference to localStorage and update
 *  the reactive state. */
export function setAccordionMode(mode: AccordionMode): void {
  _accordionMode = mode
  try {
    localStorage.setItem(ACCORDION_MODE_KEY, mode)
  } catch {
    /* private mode / storage full — non-fatal */
  }
}

/** Toggle a single category's expansion, respecting the current mode.
 *  In single-expand mode this collapses every other category in the
 *  same project; in multi-expand it just flips this one. */
export function toggleCategoryExpansion(
  projectID: string,
  categoryID: string,
  allCategoryIDs: string[],
): void {
  const map = browse.categoryExpansionFor(projectID)
  const isOpen = map[categoryID] === true
  if (_accordionMode === 'single') {
    // Replace the whole map so the inner $state proxy emits one
    // update instead of N — fewer re-renders, smoother on slower
    // phones with many categories.
    const next: Record<string, boolean> = {}
    for (const id of allCategoryIDs) next[id] = false
    next[categoryID] = !isOpen
    _expandedCategories[projectID] = next
  } else {
    map[categoryID] = !isOpen
  }
}

/** Force-expand every category in a project. */
export function expandAllCategories(projectID: string, categoryIDs: string[]): void {
  const next: Record<string, boolean> = {}
  for (const id of categoryIDs) next[id] = true
  _expandedCategories[projectID] = next
}

/** Force-collapse every category in a project. */
export function collapseAllCategories(projectID: string, categoryIDs: string[]): void {
  const next: Record<string, boolean> = {}
  for (const id of categoryIDs) next[id] = false
  _expandedCategories[projectID] = next
}

/** First-visit default: expand the first non-empty category if no
 *  expansion state exists yet for this project. No-op if the user has
 *  already touched the accordion (any state present). */
export function ensureInitialExpansion(
  projectID: string,
  categories: { id: string; cardCount: number }[],
): void {
  const map = browse.categoryExpansionFor(projectID)
  if (Object.keys(map).length > 0) return
  const firstNonEmpty = categories.find((c) => c.cardCount > 0)
  if (firstNonEmpty) {
    map[firstNonEmpty.id] = true
  } else if (categories.length > 0) {
    // Project has categories but no cards anywhere — open the first
    // one so the empty-state hint is visible without the user having
    // to fish for it.
    map[categories[0].id] = true
  }
}

export async function loadBrands(force = false): Promise<void> {
  if (!force && (_brands.state === 'loading' || _brands.state === 'loaded')) return
  // Silent refresh: keep existing items while refetching so the UI
  // doesn't flash empty during a force-reload (typical after a DnD
  // drop or an SSE-driven invalidation). State flag still flips to
  // 'loading' for any spinner that wants it; items stay put.
  _brands.state = 'loading'
  _brands.error = null
  try {
    const items = (await repoRPC<Brand[]>('ListBrands')) ?? []
    _brands.items = items
    _brands.state = 'loaded'
  } catch (err) {
    _brands.error = err instanceof Error ? err.message : String(err)
    _brands.state = 'error'
  }
}

export async function loadStreams(brandSlug: string, force = false): Promise<void> {
  const existing = _streamsByBrand[brandSlug]
  if (!force && existing && (existing.state === 'loading' || existing.state === 'loaded')) return
  // First load: nothing to keep. Force refresh: keep existing items
  // (silent refresh — see loadBrands).
  if (existing && (existing.state === 'loaded' || existing.state === 'error')) {
    existing.state = 'loading'
    existing.error = null
  } else {
    _streamsByBrand[brandSlug] = { items: [], state: 'loading', error: null }
  }
  try {
    const items = (await repoRPC<Stream[]>('ListStreams', [brandSlug])) ?? []
    _streamsByBrand[brandSlug] = { items, state: 'loaded', error: null }
  } catch (err) {
    _streamsByBrand[brandSlug] = {
      items: existing?.items ?? [],
      state: 'error',
      error: err instanceof Error ? err.message : String(err),
    }
  }
}

export async function loadProjects(brandSlug: string, streamSlug: string, force = false): Promise<void> {
  const key = streamKey(brandSlug, streamSlug)
  const existing = _projectsByStream[key]
  if (!force && existing && (existing.state === 'loading' || existing.state === 'loaded')) return
  if (existing && (existing.state === 'loaded' || existing.state === 'error')) {
    existing.state = 'loading'
    existing.error = null
  } else {
    _projectsByStream[key] = { items: [], state: 'loading', error: null }
  }
  try {
    const items = (await repoRPC<Project[]>('ListProjects', [brandSlug, streamSlug])) ?? []
    _projectsByStream[key] = { items, state: 'loaded', error: null }
  } catch (err) {
    _projectsByStream[key] = {
      items: existing?.items ?? [],
      state: 'error',
      error: err instanceof Error ? err.message : String(err),
    }
  }
}

export async function loadCategories(
  brandSlug: string,
  streamSlug: string,
  projectSlug: string,
  force = false,
): Promise<void> {
  const key = projectKeyStr(brandSlug, streamSlug, projectSlug)
  const existing = _categoriesByProject[key]
  if (!force && existing && (existing.state === 'loading' || existing.state === 'loaded')) return
  _categoriesByProject[key] = { items: [], state: 'loading', error: null }
  try {
    const items = (await repoRPC<Category[]>('ListCategories', [brandSlug, streamSlug, projectSlug])) ?? []
    _categoriesByProject[key] = { items, state: 'loaded', error: null }
  } catch (err) {
    _categoriesByProject[key] = {
      items: [],
      state: 'error',
      error: err instanceof Error ? err.message : String(err),
    }
  }
}

/**
 * Drop every cached level so the next browse-page mount refetches.
 * Called when the user switches repos so they don't see the previous
 * vault's tree.
 */
export function resetBrowseCache(): void {
  _brands.items = []
  _brands.state = 'idle'
  _brands.error = null
  for (const k of Object.keys(_streamsByBrand)) delete _streamsByBrand[k]
  for (const k of Object.keys(_projectsByStream)) delete _projectsByStream[k]
  for (const k of Object.keys(_categoriesByProject)) delete _categoriesByProject[k]
  for (const k of Object.keys(_expandedBrands)) delete _expandedBrands[k]
  for (const k of Object.keys(_expandedStreams)) delete _expandedStreams[k]
  for (const k of Object.keys(_expandedCategories)) delete _expandedCategories[k]
}

function streamKey(brandSlug: string, streamSlug: string): string {
  return `${brandSlug}/${streamSlug}`
}

function projectKeyStr(brandSlug: string, streamSlug: string, projectSlug: string): string {
  return `${brandSlug}/${streamSlug}/${projectSlug}`
}

// Always-on SSE listener: when the backend publishes a brand / stream
// / project / category mutation, refresh the affected cache level so
// every page that draws from the browse store reflects the change
// without needing a manual refresh. Card-specific events are handled
// per-page (CardPage refetches its card). Subscribing once at module
// load is cheap — onEvent is a no-op until startEvents() attaches
// the EventSource, and the listener stays put across the app's life.
onEvent((ev) => {
  switch (ev.topic) {
    case 'brand:updated':
    case 'brand:deleted':
      void loadBrands(true)
      return
    case 'stream:updated':
    case 'stream:deleted': {
      const brand = readSlug(ev.payload, 'brandSlug', 'brand_id', 'brandID')
      if (brand) void loadStreams(brand, true)
      else void loadBrands(true)
      return
    }
    case 'project:updated':
    case 'project:deleted': {
      const brand = readSlug(ev.payload, 'brandSlug', 'brand_id', 'brandID')
      const stream = readSlug(ev.payload, 'streamSlug', 'stream_id', 'streamID')
      if (brand && stream) void loadProjects(brand, stream, true)
      else void loadBrands(true)
      return
    }
    case 'category:updated':
    case 'category:deleted': {
      const brand = readSlug(ev.payload, 'brandSlug')
      const stream = readSlug(ev.payload, 'streamSlug')
      const project = readSlug(ev.payload, 'projectSlug')
      if (brand && stream && project) void loadCategories(brand, stream, project, true)
      return
    }
  }
})

/** Pull a slug-shaped string out of an event payload, trying multiple
 *  field names because the backend sometimes emits the domain object
 *  (brand.slug) and sometimes a {slug} map. */
function readSlug(payload: Record<string, unknown>, ...keys: string[]): string | null {
  for (const k of keys) {
    const v = payload[k]
    if (typeof v === 'string' && v !== '') return v
  }
  return null
}
