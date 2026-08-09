// Per-repo metadata: card types and tag colours (global + per-project).
//
// These are the bits a card-rendering surface needs to colour itself:
//
//   - Card-type colours come from the repo's CardTypeInfo registry
//     (ListCardTypes). These are user-customisable per repo.
//   - Tag colours come from two layers: the global GetTagColors map
//     (used everywhere) and per-project tags (GetProjectLabels — the
//     internal type is `Label` but they're user-facing "tags"). When
//     a card has a project context, project tag colours win.
//
// The store loads global metadata (card types + global tag map) on
// enrolment + repo switch. Per-project tags load lazily the first
// time a view needs them — typically Project page mount or Card page
// after we resolve the card's pinned location.

import { repoRPC } from './auth'
import type { CardTypeInfo } from '@shared/types'
import type { ProjectTag } from './model'

const _state = $state<{
  cardTypes: CardTypeInfo[]
  globalTagColors: Record<string, string>
  // Per-project tags, keyed by brand/stream/project. Internal API
  // returns a `ProjectTag[]` but we treat each entry as a tag definition.
  projectTagsByKey: Record<string, ProjectTag[]>
  loaded: boolean
}>({
  cardTypes: [],
  globalTagColors: {},
  projectTagsByKey: {},
  loaded: false,
})

export const repoMeta = {
  get cardTypes() {
    return _state.cardTypes
  },
  get loaded() {
    return _state.loaded
  },

  /**
   * Resolve a tag's display colour.
   *
   * `projectKeys` is an ORDERED precedence list (ruling 2026-08-10 for
   * multi-pinned cards: the project the card was opened from wins, then
   * the primary pin) — the first project defining the tag decides its
   * colour, then the global map, then a neutral border colour. A single
   * string still works for the common one-project surfaces.
   */
  tagColor(tag: string, projectKeys?: string | string[]): string {
    if (!tag) return 'var(--border)'
    const lower = tag.toLowerCase()
    for (const key of normalizeKeys(projectKeys)) {
      const match = (_state.projectTagsByKey[key] ?? []).find((t) => t.name.toLowerCase() === lower)
      if (match?.color) return match.color
    }
    return _state.globalTagColors[tag] || _state.globalTagColors[lower] || 'var(--border)'
  },

  /**
   * Snapshot of every tag the user is likely to want to autocomplete
   * against: each project's tag definitions in precedence order, then
   * the global tag colour map's keys. De-duplicated case-insensitively
   * (first occurrence wins). Cheap to call — derived from already-
   * loaded state.
   */
  knownTags(projectKeys?: string | string[]): string[] {
    const seen = new Set<string>()
    const out: string[] = []
    const add = (name: string) => {
      const key = name.toLowerCase()
      if (seen.has(key)) return
      seen.add(key)
      out.push(name)
    }
    for (const key of normalizeKeys(projectKeys)) {
      for (const t of _state.projectTagsByKey[key] ?? []) add(t.name)
    }
    for (const name of Object.keys(_state.globalTagColors)) add(name)
    return out
  },
}

/** Build the canonical project key — same shape used by the cache. */
export function projectKey(brand: string, stream: string, project: string): string {
  return `${brand}/${stream}/${project}`
}

function normalizeKeys(keys?: string | string[]): string[] {
  if (!keys) return []
  return Array.isArray(keys) ? keys : [keys]
}

let inFlight: Promise<void> | null = null

/**
 * Load the static per-repo metadata if it isn't loaded yet. Cheap
 * no-op once loaded — surfaces that depend on the registry (CardPage
 * badge, CardTypePicker) call this on mount so a failed boot-time load
 * heals the moment the data is actually needed.
 */
export function ensureRepoMeta(): Promise<void> {
  if (_state.loaded) return Promise.resolve()
  return loadRepoMeta()
}

/**
 * Load (or refresh) the static per-repo metadata: the card-type
 * registry and the global tag colour map. Failures stay quiet (the
 * metadata is decorative — never block the app on it) but are NOT
 * cached: `loaded` latches only when both halves arrive, so
 * ensureRepoMeta / the reconnect hook retry later. The old
 * fail-open-and-latch version cached an empty registry for the whole
 * session after one bad moment at app start — grey type badges and an
 * empty type picker with no way back (Harvey, Cambodia, 2026-08-10).
 * Concurrent calls share one in-flight request.
 */
export function loadRepoMeta(): Promise<void> {
  if (inFlight) return inFlight
  inFlight = (async () => {
    const [types, colors] = await Promise.allSettled([
      repoRPC<CardTypeInfo[]>('ListCardTypes'),
      repoRPC<Record<string, string>>('GetTagColors'),
    ])
    // Store whatever DID arrive — partial success renders immediately.
    if (types.status === 'fulfilled') _state.cardTypes = types.value ?? []
    if (colors.status === 'fulfilled') _state.globalTagColors = colors.value ?? {}
    _state.loaded = types.status === 'fulfilled' && colors.status === 'fulfilled'
  })().finally(() => {
    inFlight = null
  })
  return inFlight
}

const tagsInFlight = new Set<string>()

/**
 * Lazy-load the tag definitions for a given project. Idempotent once
 * LOADED — a failure is never cached (same class as the card-type
 * registry bug, 2026-08-10: caching the failure-empty left that
 * project's tag colours grey and autocomplete empty for the whole
 * session), so the next caller — page revisit, reconnect refresh —
 * retries. Concurrent calls for one project share the fetch.
 */
export async function loadProjectTags(brand: string, stream: string, project: string): Promise<void> {
  const key = projectKey(brand, stream, project)
  if (key in _state.projectTagsByKey || tagsInFlight.has(key)) return
  tagsInFlight.add(key)
  try {
    const tags = await repoRPC<ProjectTag[]>('GetProjectLabels', [brand, stream, project])
    _state.projectTagsByKey[key] = tags ?? []
  } catch {
    // Quiet — tag colours are decorative — but NOT cached.
  } finally {
    tagsInFlight.delete(key)
  }
}

/** Drop all cached metadata — call on repo switch. */
export function resetRepoMeta(): void {
  _state.cardTypes = []
  _state.globalTagColors = {}
  _state.projectTagsByKey = {}
  _state.loaded = false
  tagsInFlight.clear()
}
