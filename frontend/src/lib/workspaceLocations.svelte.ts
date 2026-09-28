import { ListWorkspaceDir, ResolveWorkspace } from '@shared/api'
import type { WorkspaceLocation } from '@shared/types'
import { createWorkspaceDirCache, type WorkspaceDirCache } from './workspaceTree.svelte'

/**
 * Workspace id → where it lives. A Workspace Files entry carries only its
 * workspace id (so the block renders wherever the card renders), and every
 * workspace RPC wants project slugs. Resolved once per id per session and
 * shared across every block on screen; a detached workspace resolves to
 * an error state the block shows as "workspace missing".
 *
 * Reads and writes are deliberately separate: `workspaceLocation` is a
 * pure reactive read that templates and deriveds may call freely, and
 * `ensureWorkspaceLocation` is the write, called from an effect. A write
 * inside a derived (which the first version did on a cache miss) trips
 * Svelte's state_unsafe_mutation guard and halts the component — every
 * click in the block then silently dies.
 */
export type WorkspaceLocationState =
  | { status: 'loading' }
  | { status: 'ready'; location: WorkspaceLocation }
  | { status: 'error'; error: string }

const locations = $state<Record<string, WorkspaceLocationState>>({})
const dirCaches = new Map<string, WorkspaceDirCache>()
const inFlight = new Map<string, Promise<WorkspaceLocation>>()

/** Reactive read; 'loading' until `ensureWorkspaceLocation` has resolved it. */
export function workspaceLocation(workspaceId: string): WorkspaceLocationState {
  return locations[workspaceId] ?? { status: 'loading' }
}

/** Start (or reuse) the resolution for an id. Call from an effect. */
export function ensureWorkspaceLocation(workspaceId: string): void {
  if (locations[workspaceId] || inFlight.has(workspaceId)) return
  locations[workspaceId] = { status: 'loading' }
  void resolveWorkspace(workspaceId)
}

/** The location itself, resolving on demand. Async, so safe anywhere. */
export function resolveWorkspace(workspaceId: string): Promise<WorkspaceLocation> {
  const cur = locations[workspaceId]
  if (cur?.status === 'ready') return Promise.resolve(cur.location)
  let p = inFlight.get(workspaceId)
  if (!p) {
    p = ResolveWorkspace(workspaceId)
      .then((location) => {
        locations[workspaceId] = { status: 'ready', location }
        return location
      })
      .catch((e: unknown) => {
        locations[workspaceId] = { status: 'error', error: e instanceof Error ? e.message : String(e) }
        throw e
      })
      .finally(() => inFlight.delete(workspaceId))
    inFlight.set(workspaceId, p)
  }
  return p
}

/** Retry after an error, or after the user re-attached the workspace. */
export function forgetWorkspaceLocation(workspaceId: string): void {
  delete locations[workspaceId]
  dirCaches.get(workspaceId)?.clear()
}

/**
 * One lazy directory cache per workspace, shared by every block that names
 * a file in it — a card with three entries in the same folder lists that
 * folder once. The loader resolves the workspace itself, so a cache can be
 * created before the location is known.
 */
export function workspaceDirCache(workspaceId: string): WorkspaceDirCache {
  let cache = dirCaches.get(workspaceId)
  if (!cache) {
    cache = createWorkspaceDirCache(async (rel) => {
      const loc = await resolveWorkspace(workspaceId)
      return ListWorkspaceDir(loc.brand_slug, loc.stream_slug, loc.project_slug, rel)
    })
    dirCaches.set(workspaceId, cache)
  }
  return cache
}

/** Drop cached listings so a block re-reads after a structure change. */
export function refreshWorkspaceDirs(workspaceId: string): void {
  dirCaches.get(workspaceId)?.clear()
}
