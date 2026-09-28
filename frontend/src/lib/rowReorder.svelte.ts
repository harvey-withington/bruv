// Grip-driven drag-to-reorder for a vertical list of id-keyed rows —
// the HTML5 drag handlers OptionsEditorDialog, EditableChecklist and
// friends each wrote by hand, once. The owner renders a drop indicator
// where `dropBeforeId` points and wires the handlers:
//
//   const drag = new RowReorder(() => rules, next => { rules = next })
//   <div ondrop={drag.drop} ondragover={drag.containerOver}>
//     {#each rules as r, i (r.id)}
//       {#if drag.showsIndicatorBefore(r.id)}<div class="drop-line"></div>{/if}
//       <div ondragover={(e) => drag.over(e, r.id, i)}>
//         <span draggable="true" ondragstart={(e) => drag.start(e, r.id)} ondragend={drag.end}>…grip…</span>
//     {/each}
//     {#if drag.showsIndicatorAtEnd()}<div class="drop-line"></div>{/if}
//   </div>

import { computeReorder, wouldReorder, DROP_END, type DropTarget, type Identified } from './reorder'

export class RowReorder<T extends Identified> {
  draggingId = $state<string | null>(null)
  dropBeforeId = $state<DropTarget | null>(null)

  constructor(
    private readonly items: () => T[],
    private readonly commit: (next: T[]) => void,
  ) {}

  start = (e: DragEvent, id: string) => {
    this.draggingId = id
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', id)
    }
  }

  /** Row dragover: the top half targets this row, the bottom half the next. */
  over = (e: DragEvent, overId: string, index: number) => {
    if (this.draggingId === null) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const list = this.items()
    let candidate: DropTarget
    if (e.clientY < rect.top + rect.height / 2) {
      candidate = overId
    } else {
      candidate = list[index + 1]?.id ?? DROP_END
    }
    this.dropBeforeId = wouldReorder(list, this.draggingId, candidate, 'move') ? candidate : null
  }

  /** Container dragover: keeps the drop legal in the gaps between rows. */
  containerOver = (e: DragEvent) => {
    if (this.draggingId !== null) e.preventDefault()
  }

  end = () => {
    this.draggingId = null
    this.dropBeforeId = null
  }

  drop = (e: DragEvent) => {
    e.preventDefault()
    const dragged = this.draggingId
    const target = this.dropBeforeId
    this.end()
    if (dragged === null || target === null) return
    const list = this.items()
    const next = computeReorder(list, dragged, target, { mode: 'move' })
    if (next !== list) this.commit(next)
  }

  showsIndicatorBefore(id: string): boolean {
    return this.draggingId !== null && this.dropBeforeId === id
  }

  showsIndicatorAtEnd(): boolean {
    return this.draggingId !== null && this.dropBeforeId === DROP_END
  }
}
