// Reactive store for the per-machine known-connections list.
//
// The store mirrors the backend ConnectionStore plus an `ready` flag
// for components that want to wait for the first load before rendering.
// Mutations (add / remove / setActive) call the backend, refresh the
// store from the response, and — for setActive — trigger a full
// window reload so the cloud adapter re-resolves transport against
// the new active connection (same dance as switching repos).

import { ListConnections, AddConnection, RemoveConnection, SetActiveConnection } from './api'
import type { Connection, ConnectionStore } from './types'

type ConnectionsState = {
  ready: boolean
  available: boolean         // false when the backend doesn't expose connections (browser mode against bruv-server)
  active: string             // "" = Local
  connections: Connection[]  // remote connections only; Local is implicit
}

export const connections = $state<ConnectionsState>({
  ready: false,
  available: false,
  active: '',
  connections: [],
})

function applyStore(s: ConnectionStore) {
  connections.active = s.active || ''
  connections.connections = s.connections ?? []
  connections.available = true
  connections.ready = true
}

// loadConnections asks the backend for the persisted store. When the
// backend doesn't expose connection management (e.g. browser mode
// pointing at a bruv-server, whose RPC target is the headless runtime
// struct rather than the desktop App), the call fails — we treat that
// as "this UI surface isn't available" and let the indicator hide
// itself rather than blow up the boot path.
export async function loadConnections(): Promise<void> {
  try {
    const s = await ListConnections()
    applyStore(s)
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
  const created = await AddConnection(name, url, deviceToken)
  // Refresh local view so the dialog UI sees the new entry immediately.
  await loadConnections()
  if (opts.activate) {
    await switchConnection(created.id)
    // switchConnection triggers reload — code below this point is unreachable.
  }
  return created
}

export async function removeConnection(id: string): Promise<void> {
  await RemoveConnection(id)
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
  await SetActiveConnection(id)
  // Tiny delay so any in-flight RPCs settle before the reload.
  setTimeout(() => window.location.reload(), 50)
}

// Helpers used by indicator components.

export function activeConnectionLabel(): string {
  if (!connections.active) return 'Local'
  const found = connections.connections.find(c => c.id === connections.active)
  return found ? found.name : 'Local'
}

export function isLocalActive(): boolean {
  return !connections.active
}
