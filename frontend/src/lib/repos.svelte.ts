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

// setRepoEnabled toggles a repo's Disabled flag on the server. When
// enabled, the supervisor lazy-starts the runtime; when disabled, it
// shuts the runtime down. Both persist to repos.json so the state
// survives restart.
export async function setRepoEnabled(repoID: string, enabled: boolean): Promise<void> {
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
  // Lazy import to avoid pulling Wails-specific surfaces at module
  // load time (the function is only called after the app is alive).
  const { ListConnections } = await import('./api')
  const store = await ListConnections()

  const localInfo = await resolveTransportInfo()
  const isLocalActiveBackend = localInfo.remote !== 'true'

  // For Local: only fetch if the active connection IS Local. Otherwise
  // we'd need to round-trip through Wails to get the loopback URL +
  // token without disturbing the active session, which is a follow-up.
  // For now, Local appears as an unreachable placeholder when the
  // user is on a Remote connection; the picker still lets them
  // switch back to Local via the chip's existing flow.
  const local: TreeConnectionNode = {
    connectionID: '',
    connectionName: 'Local',
    isLocal: true,
    reachable: isLocalActiveBackend,
    repos: [],
  }
  if (isLocalActiveBackend) {
    try {
      local.repos = await listServerRepos()
    } catch (e) {
      local.reachable = false
      local.error = (e as Error)?.message ?? String(e)
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
// transitions the app there. Combines connection + repo in one
// click — for the user this looks instant (page reload to land
// on the new transport).
export async function switchToRepo(connectionID: string, repoID: string): Promise<void> {
  // selectRepo persists the repo choice for the active connection.
  // If we need to switch connection too, do that first (which itself
  // reloads), then the next launch picks up the repo we're about
  // to set. Order matters because both reload.
  const { connections, switchConnection } = await import('./connections.svelte')
  if (connectionID !== connections.active) {
    // Persist the desired repo BEFORE switching so the new
    // connection's boot path lands on it.
    const { SetActiveRepo } = await import('./api')
    // We have to set the recent for the *target* connection. Today
    // SetActiveRepo only knows about the currently-active connection,
    // so this is a known limitation — the user will land on the
    // target connection's last-viewed repo (or the picker if none).
    // True per-target persistence is a follow-up.
    void SetActiveRepo
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
