// Reorder helpers for drag-and-drop lists keyed by stable ids rather
// than array indices (CLAUDE.md: "Never key mutable state by array
// index"). Exported as pure functions so the drop math can be unit-
// tested without mounting any Svelte component.

export type Identified = { id: string }

// Sentinel that dropBeforeId can take to mean "append to the end of
// the list" — useful when the cursor drops below the last item.
export const DROP_END = '__end__'
export type DropTarget = string | typeof DROP_END

type ReorderOptions = {
  mode: 'move' | 'copy'
  // Used in 'copy' mode to generate an id for the duplicate. Default
  // returns the original id with a "-copy" suffix; callers that need
  // uuid-style ids (e.g. CardDetail's block duplication) pass their
  // own generator.
  newId?: () => string
}

// Ask whether a proposed drop would change the list, given the
// current state of a drag. Used by the UI to suppress drop-indicator
// rendering for no-op targets — otherwise the user sees an insertion
// line that does nothing when released, which reads as a bug.
//
// Copy drops are always effective (even onto self — that inserts a
// duplicate). Move drops are no-ops when dropping onto self or into
// the slot immediately after self, because both resolve to the
// dragged block's current position.
export function wouldReorder<T extends Identified>(
  items: T[],
  draggedId: string,
  dropBeforeId: DropTarget,
  mode: 'move' | 'copy',
): boolean {
  if (mode === 'copy') return true
  const fromIdx = items.findIndex(x => x.id === draggedId)
  const toIdx = dropBeforeId === DROP_END
    ? items.length
    : items.findIndex(x => x.id === dropBeforeId)
  if (fromIdx < 0 || toIdx < 0) return false
  return fromIdx !== toIdx && fromIdx !== toIdx - 1
}

// Compute the new list order after a drag-drop. Returns the original
// list reference (===) when the drop would be a no-op — caller can
// skip a re-render by identity check.
export function computeReorder<T extends Identified>(
  items: T[],
  draggedId: string,
  dropBeforeId: DropTarget,
  opts: ReorderOptions,
): T[] {
  const fromIdx = items.findIndex(x => x.id === draggedId)
  const toIdx = dropBeforeId === DROP_END
    ? items.length
    : items.findIndex(x => x.id === dropBeforeId)
  if (fromIdx < 0 || toIdx < 0) return items

  if (opts.mode === 'copy') {
    const next = [...items]
    const original = next[fromIdx]
    const dupId = opts.newId ? opts.newId() : `${original.id}-copy`
    next.splice(toIdx, 0, { ...original, id: dupId })
    return next
  }

  // Move mode: no-op if dropping onto self or directly below self
  if (fromIdx === toIdx || fromIdx === toIdx - 1) return items

  const next = [...items]
  const [item] = next.splice(fromIdx, 1)
  const adjustedTo = toIdx > fromIdx ? toIdx - 1 : toIdx
  next.splice(adjustedTo, 0, item)
  return next
}
