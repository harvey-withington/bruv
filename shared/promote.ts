// Promote lattice for multi-item blocks: convert a block in place to a more
// structured type, preserving item text (and ids where the id namespaces
// allow). The lattice is one-directional — an idea "gets legs":
//
//   list → checklist → slide_deck
//
// Kept in shared/ so both surfaces drive promotion from one mapping table.

import type { Block, BlockType, ChecklistItem, Slide, SlideDeckValue } from './types'

// promoteTargets lists the block types `type` can be promoted to, in order.
export function promoteTargets(type: BlockType): BlockType[] {
  switch (type) {
    case 'list':
      return ['checklist', 'slide_deck']
    case 'checklist':
      return ['slide_deck']
    default:
      return []
  }
}

type ItemText = { id?: string; text: string }

// extractItemTexts pulls the ordered item texts (with source ids) from a
// list or checklist block, or null if the block can't be promoted.
function extractItemTexts(block: Block): ItemText[] | null {
  const v = block.value
  if (!Array.isArray(v)) return null
  if (block.type === 'list' || block.type === 'checklist') {
    // Both ListItem and ChecklistItem carry {id, text}.
    return (v as ReadonlyArray<{ id?: string; text?: string }>)
      .map((it) => ({ id: it.id, text: (it.text ?? '').trim() }))
      .filter((it) => it.text !== '')
  }
  return null
}

function newId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID().slice(0, 8)}`
}

// promoteBlockValue returns the converted value for `block` as `target`, or
// null when the conversion isn't part of the lattice. Callers persist the
// returned value (and set block.type = target) via the usual save path.
export function promoteBlockValue(block: Block, target: BlockType): Block['value'] | null {
  const items = extractItemTexts(block)
  if (items === null) return null

  if (target === 'checklist') {
    // Reuse source ids — list ("li-") and checklist ("cli-") ids are both
    // just stable keys, so carrying them over preserves any per-item state.
    return items.map((it): ChecklistItem => ({ id: it.id ?? newId('cli'), text: it.text, done: false }))
  }

  if (target === 'slide_deck') {
    const slides: Slide[] = items.map((it): Slide => ({
      id: newId('sld'),
      contentTypeId: 'title',
      values: { title: it.text },
    }))
    const value: SlideDeckValue = { slides, currentIndex: 0 }
    return value
  }

  return null
}
