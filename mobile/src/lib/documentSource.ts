import type { WorkspaceFileContent, WorkspaceFileStamp, WorkspaceSaveResult } from '@shared/types'
import { repoRPC } from './auth'

/**
 * Where a document's bytes come from — the mobile twin of the desktop's
 * lib/editor/documentSource.ts, over repoRPC. Same two sources: a
 * workspace file (the user's folder, edited on the host over the
 * connection) and a text attachment (a file the card owns in the vault).
 * Stamps are the divergence guard: save presents the hash it loaded and
 * is refused (diverged, nothing written) when the file changed meanwhile.
 */
export interface DocumentSource {
  /** Shown as the sheet title. */
  path: string
  open(): Promise<WorkspaceFileContent>
  stat(): Promise<WorkspaceFileStamp>
  save(content: string, expectedHash: string): Promise<WorkspaceSaveResult>
}

export function workspaceDocumentSource(brandSlug: string, streamSlug: string, projectSlug: string, path: string): DocumentSource {
  return {
    path,
    open: () => repoRPC<WorkspaceFileContent>('OpenWorkspaceFile', [brandSlug, streamSlug, projectSlug, path]),
    stat: () => repoRPC<WorkspaceFileStamp>('StatWorkspaceFile', [brandSlug, streamSlug, projectSlug, path]),
    save: (content, expectedHash) => repoRPC<WorkspaceSaveResult>('SaveWorkspaceFile', [brandSlug, streamSlug, projectSlug, path, content, expectedHash]),
  }
}

export function attachmentDocumentSource(cardId: string, attachmentId: string, name: string): DocumentSource {
  return {
    path: name,
    open: () => repoRPC<WorkspaceFileContent>('OpenCardAttachmentText', [cardId, attachmentId]),
    stat: () => repoRPC<WorkspaceFileStamp>('StatCardAttachmentText', [cardId, attachmentId]),
    save: (content, expectedHash) => repoRPC<WorkspaceSaveResult>('SaveCardAttachmentText', [cardId, attachmentId, content, expectedHash]),
  }
}
