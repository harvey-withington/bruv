// Three-way card merge for offline sync.
//
// base is the card as downloaded, mine is the offline working copy,
// theirs is the server's card now. Each part merges at the smallest unit
// that has a meaning of its own: scalar fields one by one, tags as a set,
// blocks by their stable ids (and checklist / list items by theirs),
// text with a line merge. Whatever both sides changed differently comes
// back as a conflict for the user. The merged card holds the server's
// value there until the conflict is resolved, so nothing is guessed.

import type { Block, Card, CardEdit } from '../types'
import { merge3 } from './diff3'

export type ConflictKind = 'title' | 'description' | 'due_date' | 'block'

export type OfflineConflict = {
  /** Stable within a card: "title", "description", "due_date", "block:<id>". */
  id: string
  kind: ConflictKind
  /** Block label, for block conflicts. */
  label?: string
  /** The offline value; null when this side deleted the block. */
  mine: unknown
  /** The server's value; null when the server deleted the block. */
  theirs: unknown
}

/** keep the offline value, keep the server's, or (text and blocks) keep both. */
export type ConflictChoice = 'mine' | 'theirs' | 'both'

export type CardMergeResult = { card: Card; conflicts: OfflineConflict[] }

export function mergeCard(base: Card, mine: Card, theirs: Card): CardMergeResult {
  const conflicts: OfflineConflict[] = []
  const card: Card = { ...theirs }

  const title = mergeScalar(base.title, mine.title, theirs.title)
  if (title.conflict) conflicts.push({ id: 'title', kind: 'title', mine: mine.title, theirs: theirs.title })
  else card.title = title.value

  const description = merge3(base.description ?? '', mine.description ?? '', theirs.description ?? '')
  if (description.clean) card.description = description.text
  else conflicts.push({ id: 'description', kind: 'description', mine: mine.description, theirs: theirs.description })

  const due = mergeScalar(base.due_date, mine.due_date, theirs.due_date, sameDueDate)
  if (due.conflict) conflicts.push({ id: 'due_date', kind: 'due_date', mine: mine.due_date, theirs: theirs.due_date })
  else card.due_date = due.value

  card.tags = mergeSet(base.tags ?? [], mine.tags ?? [], theirs.tags ?? [])

  const blocks = mergeById(base.blocks ?? [], mine.blocks ?? [], theirs.blocks ?? [], mergeBlock)
  card.blocks = blocks.items
  for (const c of blocks.conflicts) {
    const label = (c.mine ?? c.theirs)?.label
    conflicts.push({ id: `block:${c.id}`, kind: 'block', label, mine: c.mine, theirs: c.theirs })
  }
  return { card, conflicts }
}

/** The edit that turns `from` into `to` — only the parts that differ. */
export function cardEditBetween(from: Card, to: Card): CardEdit {
  const edit: CardEdit = {}
  if (from.title !== to.title) edit.title = to.title
  if ((from.description ?? '') !== (to.description ?? '')) edit.description = to.description ?? ''
  if (!sameDueDate(from.due_date, to.due_date)) edit.due_date = to.due_date ?? ''
  if (!same(from.tags ?? [], to.tags ?? [])) edit.tags = to.tags ?? []
  if (!same(from.blocks ?? [], to.blocks ?? [])) edit.blocks = to.blocks ?? []
  return edit
}

export function isEmptyEdit(edit: CardEdit): boolean {
  return Object.keys(edit).length === 0
}

// --- parts ---

type Scalar<T> = { conflict: false; value: T } | { conflict: true }

function mergeScalar<T>(base: T, mine: T, theirs: T, eq: (a: T, b: T) => boolean = same): Scalar<T> {
  if (eq(mine, base) || eq(mine, theirs)) return { conflict: false, value: theirs }
  if (eq(theirs, base)) return { conflict: false, value: mine }
  return { conflict: true }
}

/** Server order, plus what mine added, minus what mine removed. */
function mergeSet(base: string[], mine: string[], theirs: string[]): string[] {
  const added = mine.filter((t) => !base.includes(t))
  const removed = new Set(base.filter((t) => !mine.includes(t)))
  const out = theirs.filter((t) => !removed.has(t))
  for (const t of added) if (!out.includes(t)) out.push(t)
  return out
}

type Identified = { id: string }
type ItemMerge<T> = (base: T, mine: T, theirs: T) => { conflict: false; value: T } | { conflict: true }
type IdConflict<T> = { id: string; mine: T | null; theirs: T | null }

/** Merge two edited copies of an id-keyed list. */
export function mergeById<T extends Identified>(
  base: T[], mine: T[], theirs: T[], mergeItem: ItemMerge<T>,
): { items: T[]; conflicts: IdConflict<T>[] } {
  const byId = (list: T[]) => new Map(list.map((x) => [x.id, x]))
  const b = byId(base), m = byId(mine), t = byId(theirs)
  const conflicts: IdConflict<T>[] = []
  const keep = new Map<string, T>()

  for (const id of new Set([...b.keys(), ...m.keys(), ...t.keys()])) {
    const bi = b.get(id), mi = m.get(id), ti = t.get(id)
    if (bi && mi && ti) {
      const r = mergeItem(bi, mi, ti)
      if (r.conflict) { conflicts.push({ id, mine: mi, theirs: ti }); keep.set(id, ti) }
      else keep.set(id, r.value)
    } else if (!bi) {
      // Added on one side (or, unlikely, both with one id).
      if (mi && ti && !same(mi, ti)) { conflicts.push({ id, mine: mi, theirs: ti }); keep.set(id, ti) }
      else keep.set(id, (ti ?? mi)!)
    } else if (mi && !ti) {
      // The server deleted it: fine unless mine changed it.
      if (!same(mi, bi)) conflicts.push({ id, mine: mi, theirs: null })
    } else if (ti && !mi) {
      // Mine deleted it: fine unless the server changed it.
      if (same(ti, bi)) continue
      conflicts.push({ id, mine: null, theirs: ti })
      keep.set(id, ti)
    }
    // Deleted on both sides: gone.
  }

  return { items: mergeOrder(base, mine, theirs, keep), conflicts }
}

/** Mine's order if only mine reordered, else the server's; items new on
 *  the other side go in after the item they followed there. */
function mergeOrder<T extends Identified>(base: T[], mine: T[], theirs: T[], keep: Map<string, T>): T[] {
  const ids = (list: T[]) => list.map((x) => x.id)
  const common = (list: string[], other: Set<string>) => list.filter((id) => other.has(id))
  const baseIds = ids(base), mineIds = ids(mine), theirIds = ids(theirs)
  const inAll = new Set(baseIds.filter((id) => mineIds.includes(id) && theirIds.includes(id)))
  const theirsReordered = !same(common(baseIds, inAll), common(theirIds, inAll))
  const [primary, secondary] = theirsReordered ? [theirIds, mineIds] : [mineIds, theirIds]

  const order = primary.filter((id) => keep.has(id))
  secondary.forEach((id, i) => {
    if (!keep.has(id) || order.includes(id)) return
    let at = 0
    for (let p = i - 1; p >= 0; p--) {
      const idx = order.indexOf(secondary[p])
      if (idx !== -1) { at = idx + 1; break }
    }
    order.splice(at, 0, id)
  })
  // Anything kept but in neither list (kept server copies of a block mine
  // deleted are in theirs, so this is only belt and braces).
  for (const id of keep.keys()) if (!order.includes(id)) order.push(id)
  return order.map((id) => keep.get(id)!)
}

function mergeBlock(base: Block, mine: Block, theirs: Block): { conflict: false; value: Block } | { conflict: true } {
  if (same(mine, base) || same(mine, theirs)) return { conflict: false, value: theirs }
  if (same(theirs, base)) return { conflict: false, value: mine }
  if (mine.type !== theirs.type) return { conflict: true }

  const label = mergeScalar(base.label, mine.label, theirs.label)
  const meta = mergeScalar(base.meta, mine.meta, theirs.meta)
  if (label.conflict || meta.conflict) return { conflict: true }
  const value = mergeBlockValue(base, mine, theirs)
  if (value.conflict) return { conflict: true }
  return { conflict: false, value: { ...theirs, label: label.value, meta: meta.value, value: value.value } }
}

function mergeBlockValue(base: Block, mine: Block, theirs: Block): Scalar<Block['value']> {
  const plain = mergeScalar(base.value, mine.value, theirs.value)
  if (!plain.conflict) return plain
  const [bv, mv, tv]: unknown[] = [base.value, mine.value, theirs.value]
  if (typeof bv === 'string' && typeof mv === 'string' && typeof tv === 'string' && mine.type === 'text') {
    const text = merge3(bv, mv, tv)
    return text.clean ? { conflict: false, value: text.text } : { conflict: true }
  }
  if (isItemList(bv) && isItemList(mv) && isItemList(tv)) {
    const items = mergeById(bv, mv, tv, (b, m, t) => {
      const r = mergeScalar<Identified>(b, m, t)
      return r.conflict ? r : { conflict: false, value: r.value }
    })
    if (items.conflicts.length) return { conflict: true }
    return { conflict: false, value: items.items as Block['value'] }
  }
  return { conflict: true }
}

function isItemList(v: unknown): v is Identified[] {
  return Array.isArray(v) && v.every((x) => !!x && typeof x === 'object' && typeof (x as Identified).id === 'string')
}

// --- equality ---

/** Deep equality that ignores object key order (the server writes sorted
 *  keys, the client doesn't). Missing and undefined keys are equal. */
export function same(a: unknown, b: unknown): boolean {
  return canonical(a) === canonical(b)
}

function canonical(v: unknown): string {
  return JSON.stringify(v, (_k, val: unknown) => {
    if (val && typeof val === 'object' && !Array.isArray(val)) {
      const obj = val as Record<string, unknown>
      return Object.fromEntries(Object.keys(obj).sort().filter((k) => obj[k] !== undefined).map((k) => [k, obj[k]]))
    }
    return val
  }) ?? 'undefined'
}

/** "2026-11-01" and "2026-11-01T00:00:00Z" are the same due date. */
export function sameDueDate(a: string | null | undefined, b: string | null | undefined): boolean {
  if (!a || !b) return !a && !b
  if (a === b) return true
  const ta = Date.parse(a), tb = Date.parse(b)
  return !Number.isNaN(ta) && ta === tb
}

// --- resolving ---

/** Separates the two halves of a "keep both" text resolution. */
const BOTH_SEPARATOR = '\n\n---\n\n'

/** Whether "keep both" means anything for this conflict. */
export function canKeepBoth(c: OfflineConflict): boolean {
  if (c.kind === 'description') return true
  return c.kind === 'block' && c.mine !== null && c.theirs !== null
}

/**
 * The merged card with the user's choices applied. `merged` holds the
 * server's value at every conflict (mergeCard's contract), so "theirs"
 * changes nothing; `mine` is the offline working copy, used to place a
 * block that only exists there.
 */
export function applyResolutions(merged: Card, mine: Card, conflicts: OfflineConflict[], choices: Record<string, ConflictChoice>): Card {
  const card: Card = { ...merged, blocks: [...merged.blocks] }
  for (const c of conflicts) {
    const choice = choices[c.id] ?? 'theirs'
    if (choice === 'theirs') continue
    switch (c.kind) {
      case 'title':
        card.title = String(c.mine ?? '')
        break
      case 'due_date':
        card.due_date = (c.mine as string | null) ?? null
        break
      case 'description':
        card.description = choice === 'both'
          ? joinBoth(String(c.theirs ?? ''), String(c.mine ?? ''))
          : String(c.mine ?? '')
        break
      case 'block':
        card.blocks = resolveBlock(card.blocks, mine.blocks, c, choice)
        break
    }
  }
  return card
}

function joinBoth(theirs: string, mine: string): string {
  if (!theirs.trim()) return mine
  if (!mine.trim()) return theirs
  return theirs + BOTH_SEPARATOR + mine
}

function resolveBlock(blocks: Block[], mineBlocks: Block[], c: OfflineConflict, choice: ConflictChoice): Block[] {
  const id = c.id.slice('block:'.length)
  const mine = c.mine as Block | null
  const theirs = c.theirs as Block | null
  const at = blocks.findIndex((b) => b.id === id)
  if (choice === 'mine') {
    if (!mine) return blocks.filter((b) => b.id !== id)
    if (at !== -1) return blocks.map((b) => (b.id === id ? mine : b))
    return insertAfterPredecessor(blocks, mineBlocks, mine)
  }
  // Keep both: one block holding both texts, or the offline copy beside the server's.
  if (!mine || !theirs) return blocks
  if (mine.type === 'text' && theirs.type === 'text') {
    return blocks.map((b) => (b.id === id ? { ...theirs, value: joinBoth(String(theirs.value ?? ''), String(mine.value ?? '')) } : b))
  }
  const copy: Block = { ...mine, id: `${mine.id}-offline` }
  const out = [...blocks]
  out.splice(at === -1 ? out.length : at + 1, 0, copy)
  return out
}

function insertAfterPredecessor(blocks: Block[], order: Block[], block: Block): Block[] {
  const out = [...blocks]
  const i = order.findIndex((b) => b.id === block.id)
  for (let p = i - 1; p >= 0; p--) {
    const idx = out.findIndex((b) => b.id === order[p].id)
    if (idx !== -1) { out.splice(idx + 1, 0, block); return out }
  }
  out.unshift(block)
  return out
}
