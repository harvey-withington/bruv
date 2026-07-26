// Row-list display label for a slide. Both deck row lists (desktop editor,
// mobile read-only list) resolve labels through here so the priority order
// stays identical:
//
//   1. slide.title — manual override typed in the slide editor
//   2. the linked card's live title — the caller supplies it (each surface
//      has its own card lookup/cache), so renaming the card renames the row
//   3. first non-empty literal field value; `platform` is routing data,
//      never a label
//   4. null — the caller falls back to its localized content-type name
//
// Never rendered on the slide itself; presentation output is unaffected.

import type { Slide } from './types'
import { resolveContentType } from './slideContentTypes'

export function slideDisplayLabel(slide: Slide, linkedCardTitle?: string): string | null {
  if (slide.title?.trim()) return slide.title.trim()
  if (slide.cardId && linkedCardTitle?.trim()) return linkedCardTitle.trim()
  const ct = resolveContentType(slide.contentTypeId)
  if (!ct) return null
  for (const f of ct.fields) {
    if (f.key === 'platform') continue
    const v = slide.values?.[f.key]
    if (v && v.trim()) return v.trim()
  }
  return null
}
