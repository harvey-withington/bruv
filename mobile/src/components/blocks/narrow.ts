// Type-narrowing helpers shared across the block editor components.
// Block.value is a JSON union (string | number | boolean | array of
// shapes | null), so each editor component runtime-checks before
// reading. Keeping these in one place avoids drift between blocks.

import type { Block, ChecklistItem, ListItem, MediaItem, SurveyQuestion } from '@shared/types'

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

export function asUrlValue(v: unknown): { url: string; caption?: string } {
  if (v && typeof v === 'object' && 'url' in v) {
    const obj = v as { url?: unknown; caption?: unknown }
    return {
      url: typeof obj.url === 'string' ? obj.url : '',
      caption: typeof obj.caption === 'string' ? obj.caption : undefined,
    }
  }
  return { url: '' }
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
