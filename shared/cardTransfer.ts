import {
  buildCardExport,
  parseCardImport,
  type BruvCardExport,
  type CardImportError,
  type EmbeddedAttachment,
} from './cardJson'
import type { Card, CardComment } from './types'

// --- Transport-agnostic card transfer ---------------------------------
//
// Owns the export-envelope building and the import replay for the
// "share a card between BRUV repos" feature. The host app injects its
// backend surface as a `CardTransferApi`: desktop binds the @shared/api
// wrappers, mobile binds repoRPC calls. Everything else is identical
// across surfaces, so it lives here once.
//
// Import notes:
// - Comment timestamps reset to "now" — AddCardComment doesn't accept
//   createdAt. Author + text are preserved.
// - Members are dropped — per-repo identity IDs would dangle.

export interface CardTransferApi {
  createCard(cardType: string, title: string): Promise<Card>
  deleteCard(cardId: string): Promise<void>
  pinCard(cardId: string, categoryId: string): Promise<void>
  updateCardType(cardId: string, cardType: string): Promise<unknown>
  updateCardDescription(cardId: string, description: string): Promise<unknown>
  updateCardBlocks(cardId: string, blocks: Card['blocks']): Promise<unknown>
  updateCardTags(cardId: string, tags: string[]): Promise<unknown>
  updateCardDueDate(cardId: string, dueDate: string): Promise<unknown>
  addCardAttachment(cardId: string, name: string, base64Data: string): Promise<unknown>
  addCardComment(cardId: string, author: string, text: string): Promise<unknown>
  listCardComments(cardId: string): Promise<CardComment[]>
  signAttachmentURL(cardId: string, attachmentId: string): Promise<string>
}

export type CardTransferError = CardImportError | 'pin_rejected'

export class ImportError extends Error {
  constructor(public readonly code: CardTransferError) {
    super(code)
    this.name = 'ImportError'
  }
}

// --- Export ------------------------------------------------------------

export async function buildCardExportPayload(api: CardTransferApi, card: Card): Promise<BruvCardExport> {
  const attachments = await fetchAttachmentsAsBase64(api, card)
  let comments: CardComment[] = []
  try { comments = (await api.listCardComments(card.id)) ?? [] }
  catch { /* comments are optional in the export — degrade to empty */ }
  return buildCardExport(card, attachments, comments, new Date().toISOString())
}

async function fetchAttachmentsAsBase64(api: CardTransferApi, card: Card): Promise<EmbeddedAttachment[]> {
  const out: EmbeddedAttachment[] = []
  for (const att of card.file_attachments ?? []) {
    const url = await api.signAttachmentURL(card.id, att.id)
    const res = await fetch(url)
    if (!res.ok) throw new Error(`fetch attachment ${att.name}: ${res.status}`)
    const buf = await res.arrayBuffer()
    out.push({
      name: att.name,
      mime: att.mime,
      size: att.size,
      data: arrayBufferToBase64(buf),
    })
  }
  return out
}

function arrayBufferToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  // Chunked encoding avoids the "too many args" stack limit on
  // String.fromCharCode for files >~100KB.
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    const slice = bytes.subarray(i, i + chunk)
    binary += String.fromCharCode.apply(null, slice as unknown as number[])
  }
  return btoa(binary)
}

// --- Import: parse + replay against the live API -----------------------

export type ImportOutcome = {
  cardId: string
  /** Names of attachments we couldn't restore. */
  failedAttachments: string[]
  /** Authors of comments we couldn't restore. */
  failedComments: string[]
}

export type ImportOptions = {
  /** Localized title used when the export's card has a blank title. */
  fallbackTitle: string
}

export async function importCardFromJson(
  api: CardTransferApi,
  text: string,
  categoryId: string,
  opts: ImportOptions,
): Promise<ImportOutcome> {
  const parsed = parseCardImport(text)
  if (!parsed.ok) throw new ImportError(parsed.error)
  const env = parsed.value

  const title = env.card.title.trim() || opts.fallbackTitle
  const created = await api.createCard(env.card.type, title)

  // Pin FIRST: Pin validates the card's type against the category's
  // accepted types, so a rejected import fails here — before blocks,
  // attachments, and comments are replayed — and we can clean up the
  // bare card instead of stranding a fully-built one off-board. It
  // also keeps any later partial failure visible in the target column.
  try {
    await api.pinCard(created.id, categoryId)
  } catch {
    try { await api.deleteCard(created.id) } catch { /* best-effort cleanup */ }
    throw new ImportError('pin_rejected')
  }

  if (env.card.type && env.card.type !== created.type) {
    await api.updateCardType(created.id, env.card.type)
  }
  if (env.card.description) {
    await api.updateCardDescription(created.id, env.card.description)
  }
  // Always replace blocks: CreateCard applies the type's template
  // blocks, and those must not survive when the export had none.
  await api.updateCardBlocks(created.id, env.card.blocks ?? [])
  if (env.card.tags?.length) {
    await api.updateCardTags(created.id, env.card.tags)
  }
  if (env.card.due_date) {
    await api.updateCardDueDate(created.id, env.card.due_date)
  }

  const failedAttachments: string[] = []
  for (const att of env.attachments) {
    try { await api.addCardAttachment(created.id, att.name, att.data) }
    catch { failedAttachments.push(att.name) }
  }

  const failedComments: string[] = []
  for (const c of env.comments) {
    try { await api.addCardComment(created.id, c.author || '', c.text) }
    catch { failedComments.push(c.author || 'unknown') }
  }

  return { cardId: created.id, failedAttachments, failedComments }
}
