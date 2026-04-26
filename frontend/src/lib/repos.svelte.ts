// Repo-selection state + helpers.
//
// "Which repo am I on right now" is per-machine + per-connection
// state — see internal/config/repo_recents.go on the backend. This
// module wraps the two operations the frontend needs around it:
//
//   - listServerRepos(): fetch GET /repos directly via serverGet,
//     bypassing the RPC layer (which requires a /repos/<id>/ prefix
//     and would fail when the user hasn't picked yet).
//
//   - selectRepo(repoID): persist the choice via the Wails Shell
//     (so it works even when the active backend is unreachable in
//     other ways) and reload, so the cloud adapter rebuilds with
//     the new prefix.
//
//   - probeBackend(): low-overhead reachability check used by the
//     boot path to distinguish "remote up" from "remote down" before
//     we make any normal RPC.

import { resolveTransportInfo, serverGet } from './adapters/cloud'

export type ServerRepo = {
  id: string
  name: string
  disabled?: boolean
}

// setRepoEnabled toggles a repo's Disabled flag. Local rows route
// through the Wails Shell binding so they hit the desktop App's
// repos.json regardless of which Remote is currently active. Remote
// rows post to /repos/<id>/(enable|disable) on the active connection
// — the existing picker UX gates this with "switch to that connection
// first", so the Remote branch only fires when active connection is
// already that connection.
//
// connectionID="" means Local. Anything else is treated as Remote.
export async function setRepoEnabled(connectionID: string, repoID: string, enabled: boolean): Promise<void> {
  if (connectionID === '') {
    const { setLocalRepoEnabled } = await import('./local')
    await setLocalRepoEnabled(repoID, enabled)
    return
  }
  const info = await resolveTransportInfo()
  const verb = enabled ? 'enable' : 'disable'
  const res = await serverGet(info, `/repos/${encodeURIComponent(repoID)}/${verb}`, { method: 'POST' })
  if (!res.ok) {
    throw new Error(`set repo ${verb}: HTTP ${res.status} ${res.statusText}`)
  }
}

// listServerRepos fetches the active connection's GET /repos endpoint.
// Returns an empty list on error (caller decides how to surface it —
// usually by routing to the Unreachable screen).
export async function listServerRepos(): Promise<ServerRepo[]> {
  const info = await resolveTransportInfo()
  const res = await serverGet(info, '/repos')
  if (!res.ok) {
    throw new Error(`list repos: HTTP ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as ServerRepo[]
}

// listAllConnectionRepos fetches /repos from every known connection
// in parallel (Local + every Remote in connections.json) so the
// tree picker can render a cross-connection view without page
// reloads. Each entry is stamped with its source connectionID and
// reachability state so the picker can show greyed/unreachable
// rows alongside the live ones.
export type TreeConnectionNode = {
  connectionID: string  // "" for the implicit Local
  connectionName: string
  isLocal: boolean
  reachable: boolean
  repos: ServerRepo[]
  error?: string
}

export async function listAllConnectionRepos(): Promise<TreeConnectionNode[]> {
  // Connection management goes through the Wails Shell binding when
  // available (desktop) — the same posture connections.svelte.ts uses.
  // Critical: when the active connection is a Remote (multi-repo)
  // server, the cloud adapter routes EVERYTHING through
  // /repos/<id>/rpc. There's no bare /rpc on a multi-repo server,
  // so falling through to the cloud-adapter ListConnections returns
  // HTTP 404 and the picker can't even render the connection list
  // — exactly the state where the user needs the picker to work
  // (so they can switch back to Local or pick a remote repo).
  type ShellConn = { ListConnections?: () => Promise<{ active: string; connections: Array<{ id: string; name: string; url: string; device_token: string }> }> }
  const shell = (window as unknown as { go?: { main?: { ShellAPI?: ShellConn } } }).go?.main?.ShellAPI
  let store: { active: string; connections: Array<{ id: string; name: string; url: string; device_token: string }> }
  if (shell?.ListConnections) {
    store = await shell.ListConnections()
  } else {
    const { ListConnections } = await import('./api')
    store = await ListConnections()
  }

  const localInfo = await resolveTransportInfo()
  const isLocalActiveBackend = localInfo.remote !== 'true'

  // Local's repo list comes from one of two sources depending on which
  // connection the cloud adapter is currently routed at:
  //   - Local active → GET /repos via the active (loopback) transport.
  //   - Remote active → can't go via the cloud adapter (it routes
  //     to the Remote server), so we ask the desktop App directly
  //     through the Wails Shell binding. Same source-of-truth either
  //     way (<userConfigDir>/repos.json), just different access path.
  // Without the Shell fallback, adding a repo to Local while a Remote
  // is active worked silently (the entry got written) but the picker
  // never displayed it — exactly the "doesn't add the folder" bug.
  const local: TreeConnectionNode = {
    connectionID: '',
    connectionName: 'Local',
    isLocal: true,
    reachable: true,
    repos: [],
  }
  if (isLocalActiveBackend) {
    try {
      local.repos = await listServerRepos()
    } catch (e) {
      local.reachable = false
      local.error = (e as Error)?.message ?? String(e)
    }
  } else {
    type ShellLocalRepos = { ListLocalRepos?: () => Promise<Array<{ id: string; name: string; path?: string; disabled?: boolean }>> }
    const sLocal = (window as unknown as { go?: { main?: { ShellAPI?: ShellLocalRepos } } }).go?.main?.ShellAPI
    if (sLocal?.ListLocalRepos) {
      try {
        const entries = await sLocal.ListLocalRepos()
        local.repos = (entries ?? []).map(e => ({ id: e.id, name: e.name, disabled: e.disabled }))
      } catch (e) {
        local.reachable = false
        local.error = (e as Error)?.message ?? String(e)
      }
    } else {
      // Browser mode (no Shell): there's no separate "Local" beyond
      // the server we loaded from, so this row is meaningless. Mark
      // unreachable rather than silently empty.
      local.reachable = false
    }
  }

  // Each remote: parallel /repos fetch with its own URL + token.
  const remotes = await Promise.all((store.connections ?? []).map(async (c): Promise<TreeConnectionNode> => {
    const node: TreeConnectionNode = {
      connectionID: c.id,
      connectionName: c.name,
      isLocal: false,
      reachable: false,
      repos: [],
    }
    try {
      const url = c.url.replace(/\/+$/, '')
      const ctrl = new AbortController()
      const timer = setTimeout(() => ctrl.abort(), 4000)
      try {
        const res = await fetch(`${url}/repos`, {
          headers: { Authorization: `Bearer ${c.device_token}` },
          signal: ctrl.signal,
        })
        if (!res.ok) {
          node.error = `HTTP ${res.status}`
          return node
        }
        node.repos = (await res.json()) as ServerRepo[]
        node.reachable = true
      } finally {
        clearTimeout(timer)
      }
    } catch (e) {
      node.error = (e as Error)?.message ?? String(e)
    }
    return node
  }))

  return [local, ...remotes]
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
  const { connections, switchConnection } = await import('./connections.svelte')
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
    const { SetActiveRepo } = await import('./api')
    await SetActiveRepo(repoID)
  }
  // Tiny delay so the Wails IPC call settles, then reload — the
  // adapter re-resolves transportInfo and includes the new repoID
  // in every URL.
  setTimeout(() => window.location.reload(), 50)
}
