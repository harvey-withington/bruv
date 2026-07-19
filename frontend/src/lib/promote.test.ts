import { describe, it, expect } from 'vitest'
import { promoteTargets, promoteBlockValue } from '@shared/promote'
import type { Block, ChecklistItem, SlideDeckValue } from '@shared/types'

describe('promote lattice', () => {
  it('offers the one-directional lattice list → checklist → slide_deck', () => {
    expect(promoteTargets('list')).toEqual(['checklist', 'slide_deck'])
    expect(promoteTargets('checklist')).toEqual(['slide_deck'])
    expect(promoteTargets('text')).toEqual([])
    expect(promoteTargets('slide_deck')).toEqual([])
  })

  it('promotes a list to a checklist, preserving text and reusing ids', () => {
    const block: Block = {
      id: 'b1', type: 'list', label: '', key: '',
      value: [{ id: 'li-1', text: 'One' }, { id: 'li-2', text: 'Two' }],
    }
    const out = promoteBlockValue(block, 'checklist') as ChecklistItem[]
    expect(out).toEqual([
      { id: 'li-1', text: 'One', done: false },
      { id: 'li-2', text: 'Two', done: false },
    ])
  })

  it('promotes a checklist to a slide deck of title slides', () => {
    const block: Block = {
      id: 'b1', type: 'checklist', label: '', key: '',
      value: [{ id: 'c1', text: 'Intro', done: true }, { id: 'c2', text: 'Body', done: false }],
    }
    const out = promoteBlockValue(block, 'slide_deck') as SlideDeckValue
    expect(out.currentIndex).toBe(0)
    expect(out.slides.map((s) => ({ title: s.values.title, contentTypeId: s.contentTypeId }))).toEqual([
      { title: 'Intro', contentTypeId: 'title' },
      { title: 'Body', contentTypeId: 'title' },
    ])
    expect(out.slides[0].id).toMatch(/^sld-/)
  })

  it('drops blank items and rejects conversions outside the lattice', () => {
    const block: Block = {
      id: 'b1', type: 'list', label: '', key: '',
      value: [{ id: 'li-1', text: 'Keep' }, { id: 'li-2', text: '  ' }],
    }
    expect((promoteBlockValue(block, 'checklist') as ChecklistItem[]).length).toBe(1)
    expect(promoteBlockValue(block, 'number')).toBeNull()
  })
})
