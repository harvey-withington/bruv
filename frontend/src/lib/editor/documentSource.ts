import { OpenWorkspaceFile, SaveWorkspaceFile, StatWorkspaceFile } from '@shared/api'
import type { WorkspaceFileContent, WorkspaceFileStamp, WorkspaceSaveResult } from '@shared/types'

/**
 * Where a document's bytes come from. The editor shell is written against
 * this, not against workspace RPCs, so a second source (Card Documents'
 * managed `documents/<cardID>/` files) plugs in without touching the shell.
 *
 * Stamps are the divergence guard: `open` hands one out, `save` presents
 * it back and reports `diverged` (nothing written) when the file changed
 * under us; an empty `expectedHash` overwrites.
 */
export interface DocumentSource {
  /** Path as shown to the user (workspace-relative). */
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
