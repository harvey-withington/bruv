import {
  OpenCardAttachmentText, OpenWorkspaceFile, SaveCardAttachmentText, SaveWorkspaceFile,
  StatCardAttachmentText, StatWorkspaceFile,
} from '@shared/api'
import type { WorkspaceFileContent, WorkspaceFileStamp, WorkspaceSaveResult } from '@shared/types'

/**
 * Where a document's bytes come from. The editor shell is written against
 * this, not against workspace RPCs, so every source plugs in without
 * touching the shell: a workspace file (the user's folder) and a text
 * attachment (a file the card owns in the vault) are the two today.
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
}

export function workspaceDocumentSource(brandSlug: string, streamSlug: string, projectSlug: string, path: string): DocumentSource {
  return {
    path,
    open: () => OpenWorkspaceFile(brandSlug, streamSlug, projectSlug, path),
    stat: () => StatWorkspaceFile(brandSlug, streamSlug, projectSlug, path),
    save: (content, expectedHash) => SaveWorkspaceFile(brandSlug, streamSlug, projectSlug, path, content, expectedHash),
  }
}

export function attachmentDocumentSource(cardId: string, attachmentId: string, name: string): DocumentSource {
  return {
    path: name,
    open: () => OpenCardAttachmentText(cardId, attachmentId),
    stat: () => StatCardAttachmentText(cardId, attachmentId),
    save: (content, expectedHash) => SaveCardAttachmentText(cardId, attachmentId, content, expectedHash),
  }
}
