import type { WorkspaceEntry, WorkspaceLocation } from '@shared/types'
import { repoRPC } from './auth'

/**
 * Workspace id → where it lives, resolved once per session and shared by
 * every Workspace Files block on screen (mobile twin of the desktop's
 * lib/workspaceLocations.svelte.ts). A detached workspace resolves to an
 * error the block shows plainly.
 *
 * `workspaceLocation` is a pure reactive read (safe in deriveds and
 * templates); `ensureWorkspaceLocation` is the write, called from an
 * effect. Writing state inside a derived trips Svelte's
 * state_unsafe_mutation guard and halts the component.
 */
export type WorkspaceLocationState =
  | { status: 'loading' }
  | { status: 'ready'; location: WorkspaceLocation }
  | { status: 'error'; error: string }

const locations = $state<Record<string, WorkspaceLocationState>>({})
const inFlight = new Map<string, Promise<WorkspaceLocation>>()

export function workspaceLocation(workspaceId: string): WorkspaceLocationState {
  return locations[workspaceId] ?? { status: 'loading' }
}

export function ensureWorkspaceLocation(workspaceId: string): void {
  if (locations[workspaceId] || inFlight.has(workspaceId)) return
  locations[workspaceId] = { status: 'loading' }
  void resolveWorkspace(workspaceId).catch(() => { /* recorded in state */ })
}

export function resolveWorkspace(workspaceId: string): Promise<WorkspaceLocation> {
  const cur = locations[workspaceId]
  if (cur?.status === 'ready') return Promise.resolve(cur.location)
  let p = inFlight.get(workspaceId)
  if (!p) {
    p = repoRPC<WorkspaceLocation>('ResolveWorkspace', [workspaceId])
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

/** One directory's children, resolved through the workspace id. */
export async function listWorkspaceDir(workspaceId: string, rel: string): Promise<WorkspaceEntry[]> {
  const loc = await resolveWorkspace(workspaceId)
  return (await repoRPC<WorkspaceEntry[]>('ListWorkspaceDir', [loc.brand_slug, loc.stream_slug, loc.project_slug, rel])) ?? []
}
