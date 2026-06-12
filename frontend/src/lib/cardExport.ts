import {
  AddCardAttachment,
  AddCardComment,
  CreateCard,
  ListCardComments,
  PinCard,
  SignAttachmentURL,
  UpdateCardBlocks,
  UpdateCardDescription,
  UpdateCardDueDate,
  UpdateCardTags,
  UpdateCardType,
} from '@shared/api'
import {
  buildCardExport,
  parseCardImport,
  type BruvCardExport,
  type CardImportError,
  type EmbeddedAttachment,
} from '@shared/cardJson'
import type { Card } from '@shared/types'

// --- Build the export envelope ---------------------------------------
//
// Fetches each attachment's bytes via the signed URL the desktop
// frontend already uses for downloads/previews, base64-encodes them,
// and packs everything into a `BruvCardExport`. Comments are pulled
// fresh from the backend since the in-memory Card doesn't carry them.

export async function buildCardExportPayload(card: Card): Promise<BruvCardExport> {
  const attachments = await fetchAttachmentsAsBase64(card)
  const comments = await ListCardComments(card.id).catch(() => [])
  return buildCardExport(card, attachments, comments, new Date().toISOString())
}

async function fetchAttachmentsAsBase64(card: Card): Promise<EmbeddedAttachment[]> {
  const out: EmbeddedAttachment[] = []
  for (const att of card.file_attachments ?? []) {
    const url = await SignAttachmentURL(card.id, att.id)
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

// --- Import: parse + replay against the live API --------------------
//
// Best-effort replay. If a single step fails partway through (e.g. an
// attachment blob is corrupt), surface the error to the caller but
// leave the partially-built card in place — restoring partial state is
// strictly better than deleting the user's title/description.

export type ImportOutcome = {
  cardId: string
  /** Names of attachments we couldn't restore. */
  failedAttachments: string[]
  /** Authors of comments we couldn't restore. */
  failedComments: string[]
}

export async function importCardFromJson(
  text: string,
  categoryId: string,
): Promise<ImportOutcome> {
  const parsed = parseCardImport(text)
  if (!parsed.ok) throw new ImportError(parsed.error)
  const env = parsed.value

  const title = env.card.title.trim() || 'Imported card'
  const created = await CreateCard(env.card.type ?? '', title) as Card

  // Type was already set by CreateCard if non-empty; re-applying is a
  // no-op so we only call when we want to *clear* it (don't bother).
  if (env.card.type && env.card.type !== created.type) {
    await UpdateCardType(created.id, env.card.type)
  }
  if (env.card.description) {
    await UpdateCardDescription(created.id, env.card.description)
  }
  if (env.card.blocks?.length) {
    await UpdateCardBlocks(created.id, env.card.blocks)
  }
  if (env.card.tags?.length) {
    await UpdateCardTags(created.id, env.card.tags)
  }
  if (env.card.due_date) {
    await UpdateCardDueDate(created.id, env.card.due_date)
  }

  const failedAttachments: string[] = []
  for (const att of env.attachments) {
    try {
      await AddCardAttachment(created.id, att.name, att.data)
    } catch {
      failedAttachments.push(att.name)
    }
  }

  const failedComments: string[] = []
  for (const c of env.comments) {
    try {
      await AddCardComment(created.id, c.author || '', c.text)
    } catch {
      failedComments.push(c.author || 'unknown')
    }
  }

  await PinCard(created.id, categoryId)

  return { cardId: created.id, failedAttachments, failedComments }
}

export class ImportError extends Error {
  constructor(public readonly code: CardImportError) {
    super(code)
    this.name = 'ImportError'
  }
}
