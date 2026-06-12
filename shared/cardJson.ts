import type { Card, CardComment } from './types'

// --- Card ⇄ JSON envelope (lossless, round-trippable) ---
//
// This is the canonical "share a card between BRUV repos" format. Unlike
// `cardMarkdown.ts` (one-way human-readable export), this is meant to be
// re-imported with full fidelity: blocks, attachments (embedded as
// base64), comments, tags, type, due date.
//
// On import we MINT new IDs for the card, attachments, and comments —
// importing the same file twice gives two distinct cards rather than
// overwriting. Members are exported for reference but NOT restored on
// import: member IDs are per-repo identities and would dangle.
//
// Version bumps are reserved for breaking schema changes — additive
// fields stay at v1 and are tolerated by the validator.

export const CARD_JSON_FORMAT = 'bruv-card'
export const CARD_JSON_VERSION = 1

export type EmbeddedAttachment = {
  name: string
  mime: string
  size: number
  /** Base64 string WITHOUT the data-URL prefix. */
  data: string
}

export type ExportedComment = {
  author: string
  created_at: string
  updated_at?: string
  text: string
}

export type ExportedCard = {
  title: string
  description: string
  type: string
  tags: string[]
  due_date: string | null
  blocks: Card['blocks']
  members?: string[]
}

export type BruvCardExport = {
  format: typeof CARD_JSON_FORMAT
  version: number
  exported_at: string
  card: ExportedCard
  attachments: EmbeddedAttachment[]
  comments: ExportedComment[]
}

export function buildCardExport(
  card: Card,
  attachments: EmbeddedAttachment[],
  comments: CardComment[],
  exportedAt: string,
): BruvCardExport {
  return {
    format: CARD_JSON_FORMAT,
    version: CARD_JSON_VERSION,
    exported_at: exportedAt,
    card: {
      title: card.title,
      description: card.description,
      type: card.type,
      tags: [...(card.tags ?? [])],
      due_date: card.due_date,
      blocks: card.blocks ?? [],
      members: card.members ? [...card.members] : undefined,
    },
    attachments,
    comments: comments.map(c => ({
      author: c.author,
      created_at: c.created_at,
      updated_at: c.updated_at,
      text: c.text,
    })),
  }
}

export type CardImportError =
  | 'not_json'
  | 'wrong_format'
  | 'unsupported_version'
  | 'missing_card'
  | 'invalid_title'

export type CardImportResult =
  | { ok: true; value: BruvCardExport }
  | { ok: false; error: CardImportError }

export function parseCardImport(text: string): CardImportResult {
  let parsed: unknown
  try { parsed = JSON.parse(text) }
  catch { return { ok: false, error: 'not_json' } }

  if (!isObject(parsed)) return { ok: false, error: 'not_json' }
  if (parsed.format !== CARD_JSON_FORMAT) return { ok: false, error: 'wrong_format' }
  if (typeof parsed.version !== 'number' || parsed.version > CARD_JSON_VERSION) {
    return { ok: false, error: 'unsupported_version' }
  }
  if (!isObject(parsed.card)) return { ok: false, error: 'missing_card' }
  if (typeof parsed.card.title !== 'string') return { ok: false, error: 'invalid_title' }

  // Normalise: tolerate missing optional fields rather than failing.
  const card = parsed.card as Partial<ExportedCard> & { title: string }
  const value: BruvCardExport = {
    format: CARD_JSON_FORMAT,
    version: parsed.version,
    exported_at: typeof parsed.exported_at === 'string' ? parsed.exported_at : '',
    card: {
      title: card.title,
      description: typeof card.description === 'string' ? card.description : '',
      type: typeof card.type === 'string' ? card.type : '',
      tags: Array.isArray(card.tags) ? card.tags.filter(t => typeof t === 'string') : [],
      due_date: typeof card.due_date === 'string' ? card.due_date : null,
      blocks: Array.isArray(card.blocks) ? card.blocks as Card['blocks'] : [],
      members: Array.isArray(card.members) ? card.members.filter(m => typeof m === 'string') : undefined,
    },
    attachments: Array.isArray(parsed.attachments)
      ? parsed.attachments.filter(isEmbeddedAttachment)
      : [],
    comments: Array.isArray(parsed.comments)
      ? parsed.comments.filter(isExportedComment)
      : [],
  }
  return { ok: true, value }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isEmbeddedAttachment(v: unknown): v is EmbeddedAttachment {
  return isObject(v)
    && typeof v.name === 'string'
    && typeof v.mime === 'string'
    && typeof v.size === 'number'
    && typeof v.data === 'string'
}

function isExportedComment(v: unknown): v is ExportedComment {
  return isObject(v)
    && typeof v.author === 'string'
    && typeof v.created_at === 'string'
    && typeof v.text === 'string'
}
