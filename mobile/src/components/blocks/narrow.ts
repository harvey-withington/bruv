// Type-narrowing helpers shared across the block editor components.
// Block.value is a JSON union (string | number | boolean | array of
// shapes | null), so each editor component runtime-checks before
// reading. Keeping these in one place avoids drift between blocks.

import type { Block, ChecklistItem, ListItem, MediaItem, SurveyQuestion, Slide, SlideDeckValue } from '@shared/types'

export function asString(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

export function asNumber(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null
}

export function asBool(v: unknown): boolean {
  return v === true
}

export function asStringArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}

export function asChecklist(v: unknown): ChecklistItem[] {
  if (!Array.isArray(v)) return []
  return v.filter((x): x is ChecklistItem =>
    !!x && typeof x === 'object' && typeof (x as ChecklistItem).id === 'string',
  )
}

export function asList(v: unknown): ListItem[] {
  if (!Array.isArray(v)) return []
  return v.filter((x): x is ListItem =>
    !!x && typeof x === 'object' && typeof (x as ListItem).id === 'string',
  )
}

export function asMedia(v: unknown): MediaItem[] {
  if (!Array.isArray(v)) return []
  return v.filter((x): x is MediaItem =>
    !!x && typeof x === 'object' && typeof (x as MediaItem).id === 'string',
  )
}

export function asSurveyQuestions(v: unknown): SurveyQuestion[] {
  if (!Array.isArray(v)) return []
  return v.filter((x): x is SurveyQuestion =>
    !!x && typeof x === 'object' && typeof (x as SurveyQuestion).id === 'string',
  )
}

export function asSlideDeck(v: unknown): SlideDeckValue {
  if (v && typeof v === 'object' && !Array.isArray(v) && 'slides' in v) {
    const obj = v as { slides?: unknown }
    const slides = Array.isArray(obj.slides)
      ? obj.slides.filter((s): s is Slide => !!s && typeof s === 'object' && typeof (s as Slide).id === 'string')
      : []
    return { slides }
  }
  return { slides: [] }
}

// asUrlValue lives in shared/ now — desktop needs the same narrowing,
// and this copy silently dropped legacy STRING values (url blocks saved
// by older desktop edits rendered empty here).
export { asUrlValue } from '@shared/blockValues'

/** A checklist item dropped onto a DIFFERENT checklist block. Emitted by
 *  ChecklistBlock's drag handler; one block's onChange can't edit a
 *  sibling block, so the blocks host (CardPage) performs the move and
 *  persists both blocks in a single save. */
export interface ChecklistCrossMove {
  itemID: string
  toBlockID: string
  /** Insertion index within the destination checklist. */
  toPosition: number
}

/** Apply a cross-block checklist move to a blocks array. Returns the new
 *  array, or null when the move can't apply (unknown blocks, destination
 *  not a checklist, item missing) — callers skip the save then. Pure:
 *  never mutates the input; source and destination change in the SAME
 *  returned array so the caller can persist both in one save. */
export function applyChecklistCrossMove(
  blocks: Block[],
  fromBlockID: string,
  move: ChecklistCrossMove,
): Block[] | null {
  const from = blocks.find((b) => b.id === fromBlockID)
  const to = blocks.find((b) => b.id === move.toBlockID)
  if (!from || !to || to.type !== 'checklist') return null
  const fromItems = asChecklist(from.value)
  const moved = fromItems.find((it) => it.id === move.itemID)
  if (!moved) return null
  const toItems = asChecklist(to.value)
  const insertAt = Math.max(0, Math.min(move.toPosition, toItems.length))
  const nextTo = [...toItems]
  nextTo.splice(insertAt, 0, moved)
  const nextFrom = fromItems.filter((it) => it.id !== move.itemID)
  return blocks.map((b) => {
    if (b.id === from.id) return { ...b, value: nextFrom }
    if (b.id === to.id) return { ...b, value: nextTo }
    return b
  })
}

/** Construct a copy of `block` with a new value. Helper for editors that
 *  fire onChange — keeps immutability tidy and centralises any future
 *  shape-level validation. */
export function withValue<T extends Block['value']>(block: Block, value: T): Block {
  return { ...block, value }
}

/** Construct a copy of `block` with a partially-merged meta. Editors
 *  that mutate `meta` (e.g. AlarmBlock, ProgressBlock) call this. */
export function withMeta(block: Block, meta: Block['meta']): Block {
  return { ...block, meta: { ...(block.meta ?? {}), ...(meta ?? {}) } }
}

/** Stable random ID for new list / checklist items. crypto.randomUUID
 *  is available in all modern browsers including Android Chrome and
 *  iOS Safari 16.4+ (the same baseline as Web Push), so safe to use
 *  without a polyfill. */
export function newID(): string {
  return crypto.randomUUID()
}
