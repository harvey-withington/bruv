import { BLOCK_TYPES } from './types'
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
  | 'invalid_blocks'

export type CardImportResult =
  | { ok: true; value: BruvCardExport }
  | { ok: false; error: CardImportError }

export function parseCardImport(text: string): CardImportResult {
  let parsed: unknown
  try { parsed = JSON.parse(text) }
  catch { return { ok: false, error: 'not_json' } }

  if (!isObject(parsed)) return { ok: false, error: 'not_json' }
  if (parsed.format !== CARD_JSON_FORMAT) return { ok: false, error: 'wrong_format' }
  if (
    typeof parsed.version !== 'number'
    || !Number.isInteger(parsed.version)   // rejects NaN and fractional versions
    || parsed.version < 1
    || parsed.version > CARD_JSON_VERSION
  ) {
    return { ok: false, error: 'unsupported_version' }
  }
  if (!isObject(parsed.card)) return { ok: false, error: 'missing_card' }
  if (typeof parsed.card.title !== 'string') return { ok: false, error: 'invalid_title' }

  // Normalise: tolerate missing optional fields rather than failing.
  const card = parsed.card as Partial<ExportedCard> & { title: string }

  // Blocks are the one field we validate strictly instead of dropping:
  // silently discarding a corrupt or unknown block would import a card
  // that LOOKS complete but lost data. A missing/absent field still
  // normalises to [], but a present-and-malformed one fails the parse.
  let blocks: Card['blocks'] = []
  if (card.blocks !== undefined && card.blocks !== null) {
    if (!Array.isArray(card.blocks) || !card.blocks.every(isValidBlock)) {
      return { ok: false, error: 'invalid_blocks' }
    }
    blocks = card.blocks
  }

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
      blocks,
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

const KNOWN_BLOCK_TYPES = new Set<string>(BLOCK_TYPES)

// Shape gate for imported blocks: id/type must be strings and the type
// must be one the model knows — anything else fails the parse with
// 'invalid_blocks' rather than injecting arbitrary JSON as a block.
// Values stay untyped (the Block value union is too broad to validate
// cheaply); renderers already treat block values defensively.
function isValidBlock(v: unknown): v is Card['blocks'][number] {
  return isObject(v)
    && typeof v.id === 'string'
    && typeof v.type === 'string'
    && KNOWN_BLOCK_TYPES.has(v.type)
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
