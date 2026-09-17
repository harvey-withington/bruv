// Canonical narrowing for block values whose on-disk shape is richer
// than the thing being displayed.
//
// URL blocks store `{ url, caption? }` — that's what cardMarkdown
// exports, what mobile's UrlBlock edits, and what the clipper and
// server-side capture ingest write. Desktop's BlockItem used to treat
// the value as a bare string, so any URL block created anywhere else
// rendered as "[object Object]" (found 2026-08-01 on a captured link
// card), and desktop's own edits wrote a string that mobile then showed
// as empty. One helper, tolerant of both shapes, ends the disagreement.

import type { WorkspaceFileEntry, WorkspaceFilesDisplay } from './types'

export type UrlBlockValue = { url: string; caption?: string }

/**
 * Narrow a url block's value to `{ url, caption? }`.
 *
 * Accepts the canonical object AND a legacy bare string (what older
 * desktop edits wrote), so no migration is needed — cards heal into the
 * canonical shape the next time they're edited.
 */
export function asUrlValue(v: unknown): UrlBlockValue {
  if (typeof v === 'string') return { url: v }
  if (v && typeof v === 'object' && 'url' in v) {
    const obj = v as { url?: unknown; caption?: unknown }
    return {
      url: typeof obj.url === 'string' ? obj.url : '',
      caption: typeof obj.caption === 'string' ? obj.caption : undefined,
    }
  }
  return { url: '' }
}

/**
 * Build the canonical stored value, preserving an existing caption —
 * editing the URL must never silently drop one.
 */
export function urlBlockValue(url: string, previous?: unknown): UrlBlockValue {
  const caption = asUrlValue(previous).caption
  return caption ? { url, caption } : { url }
}

/**
 * Narrow a workspace_files block's value to its entry list. Tolerates
 * anything that isn't an array of `{ workspace_id, path }` objects by
 * dropping it — a malformed entry can't be opened anyway.
 */
export function asWorkspaceFiles(v: unknown): WorkspaceFileEntry[] {
  if (!Array.isArray(v)) return []
  const out: WorkspaceFileEntry[] = []
  for (const raw of v) {
    if (!raw || typeof raw !== 'object') continue
    const o = raw as { id?: unknown; workspace_id?: unknown; path?: unknown; is_dir?: unknown }
    if (typeof o.workspace_id !== 'string' || typeof o.path !== 'string' || o.path === '') continue
    out.push({
      id: typeof o.id === 'string' && o.id ? o.id : `wsf-${o.workspace_id}-${o.path}`,
      workspace_id: o.workspace_id,
      path: o.path,
      is_dir: o.is_dir === true ? true : undefined,
    })
  }
  return out
}

export function asWorkspaceFilesDisplay(v: unknown): WorkspaceFilesDisplay {
  return v === 'flat' ? 'flat' : 'tree'
}

/** Last path segment — what a file row shows. */
export function workspacePathName(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const i = trimmed.lastIndexOf('/')
  return i < 0 ? trimmed : trimmed.slice(i + 1)
}

/** Parent folder of a workspace-relative path ('' at the root). */
export function workspaceParentPath(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const i = trimmed.lastIndexOf('/')
  return i < 0 ? '' : trimmed.slice(0, i)
}
