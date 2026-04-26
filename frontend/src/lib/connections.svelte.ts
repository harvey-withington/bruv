// Reactive store for the per-machine known-connections list.
//
// The store mirrors the backend ConnectionStore plus a `ready` flag
// for components that want to wait for the first load before rendering.
// Mutations (add / remove / setActive) call the backend, refresh the
// store from the response, and — for setActive — trigger a full
// window reload so the cloud adapter re-resolves transport against
// the new active connection (same dance as switching repos).
//
// Since the local-as-remote pivot, "Local" is a normal entry in the
// store with the well-known ID "local" — there is no implicit empty-
// string sentinel anymore. isLocalActive() checks ID equality.

import { ListConnections, AddConnection, RemoveConnection, UpdateConnection, SetActiveConnection } from './api'
import type { Connection, ConnectionStore } from './types'

// Connection management is per-machine local state and must stay
// reachable when the active connection's backend isn't (an
// unreachable remote would otherwise lock the user out of their
// own connections list — circular dependency: you'd need a
// working backend to switch backends). The Wails Shell binds these
// methods directly so they can bypass the cloud adapter / RPC.
//
// Browser mode (no Wails Shell) falls back to the cloud adapter,
// which is fine because there's no "active connection" concept in
// the browser — the URL the page was loaded from IS the only
// backend, and the backend's connection list belongs to it.
type ShellConnections = {
  ListConnections?: () => Promise<ConnectionStore>
  AddConnection?: (name: string, url: string, token: string) => Promise<Connection>
  RemoveConnection?: (id: string) => Promise<void>
  UpdateConnection?: (id: string, name: string, url: string, token: string) => Promise<Connection>
  SetActiveConnection?: (id: string) => Promise<void>
}

function shell(): ShellConnections | null {
  const s = (window as unknown as { go?: { main?: { ShellAPI?: ShellConnections } } }).go?.main?.ShellAPI
  return s ?? null
}

// LOCAL_CONNECTION_ID is the well-known ID for the desktop's loopback
// connection. Mirrors internal/config.LocalConnectionID on the Go side
// — kept as a UI constant so isLocalActive / picker rendering can
// recognise it without prefixing checks against a constant elsewhere.
export const LOCAL_CONNECTION_ID = 'local'

type ConnectionsState = {
  ready: boolean
  available: boolean         // false when the backend doesn't expose connections (browser mode against bruv-server)
  active: string             // ID of the active connection ("local" for desktop loopback)
  connections: Connection[]  // every connection — Local is a real entry now, not implicit
}

export const connections = $state<ConnectionsState>({
  ready: false,
  available: false,
  active: LOCAL_CONNECTION_ID,
  connections: [],
})

function applyStore(s: ConnectionStore) {
  connections.active = s.active || LOCAL_CONNECTION_ID
  connections.connections = s.connections ?? []
  connections.available = true
  connections.ready = true
}

// loadConnections asks the backend for the persisted store. Prefers
// the Wails Shell binding (direct, doesn't depend on the active
// connection being reachable); falls back to the cloud adapter for
// browser mode. When neither works, marks the UI as unavailable.
export async function loadConnections(): Promise<void> {
  try {
    const s = shell()
    const store = s?.ListConnections ? await s.ListConnections() : await ListConnections()
    applyStore(store)
  } catch {
    connections.available = false
    connections.ready = true
  }
}

// addConnection persists a freshly-enrolled remote and (optionally)
// switches to it. The "switch" path triggers a full reload — caller
// shouldn't await past the SetActiveConnection call.
export async function addConnection(
  name: string,
  url: string,
  deviceToken: string,
  opts: { activate?: boolean } = {},
): Promise<Connection> {
  const s = shell()
  const created = s?.AddConnection
    ? await s.AddConnection(name, url, deviceToken)
    : await AddConnection(name, url, deviceToken)
  // Refresh local view so the dialog UI sees the new entry immediately.
  await loadConnections()
  if (opts.activate) {
    await switchConnection(created.id)
    // switchConnection triggers reload — code below this point is unreachable.
  }
  return created
}

// updateConnection edits an existing remote's name / URL / token.
// Empty fields leave the existing value unchanged. Goes via the
// Wails Shell so it works even when the active backend is down
// (same posture as the rest of connection management).
export async function updateConnection(
  id: string,
  name: string,
  url: string,
  deviceToken: string,
): Promise<Connection> {
  const s = shell()
  const updated = s?.UpdateConnection
    ? await s.UpdateConnection(id, name, url, deviceToken)
    : await UpdateConnection(id, name, url, deviceToken)
  await loadConnections()
  return updated
}

export async function removeConnection(id: string): Promise<void> {
  const s = shell()
  if (s?.RemoveConnection) {
    await s.RemoveConnection(id)
  } else {
    await RemoveConnection(id)
  }
  await loadConnections()
  // If the removed entry was active, the backend reset Active to "".
  // The store reload above already reflects that. The caller (dialog)
  // can decide whether to reload the page; we don't force it because
  // the user might be removing a non-active connection.
}

// switchConnection sets the active pointer and reloads the window.
// Reloading is the cleanest way to swap the entire transport + every
// piece of in-memory state belonging to the old backend.
export async function switchConnection(id: string): Promise<void> {
  const s = shell()
  if (s?.SetActiveConnection) {
    await s.SetActiveConnection(id)
  } else {
    await SetActiveConnection(id)
  }
  // Tiny delay so any in-flight RPCs settle before the reload.
  setTimeout(() => window.location.reload(), 50)
}

// Helpers used by indicator components.

export function activeConnectionLabel(): string {
  const found = connections.connections.find(c => c.id === connections.active)
  return found ? found.name : 'Local'
}

export function isLocalActive(): boolean {
  return connections.active === LOCAL_CONNECTION_ID
}
