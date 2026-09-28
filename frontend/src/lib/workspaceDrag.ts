// Drag payload for workspace tree rows → Workspace Files blocks.
//
// Every other drag in the app is intra-component and keeps its payload in
// component state; this one crosses from the side panel's tree into a card
// dialog, so the payload rides dataTransfer under a custom type. The type
// doubles as the drop-target test: a drag without it is not ours.

import type { WorkspaceFileEntry } from '@shared/types'

export const WORKSPACE_ENTRY_MIME = 'application/x-bruv-workspace-entry'

export interface WorkspaceDragPayload {
  workspaceId: string
  path: string
  isDir: boolean
}

export function setWorkspaceDrag(dt: DataTransfer | null, payload: WorkspaceDragPayload): void {
  if (!dt) return
  dt.effectAllowed = 'copy'
  dt.setData(WORKSPACE_ENTRY_MIME, JSON.stringify(payload))
  // A plain-text fallback keeps the drag legal everywhere and readable when
  // dropped into a text field.
  dt.setData('text/plain', payload.path)
}

export function hasWorkspaceDrag(dt: DataTransfer | null): boolean {
  return !!dt && Array.from(dt.types).includes(WORKSPACE_ENTRY_MIME)
}

export function readWorkspaceDrag(dt: DataTransfer | null): WorkspaceDragPayload | null {
  if (!dt) return null
  const raw = dt.getData(WORKSPACE_ENTRY_MIME)
  if (!raw) return null
  try {
    const p = JSON.parse(raw) as Partial<WorkspaceDragPayload>
    if (typeof p.workspaceId !== 'string' || typeof p.path !== 'string') return null
    return { workspaceId: p.workspaceId, path: p.path, isDir: p.isDir === true }
  } catch {
    return null
  }
}

export function payloadToEntry(p: WorkspaceDragPayload, id: string): WorkspaceFileEntry {
  return p.isDir
    ? { id, workspace_id: p.workspaceId, path: p.path, is_dir: true }
    : { id, workspace_id: p.workspaceId, path: p.path }
}
