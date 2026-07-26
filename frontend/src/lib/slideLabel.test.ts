import { describe, it, expect } from 'vitest'
import { slideDisplayLabel } from '@shared/slideLabel'
import type { Slide } from '@shared/types'

function slide(partial: Partial<Slide>): Slide {
  return { id: 'sld-1', contentTypeId: 'title', values: {}, ...partial }
}

describe('slideDisplayLabel', () => {
  it('prefers the explicit title override over everything else', () => {
    const s = slide({ title: '  Override  ', cardId: 'c1', values: { title: 'Literal' } })
    expect(slideDisplayLabel(s, 'Card title')).toBe('Override')
  })

  it('follows the linked card title when no override is set', () => {
    const s = slide({ cardId: 'c1', values: { title: 'Literal' } })
    expect(slideDisplayLabel(s, 'Card title')).toBe('Card title')
  })

  it('ignores the card title when the slide has no linked card', () => {
    const s = slide({ values: { title: 'Literal' } })
    expect(slideDisplayLabel(s, 'Card title')).toBe('Literal')
  })

  it('falls back to the first non-empty literal value while the card title loads', () => {
    const s = slide({ cardId: 'c1', values: { title: '', subtitle: 'Second line' } })
    expect(slideDisplayLabel(s, undefined)).toBe('Second line')
  })

  it('never uses platform as a label', () => {
    const s = slide({ contentTypeId: 'post', cardId: 'c1', values: { platform: 'twitter' } })
    expect(slideDisplayLabel(s, undefined)).toBeNull()
  })

  it('returns null for an unknown content type so callers can localize the fallback', () => {
    const s = slide({ contentTypeId: 'mystery', values: { anything: 'x' } })
    expect(slideDisplayLabel(s, undefined)).toBeNull()
  })
})
