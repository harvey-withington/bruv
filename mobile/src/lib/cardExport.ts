import { repoRPC } from './auth'
import {
  buildCardExport,
  parseCardImport,
  type BruvCardExport,
  type CardImportError,
  type EmbeddedAttachment,
} from '@shared/cardJson'
import type { Card, CardComment } from '@shared/types'

// Mobile mirror of frontend/src/lib/cardExport.ts. The shared envelope
// builder/parser lives in @shared/cardJson; everything here is the
// transport layer (repoRPC calls).
//
// Comment timestamps reset to "now" on import — the Wails-exposed
// AddCardComment doesn't accept createdAt. Author + text are preserved.
// Members are dropped on import (per-repo identity IDs would dangle).

export async function buildCardExportPayload(card: Card): Promise<BruvCardExport> {
  const attachments = await fetchAttachmentsAsBase64(card)
  let comments: CardComment[] = []
  try { comments = (await repoRPC<CardComment[]>('ListCardComments', [card.id])) ?? [] }
  catch { /* optional — fall through with empty */ }
  return buildCardExport(card, attachments, comments, new Date().toISOString())
}

async function fetchAttachmentsAsBase64(card: Card): Promise<EmbeddedAttachment[]> {
  const out: EmbeddedAttachment[] = []
  for (const att of card.file_attachments ?? []) {
    const url = await repoRPC<string>('SignAttachmentURL', [card.id, att.id])
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
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    const slice = bytes.subarray(i, i + chunk)
    binary += String.fromCharCode.apply(null, slice as unknown as number[])
  }
  return btoa(binary)
}

export type ImportOutcome = {
  cardId: string
  failedAttachments: string[]
  failedComments: string[]
}

export async function importCardFromJson(text: string, categoryId: string): Promise<ImportOutcome> {
  const parsed = parseCardImport(text)
  if (!parsed.ok) throw new ImportError(parsed.error)
  const env = parsed.value

  const title = env.card.title.trim() || 'Imported card'
  const created = await repoRPC<Card>('CreateCard', [env.card.type ?? '', title])

  if (env.card.type && env.card.type !== created.type) {
    await repoRPC('UpdateCardType', [created.id, env.card.type])
  }
  if (env.card.description) {
    await repoRPC('UpdateCardDescription', [created.id, env.card.description])
  }
  if (env.card.blocks?.length) {
    await repoRPC('UpdateCardBlocks', [created.id, env.card.blocks])
  }
  if (env.card.tags?.length) {
    await repoRPC('UpdateCardTags', [created.id, env.card.tags])
  }
  if (env.card.due_date) {
    await repoRPC('UpdateCardDueDate', [created.id, env.card.due_date])
  }

  const failedAttachments: string[] = []
  for (const att of env.attachments) {
    try { await repoRPC('AddCardAttachment', [created.id, att.name, att.data]) }
    catch { failedAttachments.push(att.name) }
  }

  const failedComments: string[] = []
  for (const c of env.comments) {
    try { await repoRPC('AddCardComment', [created.id, c.author || '', c.text]) }
    catch { failedComments.push(c.author || 'unknown') }
  }

  await repoRPC('PinCard', [created.id, categoryId])
  return { cardId: created.id, failedAttachments, failedComments }
}

export class ImportError extends Error {
  constructor(public readonly code: CardImportError) {
    super(code)
    this.name = 'ImportError'
  }
}

export function downloadBlob(content: string, filename: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function sanitizeFilenameStem(title: string | undefined | null): string {
  const base = (title?.trim() || 'card')
    .replace(/[\\/:*?"<>|]/g, '-')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
  return base || 'card'
}
