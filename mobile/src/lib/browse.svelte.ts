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
}

export async function loadBrands(force = false): Promise<void> {
  if (!force && (_brands.state === 'loading' || _brands.state === 'loaded')) return
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
  _streamsByBrand[brandSlug] = { items: [], state: 'loading', error: null }
  try {
    const items = (await repoRPC<Stream[]>('ListStreams', [brandSlug])) ?? []
    _streamsByBrand[brandSlug] = { items, state: 'loaded', error: null }
  } catch (err) {
    _streamsByBrand[brandSlug] = {
      items: [],
      state: 'error',
      error: err instanceof Error ? err.message : String(err),
    }
  }
}

export async function loadProjects(brandSlug: string, streamSlug: string, force = false): Promise<void> {
  const key = streamKey(brandSlug, streamSlug)
  const existing = _projectsByStream[key]
  if (!force && existing && (existing.state === 'loading' || existing.state === 'loaded')) return
  _projectsByStream[key] = { items: [], state: 'loading', error: null }
  try {
    const items = (await repoRPC<Project[]>('ListProjects', [brandSlug, streamSlug])) ?? []
    _projectsByStream[key] = { items, state: 'loaded', error: null }
  } catch (err) {
    _projectsByStream[key] = {
      items: [],
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
}

function streamKey(brandSlug: string, streamSlug: string): string {
  return `${brandSlug}/${streamSlug}`
}

function projectKeyStr(brandSlug: string, streamSlug: string, projectSlug: string): string {
  return `${brandSlug}/${streamSlug}/${projectSlug}`
}
