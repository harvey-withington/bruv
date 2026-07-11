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
  /** null/empty result = the category accepts every type. */
  getCategoryAcceptedTypes(categoryId: string): Promise<string[] | null>
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

// --- Import: parse + pre-flight + replay against the live API ----------

export type ImportOutcome = {
  cardId: string
  /** Names of attachments we couldn't restore. */
  failedAttachments: string[]
  /** Authors of comments we couldn't restore. */
  failedComments: string[]
}

/** The user's answer to a type-not-accepted conflict. */
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
   * isn't in the target category's accepted-types list. The surface shows
   * its ImportConfirm dialog and resolves with the user's choice, or null
   * to cancel — cancelling aborts the import with nothing created.
   * `acceptedTypes` is the category's restriction list; '' (no type) is
   * also always importable (Pin accepts typeless cards everywhere).
   */
  resolveTypeConflict?: (
    cardType: string,
    categoryName: string,
    acceptedTypes: string[],
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

  // Pre-flight: check the card's type against the category BEFORE any
  // mutation, so a cancelled conflict dialog leaves zero traces. The
  // common path (unrestricted category / accepted type / typeless card)
  // proceeds silently with no prompt.
  const accepted = (await api.getCategoryAcceptedTypes(categoryId)) ?? []
  let importType = env.card.type
  let resolution: TypeConflictResolution | null = null
  if (env.card.type && accepted.length > 0 && !accepted.includes(env.card.type)) {
    if (!opts.resolveTypeConflict) return null
    resolution = await opts.resolveTypeConflict(env.card.type, opts.categoryName ?? '', accepted)
    if (resolution === null) return null // user cancelled — nothing created
    importType = resolution.type
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

  if (env.card.description) {
    await api.updateCardDescription(created.id, env.card.description)
  }

  if (resolution && resolution.type) {
    if (resolution.merge) {
      // Merge: imported blocks first, then the type — UpdateCardType's
      // ApplyTypeBlocks merges the template into them (imported values
      // preserved, missing template fields appended).
      await api.updateCardBlocks(created.id, env.card.blocks ?? [])
      await api.updateCardType(created.id, resolution.type)
    } else {
      // Keep exactly: type first (applies its template), then the
      // imported blocks unconditionally last, overwriting the template.
      await api.updateCardType(created.id, resolution.type)
      await api.updateCardBlocks(created.id, env.card.blocks ?? [])
    }
  } else {
    // Common path (or "no type" chosen): blocks are always replaced —
    // CreateCard applied the type's template, and it must not survive
    // when the export carried different (or zero) blocks.
    if (!resolution && env.card.type && env.card.type !== created.type) {
      await api.updateCardType(created.id, env.card.type)
    }
    await api.updateCardBlocks(created.id, env.card.blocks ?? [])
  }

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
