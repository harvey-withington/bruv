import * as shared from '@shared/documentSource'
import { repoRPC } from './auth'

// The mobile binding of shared/documentSource.ts: the same sources over
// repoRPC.
export type { DocumentSource, SavedAs } from '@shared/documentSource'

const call: shared.RepoCall = <T>(method: string, params: unknown[]) => repoRPC<T>(method, params)

export function workspaceDocumentSource(brandSlug: string, streamSlug: string, projectSlug: string, path: string): shared.DocumentSource {
  return shared.workspaceDocumentSource(call, brandSlug, streamSlug, projectSlug, path)
}

export function attachmentDocumentSource(cardId: string, attachmentId: string, name: string): shared.DocumentSource {
  return shared.attachmentDocumentSource(call, cardId, attachmentId, name)
}
