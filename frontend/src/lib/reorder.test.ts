import { describe, it, expect } from 'vitest'
import { computeReorder, wouldReorder, DROP_END } from './reorder'

// Fixture items with stable ids in render order
const a = { id: 'a', label: 'A' }
const b = { id: 'b', label: 'B' }
const c = { id: 'c', label: 'C' }
const d = { id: 'd', label: 'D' }

describe('computeReorder — move', () => {
  it('moves later item earlier', () => {
    const out = computeReorder([a, b, c, d], 'c', 'a', { mode: 'move' })
    expect(out.map(x => x.id)).toEqual(['c', 'a', 'b', 'd'])
  })

  it('moves earlier item later', () => {
    const out = computeReorder([a, b, c, d], 'a', 'd', { mode: 'move' })
    expect(out.map(x => x.id)).toEqual(['b', 'c', 'a', 'd'])
  })

  it('moves to end via DROP_END', () => {
    const out = computeReorder([a, b, c, d], 'b', DROP_END, { mode: 'move' })
    expect(out.map(x => x.id)).toEqual(['a', 'c', 'd', 'b'])
  })

  it('returns original array (===) when dropping on self', () => {
    const input = [a, b, c]
    const out = computeReorder(input, 'b', 'b', { mode: 'move' })
    expect(out).toBe(input)
  })

  it('returns original array (===) when dropping directly below self', () => {
    const input = [a, b, c]
    const out = computeReorder(input, 'a', 'b', { mode: 'move' })
    expect(out).toBe(input)
  })

  it('returns original array when draggedId is missing', () => {
    const input = [a, b, c]
    const out = computeReorder(input, 'ghost', 'a', { mode: 'move' })
    expect(out).toBe(input)
  })

  it('returns original array when dropBeforeId is missing', () => {
    const input = [a, b, c]
    const out = computeReorder(input, 'a', 'ghost', { mode: 'move' })
    expect(out).toBe(input)
  })

  it('handles moving the last item forward', () => {
    const out = computeReorder([a, b, c, d], 'd', 'b', { mode: 'move' })
    expect(out.map(x => x.id)).toEqual(['a', 'd', 'b', 'c'])
  })
})

describe('wouldReorder — guards against misleading drop indicators', () => {
  it('returns false when dropping onto self (upper half)', () => {
    expect(wouldReorder([a, b, c], 'b', 'b', 'move')).toBe(false)
  })

  it('returns false when dropping directly below self', () => {
    // dragging B, dropBeforeId = C → slot is the same as B's current spot
    expect(wouldReorder([a, b, c], 'b', 'c', 'move')).toBe(false)
  })

  it('returns false when dragging the last block to its own end slot', () => {
    // Harvey's reported bug: dragging C (second-to-last) over top half of D
    // reports DROP_END as a move target when C is already last — no-op.
    // Same shape happens for D → DROP_END.
    expect(wouldReorder([a, b, c], 'c', DROP_END, 'move')).toBe(false)
  })

  it('returns true for real forward move', () => {
    expect(wouldReorder([a, b, c, d], 'a', 'd', 'move')).toBe(true)
  })

  it('returns true for real backward move', () => {
    expect(wouldReorder([a, b, c], 'c', 'a', 'move')).toBe(true)
  })

  it('returns true when moving a middle block to end', () => {
    expect(wouldReorder([a, b, c, d], 'b', DROP_END, 'move')).toBe(true)
  })

  it('copy is always effective, even onto self', () => {
    expect(wouldReorder([a, b, c], 'b', 'b', 'copy')).toBe(true)
    expect(wouldReorder([a, b, c], 'c', DROP_END, 'copy')).toBe(true)
  })

  it('returns false for unknown ids', () => {
    expect(wouldReorder([a, b, c], 'ghost', 'a', 'move')).toBe(false)
    expect(wouldReorder([a, b, c], 'a', 'ghost', 'move')).toBe(false)
  })
})

describe('computeReorder — copy', () => {
  it('inserts duplicate with generated id; original unchanged', () => {
    const out = computeReorder([a, b, c], 'a', 'c', {
      mode: 'copy',
      newId: () => 'dup-1',
    })
    expect(out.map(x => x.id)).toEqual(['a', 'b', 'dup-1', 'c'])
  })

  it('copies to end via DROP_END', () => {
    const out = computeReorder([a, b], 'a', DROP_END, {
      mode: 'copy',
      newId: () => 'dup-2',
    })
    expect(out.map(x => x.id)).toEqual(['a', 'b', 'dup-2'])
  })

  it('copy-onto-self inserts a duplicate (not a no-op)', () => {
    // Unlike move, copying "before self" is a real operation
    const out = computeReorder([a, b], 'a', 'a', {
      mode: 'copy',
      newId: () => 'dup-3',
    })
    expect(out.map(x => x.id)).toEqual(['dup-3', 'a', 'b'])
  })

  it('preserves other fields from the original', () => {
    const out = computeReorder([a, b], 'a', DROP_END, {
      mode: 'copy',
      newId: () => 'dup-4',
    })
    const dup = out.find(x => x.id === 'dup-4')
    expect(dup?.label).toBe('A')
  })

  it('falls back to "<id>-copy" when no newId is provided', () => {
    const out = computeReorder([a, b], 'a', DROP_END, { mode: 'copy' })
    expect(out.map(x => x.id)).toEqual(['a', 'b', 'a-copy'])
  })
})
