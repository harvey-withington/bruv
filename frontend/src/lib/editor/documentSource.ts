import { getBackend } from '@shared/adapters'
import * as shared from '@shared/documentSource'
import type { BackendAdapter } from '@shared/types'

// The desktop binding of shared/documentSource.ts: the same sources over
// the backend adapter.
export type { DocumentSource, SavedAs } from '@shared/documentSource'

const call: shared.RepoCall = <T>(method: string, params: unknown[]) =>
  (getBackend()[method as keyof BackendAdapter] as (...args: unknown[]) => Promise<T>)(...params)

export function workspaceDocumentSource(brandSlug: string, streamSlug: string, projectSlug: string, path: string): shared.DocumentSource {
  return shared.workspaceDocumentSource(call, brandSlug, streamSlug, projectSlug, path)
}

export function attachmentDocumentSource(cardId: string, attachmentId: string, name: string): shared.DocumentSource {
  return shared.attachmentDocumentSource(call, cardId, attachmentId, name)
}
