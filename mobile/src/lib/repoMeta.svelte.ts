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
   * When `projectKey` is provided, the project's tag definitions win
   * over the global map (matching the desktop's precedence). Falls
   * back to a neutral border colour when the tag is unconfigured.
   */
  tagColor(tag: string, projectKey?: string): string {
    if (!tag) return 'var(--border)'
    const lower = tag.toLowerCase()
    if (projectKey) {
      const projectTags = _state.projectTagsByKey[projectKey] ?? []
      const match = projectTags.find((t) => t.name.toLowerCase() === lower)
      if (match?.color) return match.color
    }
    return _state.globalTagColors[tag] || _state.globalTagColors[lower] || 'var(--border)'
  },

  /**
   * Snapshot of every tag the user is likely to want to autocomplete
   * against: the project's tag definitions (winning) plus the global
   * tag colour map's keys (falling back). De-duplicated case-
   * insensitively. Cheap to call — derived from already-loaded state.
   */
  knownTags(projectKey?: string): string[] {
    const seen = new Set<string>()
    const out: string[] = []
    if (projectKey) {
      const projectTags = _state.projectTagsByKey[projectKey] ?? []
      for (const t of projectTags) {
        const key = t.name.toLowerCase()
        if (seen.has(key)) continue
        seen.add(key)
        out.push(t.name)
      }
    }
    for (const name of Object.keys(_state.globalTagColors)) {
      const key = name.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push(name)
    }
    return out
  },
}

/** Build the canonical project key — same shape used by the cache. */
export function projectKey(brand: string, stream: string, project: string): string {
  return `${brand}/${stream}/${project}`
}

/**
 * Load (or refresh) the static per-repo metadata: the card-type
 * registry and the global tag colour map. Both fail open — quiet
 * defaults keep the app usable even if either RPC errors.
 */
export async function loadRepoMeta(): Promise<void> {
  try {
    const [cardTypes, globalTagColors] = await Promise.all([
      repoRPC<CardTypeInfo[]>('ListCardTypes').catch(() => [] as CardTypeInfo[]),
      repoRPC<Record<string, string>>('GetTagColors').catch(() => ({})),
    ])
    _state.cardTypes = cardTypes ?? []
    _state.globalTagColors = globalTagColors ?? {}
    _state.loaded = true
  } catch {
    // Static metadata is decorative — never block the app on it.
  }
}

/**
 * Lazy-load the tag definitions for a given project. Idempotent — a
 * second call for the same project is a no-op until `resetRepoMeta`.
 */
export async function loadProjectTags(brand: string, stream: string, project: string): Promise<void> {
  const key = projectKey(brand, stream, project)
  if (key in _state.projectTagsByKey) return
  try {
    const tags = await repoRPC<ProjectTag[]>('GetProjectLabels', [brand, stream, project])
    _state.projectTagsByKey[key] = tags ?? []
  } catch {
    _state.projectTagsByKey[key] = []
  }
}

/** Drop all cached metadata — call on repo switch. */
export function resetRepoMeta(): void {
  _state.cardTypes = []
  _state.globalTagColors = {}
  _state.projectTagsByKey = {}
  _state.loaded = false
}
