import { absoluteAttachmentURL } from '@shared/download'
import { readEnrolment, repoRPC } from './auth'

/**
 * A URL the browser can load for an attachment: a freshly signed server
 * URL (the RPC returns it server-relative, so the enrolled origin is
 * prepended), or — for a card kept offline — the device copy's blob: URL.
 */
export async function signAttachmentURL(cardId: string, attachmentId: string): Promise<string> {
  const path = await repoRPC<string>('SignAttachmentURL', [cardId, attachmentId])
  const enrol = readEnrolment()
  return enrol ? absoluteAttachmentURL(enrol.serverURL, path) : path
}
