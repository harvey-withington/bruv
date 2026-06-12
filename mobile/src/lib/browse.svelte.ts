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

// Switching INTO single mode enforces it immediately instead of waiting
// for the next tap — desktop parity (Sidebar.applySingleExpandCollapses
// and CardDetail's applySingleModeCollapse do the same). The page calls
// the helper for the surface it's showing, passing display order so
// "first open" matches what the user sees at the top.

/** Collapse the tree to its first open brand (display order) and that
 *  brand's first open stream. */
export function applySingleModeToTree(orderedBrandSlugs: string[]): void {
  const keepBrand = orderedBrandSlugs.find((slug) => _expandedBrands[slug])
  for (const slug of Object.keys(_expandedBrands)) {
    _expandedBrands[slug] = slug === keepBrand
  }
  let keepStreamKey: string | null = null
  if (keepBrand) {
    const streams = _streamsByBrand[keepBrand]?.items ?? []
    const keepStream = streams.find((s) => _expandedStreams[`${keepBrand}/${s.slug}`])
    if (keepStream) keepStreamKey = `${keepBrand}/${keepStream.slug}`
  }
  for (const key of Object.keys(_expandedStreams)) {
    _expandedStreams[key] = key === keepStreamKey
  }
}

/** Collapse a project's accordion to its first open category (display
 *  order). */
export function applySingleModeToCategories(projectID: string, orderedCategoryIDs: string[]): void {
  const map = browse.categoryExpansionFor(projectID)
  const keep = orderedCategoryIDs.find((id) => map[id] === true)
  const next: Record<string, boolean> = {}
  for (const id of orderedCategoryIDs) next[id] = id === keep
  _expandedCategories[projectID] = next
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

// --- Tree accordion (BrowsePage) helpers --------------------------
//
// Same single/multi-expand model as the project-page accordion but
// nested: a brand toggle affects sibling brands, a stream toggle only
// affects siblings within its parent brand. Streams in other brands
// stay where they are.

/** Toggle a brand's expansion, respecting the global accordion mode.
 *  In single mode this collapses every other brand. */
export function toggleBrandExpansion(brandSlug: string, allBrandSlugs: string[]): void {
  const isOpen = _expandedBrands[brandSlug] === true
  if (_accordionMode === 'single') {
    for (const s of allBrandSlugs) _expandedBrands[s] = false
    _expandedBrands[brandSlug] = !isOpen
  } else {
    _expandedBrands[brandSlug] = !isOpen
  }
}

/** Toggle a stream's expansion, respecting accordion mode. Single
 *  mode collapses only siblings within the same parent brand. */
export function toggleStreamExpansion(
  brandSlug: string,
  streamSlug: string,
  allStreamSlugsInBrand: string[],
): void {
  const key = `${brandSlug}/${streamSlug}`
  const isOpen = _expandedStreams[key] === true
  if (_accordionMode === 'single') {
    for (const ss of allStreamSlugsInBrand) {
      _expandedStreams[`${brandSlug}/${ss}`] = false
    }
    _expandedStreams[key] = !isOpen
  } else {
    _expandedStreams[key] = !isOpen
  }
}

/** Force-expand every brand. Streams stay where they are — expanding
 *  them all on a large vault would be visually overwhelming. */
export function expandAllBrandsTree(brandSlugs: string[]): void {
  for (const s of brandSlugs) _expandedBrands[s] = true
}

/** Force-collapse every brand. Stream state is also flipped to false
 *  — they're invisible anyway, but this stops single-mode from leaving
 *  a stream "remembered open" when the user comes back. */
export function collapseAllBrandsTree(brandSlugs: string[]): void {
  for (const s of brandSlugs) _expandedBrands[s] = false
  for (const k of Object.keys(_expandedStreams)) _expandedStreams[k] = false
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

// --- Create / rename -----------------------------------------------
//
// Thin wrappers over the Create*/Rename* RPCs. Each create call:
//   1. Picks a unique default name from the existing siblings (so we
//      don't trip the backend's "name taken" error on the first tap).
//   2. Calls Create*, returning the new entity (so the caller can
//      enter rename mode on it).
//   3. Refreshes the parent cache so the new entity appears in the
//      tree without needing the SSE listener above to round-trip.
//
// Errors propagate to the caller — the BrowsePage surfaces them as
// inline error text so the user knows the tap didn't take.

export function uniqueName(base: string, existingNames: string[]): string {
  const lower = existingNames.map((n) => n.toLowerCase())
  if (!lower.includes(base.toLowerCase())) return base
  for (let i = 2; ; i++) {
    const candidate = `${base} ${i}`
    if (!lower.includes(candidate.toLowerCase())) return candidate
  }
}

export async function createBrand(name: string): Promise<Brand> {
  const created = await repoRPC<Brand>('CreateBrand', [name])
  await loadBrands(true)
  return created
}

export async function createStream(brandSlug: string, name: string): Promise<Stream> {
  const created = await repoRPC<Stream>('CreateStream', [brandSlug, name])
  await loadStreams(brandSlug, true)
  return created
}

export async function createProject(
  brandSlug: string,
  streamSlug: string,
  name: string,
): Promise<Project> {
  const created = await repoRPC<Project>('CreateProject', [brandSlug, streamSlug, name])
  await loadProjects(brandSlug, streamSlug, true)
  return created
}

/** Rename a brand. Slug may change as a side-effect of renaming, so
 *  we also fix up expansion state (which is keyed by slug) and refresh
 *  the brand list. Returns the updated brand. */
export async function renameBrand(oldSlug: string, newName: string): Promise<Brand> {
  const updated = await repoRPC<Brand>('RenameBrand', [oldSlug, newName])
  if (updated.slug !== oldSlug) {
    if (_expandedBrands[oldSlug]) {
      _expandedBrands[updated.slug] = true
      delete _expandedBrands[oldSlug]
    }
    // Re-key any cached streams under the old brand slug.
    if (_streamsByBrand[oldSlug]) {
      _streamsByBrand[updated.slug] = _streamsByBrand[oldSlug]
      delete _streamsByBrand[oldSlug]
    }
  }
  await loadBrands(true)
  return updated
}

export async function renameStream(
  brandSlug: string,
  oldSlug: string,
  newName: string,
): Promise<Stream> {
  const updated = await repoRPC<Stream>('RenameStream', [brandSlug, oldSlug, newName])
  if (updated.slug !== oldSlug) {
    const oldKey = `${brandSlug}/${oldSlug}`
    const newKey = `${brandSlug}/${updated.slug}`
    if (_expandedStreams[oldKey]) {
      _expandedStreams[newKey] = true
      delete _expandedStreams[oldKey]
    }
    if (_projectsByStream[oldKey]) {
      _projectsByStream[newKey] = _projectsByStream[oldKey]
      delete _projectsByStream[oldKey]
    }
  }
  await loadStreams(brandSlug, true)
  return updated
}

export async function renameProject(
  brandSlug: string,
  streamSlug: string,
  oldSlug: string,
  newName: string,
): Promise<Project> {
  const updated = await repoRPC<Project>('RenameProject', [brandSlug, streamSlug, oldSlug, newName])
  await loadProjects(brandSlug, streamSlug, true)
  return updated
}

export async function deleteBrand(slug: string): Promise<void> {
  await repoRPC<void>('DeleteBrand', [slug])
  delete _expandedBrands[slug]
  delete _streamsByBrand[slug]
  // Drop any expanded-stream / cached-project entries that lived under
  // this brand, otherwise their stale keys leak into the next session.
  const prefix = `${slug}/`
  for (const k of Object.keys(_expandedStreams)) if (k.startsWith(prefix)) delete _expandedStreams[k]
  for (const k of Object.keys(_projectsByStream)) if (k.startsWith(prefix)) delete _projectsByStream[k]
  for (const k of Object.keys(_categoriesByProject)) if (k.startsWith(prefix)) delete _categoriesByProject[k]
  await loadBrands(true)
}

export async function deleteStream(brandSlug: string, streamSlug: string): Promise<void> {
  await repoRPC<void>('DeleteStream', [brandSlug, streamSlug])
  const streamKey = `${brandSlug}/${streamSlug}`
  delete _expandedStreams[streamKey]
  delete _projectsByStream[streamKey]
  const prefix = `${streamKey}/`
  for (const k of Object.keys(_categoriesByProject)) if (k.startsWith(prefix)) delete _categoriesByProject[k]
  await loadStreams(brandSlug, true)
}

export async function deleteProject(
  brandSlug: string,
  streamSlug: string,
  projectSlug: string,
): Promise<void> {
  await repoRPC<void>('DeleteProject', [brandSlug, streamSlug, projectSlug])
  delete _categoriesByProject[`${brandSlug}/${streamSlug}/${projectSlug}`]
  await loadProjects(brandSlug, streamSlug, true)
}

export async function createCategory(
  brandSlug: string,
  streamSlug: string,
  projectSlug: string,
  name: string,
  position: number,
): Promise<Category> {
  const created = await repoRPC<Category>('CreateCategory', [
    brandSlug, streamSlug, projectSlug, name, position,
  ])
  await loadCategories(brandSlug, streamSlug, projectSlug, true)
  return created
}

export async function renameCategory(
  brandSlug: string,
  streamSlug: string,
  projectSlug: string,
  oldSlug: string,
  newName: string,
): Promise<Category> {
  const updated = await repoRPC<Category>('RenameCategory', [
    brandSlug, streamSlug, projectSlug, oldSlug, newName,
  ])
  await loadCategories(brandSlug, streamSlug, projectSlug, true)
  return updated
}

export async function deleteCategory(
  brandSlug: string,
  streamSlug: string,
  projectSlug: string,
  categorySlug: string,
): Promise<void> {
  await repoRPC<void>('DeleteCategory', [brandSlug, streamSlug, projectSlug, categorySlug])
  await loadCategories(brandSlug, streamSlug, projectSlug, true)
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
