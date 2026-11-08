import type { Card, WorkspaceFileContent, WorkspaceFileStamp, WorkspaceSaveResult } from './types'

/**
 * Where a document's bytes come from. Both surfaces' editors (the desktop
 * DocumentEditor, the mobile DocumentSheet) are written against this, not
 * against RPCs, so every source plugs in without touching them: a
 * workspace file (the user's folder) and a text attachment (a file the
 * card owns in the vault) are the two today.
 *
 * Stamps are the divergence guard: `open` hands one out, `save` presents
 * it back and reports `diverged` (nothing written) when the file changed
 * under us; an empty `expectedHash` overwrites.
 */
export interface DocumentSource {
  /** Path as shown to the user (workspace-relative, or the attachment name). */
  path: string
  open(): Promise<WorkspaceFileContent>
  stat(): Promise<WorkspaceFileStamp>
  save(content: string, expectedHash: string): Promise<WorkspaceSaveResult>
  /** Save `content` as a new document named `name` beside this one; the
   *  editor carries on in the copy. Attachments only. */
  saveAs?(name: string, content: string): Promise<SavedAs>
}

export type SavedAs = {
  source: DocumentSource
  /** The card as saved, for lists showing its attachments. */
  card: Card
}

/** A per-repo RPC call — each surface binds its own transport. */
export type RepoCall = <T>(method: string, params: unknown[]) => Promise<T>

export function workspaceDocumentSource(call: RepoCall, brandSlug: string, streamSlug: string, projectSlug: string, path: string): DocumentSource {
  return {
    path,
    open: () => call<WorkspaceFileContent>('OpenWorkspaceFile', [brandSlug, streamSlug, projectSlug, path]),
    stat: () => call<WorkspaceFileStamp>('StatWorkspaceFile', [brandSlug, streamSlug, projectSlug, path]),
    save: (content, expectedHash) => call<WorkspaceSaveResult>('SaveWorkspaceFile', [brandSlug, streamSlug, projectSlug, path, content, expectedHash]),
  }
}

export function attachmentDocumentSource(call: RepoCall, cardId: string, attachmentId: string, name: string): DocumentSource {
  return {
    path: name,
    open: () => call<WorkspaceFileContent>('OpenCardAttachmentText', [cardId, attachmentId]),
    stat: () => call<WorkspaceFileStamp>('StatCardAttachmentText', [cardId, attachmentId]),
    save: (content, expectedHash) => call<WorkspaceSaveResult>('SaveCardAttachmentText', [cardId, attachmentId, content, expectedHash]),
    saveAs: async (newName, content) => {
      const before = new Set(((await call<Card>('GetCard', [cardId]))?.file_attachments ?? []).map((a) => a.id))
      const card = await call<Card>('AddCardAttachment', [cardId, newName, utf8Base64(content)])
      // The new attachment is the one that wasn't there before (names can repeat).
      const added = (card.file_attachments ?? []).filter((a) => !before.has(a.id)).pop()
      if (!added) throw new Error(`${newName} was not added to the card`)
      return { source: attachmentDocumentSource(call, cardId, added.id, added.name), card }
    },
  }
}

/**
 * Save as, as both editors do it: `draft` becomes the new document `name`,
 * and the original goes back to `opened` (how it was when the editor
 * opened it) — autosave has been overwriting it as the user typed, so
 * without the put-back the edits would be in both. The put-back presents
 * `stampHash` (the stamp of the last save), so a change made elsewhere
 * meanwhile is never clobbered; the copy is safe either way.
 */
export async function saveAsAndRestore(
  source: DocumentSource, name: string, draft: string, opened: string, lastSaved: string, stampHash: string,
): Promise<SavedAs> {
  if (!source.saveAs) throw new Error('this document cannot be saved as a copy')
  const saved = await source.saveAs(name, draft)
  if (lastSaved !== opened && stampHash) {
    try {
      await source.save(opened, stampHash)
    } catch { /* the original keeps the autosaved text; the copy is what matters */ }
  }
  return saved
}

/** "post.md" → "post (copy).md": the name Save as offers first. */
export function copyName(name: string): string {
  return nameWithSuffix(name, ' (copy)')
}

/** Insert `suffix` before the extension: ("post.md", " (x)") → "post (x).md". */
export function nameWithSuffix(name: string, suffix: string): string {
  const dot = name.lastIndexOf('.')
  return dot > 0 ? `${name.slice(0, dot)}${suffix}${name.slice(dot)}` : `${name}${suffix}`
}

/** A typed Save-as name, kept to the original's format: no extension → the
 *  original's is added (so the copy opens in the same editor). */
export function saveAsName(typed: string, original: string): string {
  const name = typed.trim()
  const dot = original.lastIndexOf('.')
  if (!name || name.lastIndexOf('.') > 0 || dot <= 0) return name
  return name + original.slice(dot)
}

export function utf8Base64(s: string): string {
  let bin = ''
  for (const byte of new TextEncoder().encode(s)) bin += String.fromCharCode(byte)
  return btoa(bin)
}
