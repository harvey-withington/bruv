import { describe, it, expect } from 'vitest'
import { applyChecklistCrossMove, asChecklist } from './narrow'
import type { Block, ChecklistItem } from '@shared/types'

// REGRESSION (2026-08-09): dragging a checklist item onto ANOTHER
// checklist block used to be handled as a same-list reorder — the item
// stayed in the source block (at a destination-computed index) and the
// destination never heard about the drop. A retry then "looked" right
// because the no-op left the manually-moved DOM node in place, but
// nothing was persisted. These tests pin the real cross-block move.

function item(id: string, text: string, done = false): ChecklistItem {
  return { id, text, done }
}

function checklistBlock(id: string, items: ChecklistItem[]): Block {
  return { id, type: 'checklist', label: id, value: items } as Block
}

function makeBlocks(): Block[] {
  return [
    checklistBlock('src', [item('a', 'Alpha'), item('b', 'Bravo', true), item('c', 'Charlie')]),
    { id: 'txt', type: 'text', label: 'Notes', value: 'hello' } as Block,
    checklistBlock('dst', [item('x', 'X-ray'), item('y', 'Yankee')]),
  ]
}

describe('applyChecklistCrossMove', () => {
  it('moves the item out of the source and into the destination at the index', () => {
    const next = applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'b', toBlockID: 'dst', toPosition: 1,
    })
    expect(next).not.toBeNull()
    expect(asChecklist(next![0].value).map((i) => i.id)).toEqual(['a', 'c'])
    expect(asChecklist(next![2].value).map((i) => i.id)).toEqual(['x', 'b', 'y'])
    // The moved item keeps its state (done flag, text).
    expect(asChecklist(next![2].value)[1]).toEqual(item('b', 'Bravo', true))
  })

  it('both blocks change in the SAME returned array (single-save contract)', () => {
    const blocks = makeBlocks()
    const next = applyChecklistCrossMove(blocks, 'src', {
      itemID: 'a', toBlockID: 'dst', toPosition: 0,
    })!
    // One array, both halves of the move present — persisting `next` in
    // one UpdateCardBlocks call can never race itself.
    expect(next).toHaveLength(3)
    expect(asChecklist(next[0].value)).toHaveLength(2)
    expect(asChecklist(next[2].value)).toHaveLength(3)
  })

  it('never mutates the input', () => {
    const blocks = makeBlocks()
    applyChecklistCrossMove(blocks, 'src', { itemID: 'a', toBlockID: 'dst', toPosition: 0 })
    expect(asChecklist(blocks[0].value).map((i) => i.id)).toEqual(['a', 'b', 'c'])
    expect(asChecklist(blocks[2].value).map((i) => i.id)).toEqual(['x', 'y'])
  })

  it('clamps an out-of-range position to the destination end', () => {
    const next = applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'a', toBlockID: 'dst', toPosition: 99,
    })!
    expect(asChecklist(next[2].value).map((i) => i.id)).toEqual(['x', 'y', 'a'])
  })

  it('clamps a negative position to the destination start', () => {
    const next = applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'a', toBlockID: 'dst', toPosition: -1,
    })!
    expect(asChecklist(next[2].value).map((i) => i.id)).toEqual(['a', 'x', 'y'])
  })

  it('returns null when the destination is not a checklist', () => {
    expect(applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'a', toBlockID: 'txt', toPosition: 0,
    })).toBeNull()
  })

  it('returns null for unknown source, destination, or item', () => {
    expect(applyChecklistCrossMove(makeBlocks(), 'nope', {
      itemID: 'a', toBlockID: 'dst', toPosition: 0,
    })).toBeNull()
    expect(applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'a', toBlockID: 'nope', toPosition: 0,
    })).toBeNull()
    expect(applyChecklistCrossMove(makeBlocks(), 'src', {
      itemID: 'nope', toBlockID: 'dst', toPosition: 0,
    })).toBeNull()
  })

  it('moving into an empty destination checklist works', () => {
    const blocks = [
      checklistBlock('src', [item('a', 'Alpha')]),
      checklistBlock('dst', []),
    ]
    const next = applyChecklistCrossMove(blocks, 'src', {
      itemID: 'a', toBlockID: 'dst', toPosition: 0,
    })!
    expect(asChecklist(next[0].value)).toEqual([])
    expect(asChecklist(next[1].value).map((i) => i.id)).toEqual(['a'])
  })
})
