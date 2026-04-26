// Local-repo management — operations that ALWAYS target the desktop
// App's `<userConfigDir>/repos.json`, regardless of which Remote
// connection is currently active. The picker can show Local rows
// while a Remote is the active connection; clicking + / pencil / X
// on those rows must manipulate Local state, not the Remote's.
//
// These wrappers prefer the Wails Shell binding when available
// (desktop) so the call goes straight to the desktop App without
// touching the cloud adapter. Browser mode falls through to the
// cloud-adapter exports — for browser-only deployments the only
// "Local" is the server you're loaded from, so the distinction
// collapses naturally.
//
// Mirrors the same pattern used by lib/connections.svelte.ts for
// connection management.

import {
  InspectRepoPath as ApiInspectRepoPath,
  InitRepository as ApiInitRepository,
  OpenRepository as ApiOpenRepository,
  RemoveLocalRepo as ApiRemoveLocalRepo,
  RenameLocalRepo as ApiRenameLocalRepo,
  SetLocalRepoEnabled as ApiSetLocalRepoEnabled,
  GetLastOpenedLocalRepoPath as ApiGetLastOpenedLocalRepoPath,
} from './api'

type ShellLocal = {
  InspectRepoPath?: (path: string) => Promise<{ exists: boolean; name: string; id: string }>
  InitRepository?: (path: string, name: string) => Promise<string>
  OpenRepository?: (path: string) => Promise<void>
  RemoveLocalRepo?: (id: string) => Promise<void>
  RenameLocalRepo?: (id: string, name: string) => Promise<void>
  SetLocalRepoEnabled?: (id: string, enabled: boolean) => Promise<void>
  GetLastOpenedLocalRepoPath?: () => Promise<string>
}

function shell(): ShellLocal | null {
  const s = (window as unknown as { go?: { main?: { ShellAPI?: ShellLocal } } }).go?.main?.ShellAPI
  return s ?? null
}

export async function inspectLocalRepoPath(path: string) {
  const s = shell()
  return s?.InspectRepoPath ? s.InspectRepoPath(path) : ApiInspectRepoPath(path)
}

export async function initLocalRepository(path: string, name: string) {
  const s = shell()
  return s?.InitRepository ? s.InitRepository(path, name) : ApiInitRepository(path, name)
}

export async function openLocalRepository(path: string) {
  const s = shell()
  if (s?.OpenRepository) return s.OpenRepository(path)
  return ApiOpenRepository(path)
}

export async function removeLocalRepository(id: string) {
  const s = shell()
  if (s?.RemoveLocalRepo) return s.RemoveLocalRepo(id)
  return ApiRemoveLocalRepo(id)
}

export async function renameLocalRepository(id: string, name: string) {
  const s = shell()
  if (s?.RenameLocalRepo) return s.RenameLocalRepo(id, name)
  return ApiRenameLocalRepo(id, name)
}

export async function setLocalRepoEnabled(id: string, enabled: boolean) {
  const s = shell()
  if (s?.SetLocalRepoEnabled) return s.SetLocalRepoEnabled(id, enabled)
  return ApiSetLocalRepoEnabled(id, enabled)
}

export async function getLastOpenedLocalRepoPath(): Promise<string> {
  const s = shell()
  if (s?.GetLastOpenedLocalRepoPath) return s.GetLastOpenedLocalRepoPath()
  return ApiGetLastOpenedLocalRepoPath()
}
