// Repo-selection state + helpers.
//
// "Which repo am I on right now" is per-connection state — see
// internal/config/repo_recents.go on the backend. This module wraps
// the operations the picker needs around it:
//
//   - listAllConnectionRepos(): fan out GET /repos to every known
//     connection in parallel so the tree picker can render Local +
//     every Remote uniformly.
//   - listServerRepos(): fetch the active connection's GET /repos.
//   - setRepoEnabled / inspectRepo / addRepo / removeRepo / renameRepo:
//     hit the per-connection HTTP routes (POST/PATCH/DELETE under
//     /repos) directly using that connection's URL + token. Works
//     cross-connection — the picker can manage Local repos while a
//     Remote is the active backend, and vice versa.
//   - selectRepo / switchToRepo: persist the user's choice and
//     trigger the page reload that cycles transport.
//   - probeBackend(): boot-time reachability check.

import { resolveTransportInfo, serverGet } from '@shared/adapters/cloud'
import { connections, LOCAL_CONNECTION_ID } from './connections.svelte'
import type { Connection } from '@shared/types'

export type ServerRepo = {
  id: string
  name: string
  disabled?: boolean
}

export type RepoInspect = {
  exists: boolean
  name?: string
  id?: string
}

// connectionByID returns a Connection record by its ID, or undefined.
// Used by the cross-connection HTTP helpers below to look up the
// URL + token before fetching.
function connectionByID(id: string): Connection | undefined {
  return connections.connections.find(c => c.id === id)
}

// connectionFetch hits a path on the given connection using its
// stored URL + bearer token. Returns the raw Response so callers can
// branch on status — for the picker UI we want "this connection isn't
// reachable" to render as a greyed row, not throw.
async function connectionFetch(c: Connection, path: string, init?: RequestInit, timeoutMs = 4000): Promise<Response> {
  const url = c.url.replace(/\/+$/, '')
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  try {
    return await fetch(`${url}${path}`, {
      ...init,
      headers: {
        ...(init?.headers ?? {}),
        Authorization: `Bearer ${c.device_token}`,
      },
      signal: ctrl.signal,
    })
  } finally {
    clearTimeout(timer)
  }
}

// setRepoEnabled toggles a repo's Disabled flag on the given
// connection. Hits POST /repos/<id>/(enable|disable) on that
// connection's URL — works whether the connection is the active one
// or not, because we always have its URL + token from connections.json.
export async function setRepoEnabled(connectionID: string, repoID: string, enabled: boolean): Promise<void> {
  const c = connectionByID(connectionID)
  if (!c) throw new Error(`unknown connection: ${connectionID}`)
  const verb = enabled ? 'enable' : 'disable'
  const res = await connectionFetch(c, `/repos/${encodeURIComponent(repoID)}/${verb}`, { method: 'POST' })
  if (!res.ok) throw new Error(`set repo ${verb}: HTTP ${res.status} ${res.statusText}`)
}

// inspectRepoOnConnection asks the given connection's backend to
// inspect a path — used by the picker's "add repo" flow before the
// user has committed to init vs open.
export async function inspectRepoOnConnection(connectionID: string, path: string): Promise<RepoInspect> {
  const c = connectionByID(connectionID)
  if (!c) throw new Error(`unknown connection: ${connectionID}`)
  const res = await connectionFetch(c, '/repos/inspect', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`inspect: HTTP ${res.status} ${text || res.statusText}`)
  }
  return (await res.json()) as RepoInspect
}

// addRepoOnConnection registers + loads a repo at the given path on
// the given connection. The backend's POST /repos handler does
// inspect-then-init-or-open; the name parameter is required for
// fresh folders and ignored for existing repos. Returns the
// resulting RepoSummary so the caller can stamp the new ID into the
// connection's repo-recents and reload.
export async function addRepoOnConnection(connectionID: string, path: string, name: string): Promise<ServerRepo> {
  const c = connectionByID(connectionID)
  if (!c) throw new Error(`unknown connection: ${connectionID}`)
  const res = await connectionFetch(c, '/repos', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`add repo: HTTP ${res.status} ${text || res.statusText}`)
  }
  return (await res.json()) as ServerRepo
}

// renameRepoOnConnection hits PATCH /repos/<id> on the given
// connection. Updates BOTH the registry name and the in-repo
// manifest name (the backend handler does both atomically).
export async function renameRepoOnConnection(connectionID: string, repoID: string, name: string): Promise<void> {
  const c = connectionByID(connectionID)
  if (!c) throw new Error(`unknown connection: ${connectionID}`)
  const res = await connectionFetch(c, `/repos/${encodeURIComponent(repoID)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`rename repo: HTTP ${res.status} ${text || res.statusText}`)
  }
}

// removeRepoOnConnection drops a repo from the given connection's
// registry. Folder on disk is left alone; the runtime is unloaded.
export async function removeRepoOnConnection(connectionID: string, repoID: string): Promise<void> {
  const c = connectionByID(connectionID)
  if (!c) throw new Error(`unknown connection: ${connectionID}`)
  const res = await connectionFetch(c, `/repos/${encodeURIComponent(repoID)}`, { method: 'DELETE' })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`remove repo: HTTP ${res.status} ${text || res.statusText}`)
  }
}

// listServerRepos fetches the active connection's GET /repos endpoint
// via the cloud adapter's already-resolved transport info. Used by
// the picker to refresh just the active row after a mutation.
export async function listServerRepos(): Promise<ServerRepo[]> {
  const info = await resolveTransportInfo()
  const res = await serverGet(info, '/repos')
  if (!res.ok) {
    throw new Error(`list repos: HTTP ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as ServerRepo[]
}

// listAllConnectionRepos fetches /repos from every known connection
// in parallel so the tree picker can render a cross-connection view.
// Local is just another connection (id="local") since the local-as-
// remote pivot — same code path as Remote.
export type TreeConnectionNode = {
  connectionID: string
  connectionName: string
  isLocal: boolean
  reachable: boolean
  repos: ServerRepo[]
  error?: string
}

export async function listAllConnectionRepos(): Promise<TreeConnectionNode[]> {
  return Promise.all(connections.connections.map(async (c): Promise<TreeConnectionNode> => {
    const node: TreeConnectionNode = {
      connectionID: c.id,
      connectionName: c.name,
      isLocal: c.id === LOCAL_CONNECTION_ID,
      reachable: false,
      repos: [],
    }
    try {
      const res = await connectionFetch(c, '/repos')
      if (!res.ok) {
        node.error = `HTTP ${res.status}`
        return node
      }
      node.repos = (await res.json()) as ServerRepo[]
      node.reachable = true
    } catch (e) {
      node.error = (e as Error)?.message ?? String(e)
    }
    return node
  }))
}

// switchToRepo picks a (connection, repo) pair from the tree and
// transitions the app there in one click. When crossing connections
// we MUST pre-set the target connection's last-active-repo via the
// Shell binding BEFORE calling switchConnection — switchConnection
// reloads, and the post-reload boot resolves the cloud adapter using
// whatever pointer is on disk for the new connection. Without the
// pre-set the target lands on the picker (no repoID), undoing the
// "I picked this specific repo" intent.
export async function switchToRepo(connectionID: string, repoID: string): Promise<void> {
  const { switchConnection } = await import('./connections.svelte')
  if (connectionID !== connections.active) {
    type ShellSet = { SetActiveRepoForConnection?: (connID: string, repoID: string) => Promise<void> }
    const s = (window as unknown as { go?: { main?: { ShellAPI?: ShellSet } } }).go?.main?.ShellAPI
    if (s?.SetActiveRepoForConnection) {
      await s.SetActiveRepoForConnection(connectionID, repoID)
    }
    await switchConnection(connectionID)
    return
  }
  await selectRepo(repoID)
}

// probeBackend hits /healthz with a short timeout. Returns true on
// 2xx, false on anything else (timeout, network error, non-OK
// response). Used at boot to detect "remote down" before any RPC.
export async function probeBackend(timeoutMs = 3000): Promise<boolean> {
  try {
    const info = await resolveTransportInfo()
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), timeoutMs)
    try {
      const res = await serverGet(info, '/healthz', { signal: ctrl.signal })
      return res.ok
    } finally {
      clearTimeout(timer)
    }
  } catch {
    return false
  }
}

// selectRepo persists the user's repo choice and reloads. Goes via
// the Wails Shell so it works even if the active backend is broken
// in other ways. Browser-mode (no Shell) falls back to the cloud
// adapter, which is fine because the only way browser mode reached
// the picker is by talking to a working server.
export async function selectRepo(repoID: string): Promise<void> {
  const shell = (window as unknown as { go?: { main?: { ShellAPI?: { SetActiveRepo?: (id: string) => Promise<void> } } } }).go?.main?.ShellAPI
  if (shell?.SetActiveRepo) {
    await shell.SetActiveRepo(repoID)
  } else {
    const { SetActiveRepo } = await import('@shared/api')
    await SetActiveRepo(repoID)
  }
  setTimeout(() => window.location.reload(), 50)
}
