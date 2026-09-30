import {
  buildCardExport,
  parseCardImport,
  type BruvCardExport,
  type CardImportError,
  type EmbeddedAttachment,
} from './cardJson'
import type { Card, CardComment } from './types'
import { planCardMerge, type MergeLabels, type MergeSummary } from './cardMerge'
import { collectAttachmentRefs, remapAttachmentRefs, type AttachmentRefRemap } from './attachmentRefs'

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
// - Attachments get NEW ids on the new card, so every
//   `attachment:<card>/<id>` ref in the blocks (image/media values,
//   gallery strings, slide decks) is re-homed onto the copies before the
//   blocks are written — see shared/attachmentRefs.ts remapAttachmentRefs.

export interface CardTransferApi {
  getCard(cardId: string): Promise<Card>
  createCard(cardType: string, title: string): Promise<Card>
  deleteCard(cardId: string): Promise<void>
  pinCard(cardId: string, categoryId: string): Promise<void>
  /** null/empty result = the category accepts every type. */
  getCategoryAcceptedTypes(categoryId: string): Promise<string[] | null>
  /** Ids of every card type defined in this repo (built-in + user). */
  listCardTypeIds(): Promise<string[]>
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

export type CardTransferError = CardImportError | 'pin_failed'

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

// An embedded attachment's SOURCE id — an additive, v1-tolerated field (the
// parser keeps unknown attachment fields) that lets an import re-home the
// blocks' `attachment:<card>/<id>` refs onto the copies. Exports written
// before it existed have no id; their refs can't be mapped and are left as
// they were.
function sourceAttachmentId(att: EmbeddedAttachment): string {
  return typeof att.id === 'string' ? att.id : ''
}

async function fetchAttachmentsAsBase64(api: CardTransferApi, card: Card): Promise<EmbeddedAttachment[]> {
  const out: EmbeddedAttachment[] = []
  for (const att of card.file_attachments ?? []) {
    const url = await api.signAttachmentURL(card.id, att.id)
    const res = await fetch(url)
    if (!res.ok) throw new Error(`fetch attachment ${att.name}: ${res.status}`)
    const buf = await res.arrayBuffer()
    out.push({
      id: att.id,
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

// --- Attachment copy + ref re-homing ---------------------------------

type AttachmentIdList = Array<{ id: string }>

function attachmentsOf(result: unknown): AttachmentIdList | null {
  if (!result || typeof result !== 'object') return null
  const list = (result as { file_attachments?: unknown }).file_attachments
  if (!Array.isArray(list)) return null
  return list.filter((a): a is { id: string } => !!a && typeof (a as { id?: unknown }).id === 'string')
}

/**
 * Finds the id the server minted for an attachment just added: the first
 * id on the card we haven't seen yet (names may be de-duplicated
 * server-side, so name matching isn't reliable). Uses the AddCardAttachment
 * result when it carries the card, else re-fetches it. '' when it can't be
 * determined — refs to that attachment are then left unmapped.
 */
async function newAttachmentId(
  api: CardTransferApi,
  cardId: string,
  addResult: unknown,
  known: Set<string>,
): Promise<string> {
  let list = attachmentsOf(addResult)
  if (!list) {
    try { list = (await api.getCard(cardId)).file_attachments ?? [] }
    catch { return '' }
  }
  for (const a of list) {
    if (!known.has(a.id)) {
      known.add(a.id)
      return a.id
    }
  }
  return ''
}

/** Source attachment ids the blocks actually reference — only those need
 *  their new id resolved. */
function referencedAttachmentIds(blocks: Card['blocks']): Set<string> {
  return new Set(collectAttachmentRefs(blocks).map((r) => r.attachmentID))
}

/**
 * The remap for re-homing `blocks` onto `cardId`: `attachments` maps source
 * attachment ids to the copies', and every source card whose refs point at
 * a copied attachment maps to `cardId` (so a slide linked to the source
 * card follows the copy too).
 */
function buildRemap(blocks: Card['blocks'], attachments: Map<string, string>, cardId: string): AttachmentRefRemap {
  const cards = new Map<string, string>()
  for (const ref of collectAttachmentRefs(blocks)) {
    if (attachments.has(ref.attachmentID)) cards.set(ref.cardID, cardId)
  }
  return { attachments, cards, targetCardID: cardId }
}

function rehomeBlocks(blocks: Card['blocks'], attachments: Map<string, string>, cardId: string): Card['blocks'] {
  if (attachments.size === 0) return blocks
  return remapAttachmentRefs(blocks, buildRemap(blocks, attachments, cardId))
}

// --- Import: parse + pre-flight + replay against the live API ----------

export type ImportOutcome = {
  cardId: string
  /** Names of attachments we couldn't restore. */
  failedAttachments: string[]
  /** Authors of comments we couldn't restore. */
  failedComments: string[]
}

/**
 * Why the export's type can't be used as-is: the target category doesn't
 * accept it, or it isn't defined in this repo at all (cards are never
 * stamped with a type that doesn't exist).
 */
export type TypeConflictReason = 'not_accepted' | 'unknown'

/** The user's answer to a type conflict. */
export type TypeConflictResolution = {
  /** Substitute card type to import as; '' = import with no type. */
  type: string
  /** Merge the chosen type's template blocks into the imported blocks. */
  merge: boolean
}

export type ImportOptions = {
  /** Localized title used when the export's card has a blank title. */
  fallbackTitle: string
  /** Name of the target category, for the conflict dialog's message. */
  categoryName?: string
  /**
   * Called (pre-flight, before ANY mutation) when the export's card type
   * can't be used: the target category doesn't accept it, or the type
   * doesn't exist in this repo. The surface shows its ImportConfirm
   * dialog and resolves with the user's choice, or null to cancel —
   * cancelling aborts the import with nothing created. `acceptedTypes`
   * lists the existing types the card may take (the category's
   * restriction list, or every type when unrestricted); '' (no type) is
   * also always importable (Pin accepts typeless cards everywhere).
   */
  resolveTypeConflict?: (
    cardType: string,
    categoryName: string,
    acceptedTypes: string[],
    reason: TypeConflictReason,
  ) => Promise<TypeConflictResolution | null>
}

/**
 * Replays a parsed export against the backend. Resolves with the new
 * card's ID, or null when the user cancelled a type-conflict dialog
 * (in which case nothing was created).
 *
 * Verified backend semantics this ordering relies on (2026-07-10):
 * - CategoryAcceptsType (internal/repo/category.go): a card with type ''
 *   is accepted by EVERY category, restricted or not — so a typeless
 *   create + pin can never fail pre-flight, and "no type" is always a
 *   legal choice in the conflict dialog.
 * - card.Service.Create / UpdateType call catalog.ApplyTypeBlocks for
 *   non-empty types; mergeTemplateBlocks (core/services/catalog) is a
 *   key-matched merge into the card's EXISTING blocks: existing values
 *   win, empty existing values are filled from the template, template
 *   keys the card lacks are appended. So:
 *     merge path  = write imported blocks, THEN set the type
 *                   (template merges into the imported blocks);
 *     keep path   = set the type, THEN write imported blocks
 *                   (unconditionally — overwrites the template).
 * - Invariant: absent a user-chosen merge, the imported JSON's blocks
 *   are EXACTLY what the card ends up with (empty array included), so
 *   a deliberately block-less card can't resurrect template blocks.
 */
export async function importCardFromJson(
  api: CardTransferApi,
  text: string,
  categoryId: string,
  opts: ImportOptions,
): Promise<ImportOutcome | null> {
  const parsed = parseCardImport(text)
  if (!parsed.ok) throw new ImportError(parsed.error)
  const env = parsed.value

  // Pre-flight: check the card's type exists here and is accepted by the
  // category BEFORE any mutation, so a cancelled conflict dialog leaves
  // zero traces. The common path (known type in an unrestricted
  // category / accepted type / typeless card) proceeds with no prompt.
  const [acceptedOrNull, known] = await Promise.all([
    api.getCategoryAcceptedTypes(categoryId),
    api.listCardTypeIds(),
  ])
  const accepted = acceptedOrNull ?? []
  let importType = env.card.type
  let resolution: TypeConflictResolution | null = null
  if (env.card.type) {
    const exists = known.includes(env.card.type)
    if (!exists || (accepted.length > 0 && !accepted.includes(env.card.type))) {
      if (!opts.resolveTypeConflict) return null
      const choices = (accepted.length > 0 ? accepted : known).filter(id => known.includes(id))
      const reason: TypeConflictReason = exists ? 'not_accepted' : 'unknown'
      resolution = await opts.resolveTypeConflict(env.card.type, opts.categoryName ?? '', choices, reason)
      if (resolution === null) return null // user cancelled — nothing created
      importType = resolution.type
    }
  }

  const title = env.card.title.trim() || opts.fallbackTitle
  // On the conflict path, create typeless: the substitute type is applied
  // later at the merge/keep-ordered point. On the common path, create with
  // the original type (its template blocks are overwritten below anyway).
  const created = await api.createCard(resolution ? '' : importType, title)

  // Pin EARLY — right after create — so a transient failure later in the
  // replay leaves the partial card visible in the target column instead
  // of orphaned. Pre-flight owns type acceptance, so a pin failure here
  // is unexpected (connection loss, category deleted mid-import): clean
  // up the bare card and surface the specific pin_failed error.
  try {
    await api.pinCard(created.id, categoryId)
  } catch {
    try { await api.deleteCard(created.id) } catch { /* best-effort cleanup */ }
    throw new ImportError('pin_failed')
  }

  // Attachments BEFORE blocks: the server mints new attachment ids, and
  // the blocks' refs must point at those copies — not at the source card,
  // which may live in another vault or be deleted later.
  const sourceBlocks = env.card.blocks ?? []
  const referenced = referencedAttachmentIds(sourceBlocks)
  const seenAttachments = new Set((created.file_attachments ?? []).map((a) => a.id))
  const attachmentMap = new Map<string, string>()
  const failedAttachments: string[] = []
  for (const att of env.attachments) {
    let result: unknown
    try { result = await api.addCardAttachment(created.id, att.name, att.data) }
    catch { failedAttachments.push(att.name); continue }
    const oldId = sourceAttachmentId(att)
    if (!oldId || !referenced.has(oldId)) continue
    const newId = await newAttachmentId(api, created.id, result, seenAttachments)
    if (newId) attachmentMap.set(oldId, newId)
  }
  const blocks = rehomeBlocks(sourceBlocks, attachmentMap, created.id)

  if (env.card.description) {
    await api.updateCardDescription(created.id, env.card.description)
  }

  if (resolution && resolution.type) {
    if (resolution.merge) {
      // Merge: imported blocks first, then the type — UpdateCardType's
      // ApplyTypeBlocks merges the template into them (imported values
      // preserved, missing template fields appended).
      await api.updateCardBlocks(created.id, blocks)
      await api.updateCardType(created.id, resolution.type)
    } else {
      // Keep exactly: type first (applies its template), then the
      // imported blocks unconditionally last, overwriting the template.
      await api.updateCardType(created.id, resolution.type)
      await api.updateCardBlocks(created.id, blocks)
    }
  } else {
    // Common path (or "no type" chosen): blocks are always replaced —
    // CreateCard applied the type's template, and it must not survive
    // when the export carried different (or zero) blocks.
    if (!resolution && env.card.type && env.card.type !== created.type) {
      await api.updateCardType(created.id, env.card.type)
    }
    await api.updateCardBlocks(created.id, blocks)
  }

  if (env.card.tags?.length) {
    await api.updateCardTags(created.id, env.card.tags)
  }
  if (env.card.due_date) {
    await api.updateCardDueDate(created.id, env.card.due_date)
  }

  const failedComments: string[] = []
  for (const c of env.comments) {
    try { await api.addCardComment(created.id, c.author || '', c.text) }
    catch { failedComments.push(c.author || 'unknown') }
  }

  return { cardId: created.id, failedAttachments, failedComments }
}

// --- Merge into an existing card ---

export type MergeOutcome = {
  cardId: string
  summary: MergeSummary
  /** Attachments added (existing name+size matches are skipped). */
  attachmentsAdded: number
  /** Comments added (existing author+text matches are skipped). */
  commentsAdded: number
  failedAttachments: string[]
  failedComments: string[]
}

export type MergeCardOptions = MergeLabels

/**
 * Merges a parsed export INTO an existing card, non-destructively — see
 * shared/cardMerge.ts for the rules. The target is re-fetched immediately
 * before planning so a stale open card can't be merged against.
 * Attachments are appended first (per-item failure tracking, mirroring
 * importCardFromJson) so the merged blocks' refs point at the target's
 * copies; blocks, tags, description and due date are then each written
 * once, and only when the plan changed them; comments last.
 */
export async function mergeCardFromJson(
  api: CardTransferApi,
  text: string,
  targetCardId: string,
  opts: MergeCardOptions,
): Promise<MergeOutcome> {
  const parsed = parseCardImport(text)
  if (!parsed.ok) throw new ImportError(parsed.error)
  const env = parsed.value

  const target = await api.getCard(targetCardId)

  // Attachments first, so the merged blocks' refs can be re-homed onto the
  // target's copies (an existing name+size match counts as the copy).
  const sourceBlocks = env.card.blocks ?? []
  const referenced = referencedAttachmentIds(sourceBlocks)
  const existingAtt = new Map((target.file_attachments ?? []).map((a) => [`${a.name.toLowerCase()}|${a.size}`, a.id]))
  const seenAttachments = new Set((target.file_attachments ?? []).map((a) => a.id))
  const attachmentMap = new Map<string, string>()
  let attachmentsAdded = 0
  const failedAttachments: string[] = []
  for (const att of env.attachments) {
    const oldId = sourceAttachmentId(att)
    const existingId = existingAtt.get(`${att.name.toLowerCase()}|${att.size}`)
    if (existingId) {
      if (oldId) attachmentMap.set(oldId, existingId)
      continue
    }
    let result: unknown
    try {
      result = await api.addCardAttachment(target.id, att.name, att.data)
      attachmentsAdded++
    } catch {
      failedAttachments.push(att.name)
      continue
    }
    if (!oldId || !referenced.has(oldId)) continue
    const newId = await newAttachmentId(api, target.id, result, seenAttachments)
    if (newId) attachmentMap.set(oldId, newId)
  }

  const source = { ...env.card, blocks: rehomeBlocks(sourceBlocks, attachmentMap, target.id) }
  const plan = planCardMerge(target, source, opts)

  if (plan.blocks) await api.updateCardBlocks(target.id, plan.blocks)
  if (plan.tags) await api.updateCardTags(target.id, plan.tags)
  if (plan.description !== null) await api.updateCardDescription(target.id, plan.description)
  if (plan.dueDate !== null) await api.updateCardDueDate(target.id, plan.dueDate)

  let existingComments = new Set<string>()
  try {
    existingComments = new Set((await api.listCardComments(target.id)).map((c) => `${c.author}|${c.text.trim()}`))
  } catch { /* can't dedupe without the list — fall through and add all */ }
  let commentsAdded = 0
  const failedComments: string[] = []
  for (const c of env.comments) {
    if (existingComments.has(`${c.author || ''}|${c.text.trim()}`)) continue
    try {
      await api.addCardComment(target.id, c.author || '', c.text)
      commentsAdded++
    } catch {
      failedComments.push(c.author || 'unknown')
    }
  }

  return { cardId: target.id, summary: plan.summary, attachmentsAdded, commentsAdded, failedAttachments, failedComments }
}
