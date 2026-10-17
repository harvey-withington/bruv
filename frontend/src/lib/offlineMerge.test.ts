import { describe, it, expect } from 'vitest'
import { merge3, matchMap } from '@shared/offline/diff3'
import { mergeCard, cardEditBetween, sameDueDate } from '@shared/offline/merge'
import type { Block, Card } from '@shared/types'

const lines = (...l: string[]) => l.join('\n')

describe('merge3', () => {
  it('takes the only side that changed', () => {
    expect(merge3('a', 'b', 'a')).toEqual({ clean: true, text: 'b' })
    expect(merge3('a', 'a', 'c')).toEqual({ clean: true, text: 'c' })
    expect(merge3('a', 'x', 'x')).toEqual({ clean: true, text: 'x' })
  })

  it('merges edits to different regions', () => {
    const base = lines('# Title', 'intro', 'middle', 'end')
    const mine = lines('# Title', 'intro, rewritten on the plane', 'middle', 'end')
    const theirs = lines('# Title', 'intro', 'middle', 'end', 'agent: reviewed')
    expect(merge3(base, mine, theirs)).toEqual({
      clean: true,
      text: lines('# Title', 'intro, rewritten on the plane', 'middle', 'end', 'agent: reviewed'),
    })
  })

  it('handles insertions and deletions on both sides', () => {
    const base = lines('a', 'b', 'c', 'd', 'e')
    const mine = lines('a', 'new', 'b', 'c', 'e')       // insert after a, delete d
    const theirs = lines('a', 'b', 'c', 'd', 'e', 'f')   // append f
    expect(merge3(base, mine, theirs)).toEqual({ clean: true, text: lines('a', 'new', 'b', 'c', 'e', 'f') })
  })

  it('reports a conflict when both sides change the same line differently', () => {
    expect(merge3(lines('a', 'b', 'c'), lines('a', 'B1', 'c'), lines('a', 'B2', 'c'))).toEqual({ clean: false })
  })

  it('treats a whole-text rewrite against an edit as a conflict', () => {
    expect(merge3('one\ntwo', 'completely different', 'one\ntwo!').clean).toBe(false)
  })

  it('matchMap finds an increasing longest match', () => {
    const map = matchMap(['a', 'b', 'c', 'd'], ['x', 'a', 'c', 'd', 'y'])
    expect(map).toEqual([1, -1, 2, 3])
  })

  it('merges a long document with scattered edits', () => {
    const base = Array.from({ length: 400 }, (_, i) => `line ${i}`)
    const mine = [...base]; mine[10] = 'mine 10'; mine.splice(200, 0, 'mine inserted')
    const theirs = [...base]; theirs[350] = 'theirs 350'; theirs.splice(5, 1)
    const want = [...mine]; want[want.indexOf('line 350')] = 'theirs 350'; want.splice(5, 1)
    expect(merge3(base.join('\n'), mine.join('\n'), theirs.join('\n'))).toEqual({ clean: true, text: want.join('\n') })
  })
})

function card(over: Partial<Card> = {}): Card {
  return {
    id: 'c1', title: 'Post', description: '', type: 'blog', tags: [], due_date: null,
    created_at: '2026-10-01T00:00:00Z', blocks: [], file_attachments: [], rev: 3, ...over,
  }
}
const text = (id: string, value: string, label = id): Block => ({ id, type: 'text', label, key: '', value })
const checklist = (id: string, items: Array<[string, string, boolean]>): Block => ({
  id, type: 'checklist', label: id, key: '', value: items.map(([iid, t, done]) => ({ id: iid, text: t, done })),
})

describe('mergeCard', () => {
  it('merges fields changed on different sides', () => {
    const base = card()
    const mine = card({ title: 'Post (draft 2)', tags: ['blog', 'travel'] })
    const theirs = card({ due_date: '2026-11-01T00:00:00Z', tags: ['blog'], rev: 5 })
    const base2 = { ...base, tags: ['blog'] }
    const { card: merged, conflicts } = mergeCard(base2, mine, theirs)
    expect(conflicts).toEqual([])
    expect(merged.title).toBe('Post (draft 2)')
    expect(merged.due_date).toBe('2026-11-01T00:00:00Z')
    expect(merged.tags).toEqual(['blog', 'travel'])
    expect(merged.rev).toBe(5)
  })

  it('reports conflicting titles and keeps the server value meanwhile', () => {
    const { card: merged, conflicts } = mergeCard(card(), card({ title: 'Mine' }), card({ title: 'Theirs' }))
    expect(conflicts.map((c) => c.id)).toEqual(['title'])
    expect(merged.title).toBe('Theirs')
  })

  it('tag set: removals on one side and additions on the other both land', () => {
    const base = card({ tags: ['a', 'b', 'c'] })
    const { card: merged } = mergeCard(base, card({ tags: ['a', 'c', 'mine'] }), card({ tags: ['a', 'b', 'c', 'theirs'] }))
    expect(merged.tags).toEqual(['a', 'c', 'theirs', 'mine'])
  })

  it('merges blocks by id: edits, additions and an untouched deletion', () => {
    const base = card({ blocks: [text('b1', 'one'), text('b2', 'two'), text('b3', 'three')] })
    const mine = card({ blocks: [text('b1', 'one, edited'), text('b2', 'two'), text('new', 'added offline'), text('b3', 'three')] })
    const theirs = card({ blocks: [text('b1', 'one'), text('b3', 'three'), text('agent', 'status: done')] })
    const { card: merged, conflicts } = mergeCard(base, mine, theirs)
    expect(conflicts).toEqual([])
    expect(merged.blocks.map((b) => [b.id, b.value])).toEqual([
      ['b1', 'one, edited'], ['new', 'added offline'], ['b3', 'three'], ['agent', 'status: done'],
    ])
  })

  it('a block deleted on one side but edited on the other is a conflict', () => {
    const base = card({ blocks: [text('b1', 'one')] })
    const r1 = mergeCard(base, card({ blocks: [] }), card({ blocks: [text('b1', 'one, changed at home')] }))
    expect(r1.conflicts).toEqual([{ id: 'block:b1', kind: 'block', label: 'b1', mine: null, theirs: text('b1', 'one, changed at home') }])
    expect(r1.card.blocks).toHaveLength(1)
    const r2 = mergeCard(base, card({ blocks: [text('b1', 'edited offline')] }), card({ blocks: [] }))
    expect(r2.conflicts[0].theirs).toBeNull()
    expect(r2.card.blocks).toHaveLength(0)
  })

  it('merges text blocks line by line and checklists item by item', () => {
    const base = card({ blocks: [text('t', 'a\nb\nc'), checklist('cl', [['i1', 'pack', false], ['i2', 'book', false]])] })
    const mine = card({ blocks: [text('t', 'A\nb\nc'), checklist('cl', [['i1', 'pack', true], ['i2', 'book', false], ['i3', 'write', false]])] })
    const theirs = card({ blocks: [text('t', 'a\nb\nC'), checklist('cl', [['i1', 'pack', false], ['i2', 'book', true]])] })
    const { card: merged, conflicts } = mergeCard(base, mine, theirs)
    expect(conflicts).toEqual([])
    expect(merged.blocks[0].value).toBe('A\nb\nC')
    expect(merged.blocks[1].value).toEqual([
      { id: 'i1', text: 'pack', done: true }, { id: 'i2', text: 'book', done: true }, { id: 'i3', text: 'write', done: false },
    ])
  })

  it('follows a reorder made on only one side', () => {
    const base = card({ blocks: [text('a', '1'), text('b', '2'), text('c', '3')] })
    const mine = card({ blocks: [text('c', '3'), text('a', '1'), text('b', '2')] })
    const theirs = card({ blocks: [text('a', '1'), text('b', '2 changed'), text('c', '3')] })
    expect(mergeCard(base, mine, theirs).card.blocks.map((b) => b.id)).toEqual(['c', 'a', 'b'])
  })

  it('ignores object key order when comparing', () => {
    const base = card({ blocks: [{ id: 'b', type: 'text', label: 'L', key: '', value: 'v', meta: { min: 1, max: 2 } }] })
    const reordered = card({ blocks: [{ value: 'v', meta: { max: 2, min: 1 }, key: '', label: 'L', type: 'text', id: 'b' }] })
    expect(mergeCard(base, reordered, base).conflicts).toEqual([])
  })
})

describe('cardEditBetween', () => {
  it('carries only the changed parts', () => {
    const from = card({ description: 'old', due_date: '2026-11-01T00:00:00Z' })
    expect(cardEditBetween(from, card({ description: 'new', due_date: '2026-11-01' }))).toEqual({ description: 'new' })
    expect(cardEditBetween(from, card({ description: 'old', due_date: null }))).toEqual({ due_date: '' })
    expect(cardEditBetween(from, from)).toEqual({})
  })

  it('sameDueDate compares instants, not strings', () => {
    expect(sameDueDate('2026-11-01', '2026-11-01T00:00:00Z')).toBe(true)
    expect(sameDueDate(null, '')).toBe(true)
    expect(sameDueDate('2026-11-01', null)).toBe(false)
  })
})
