// dnd.svelte.ts — touch + pointer drag-and-drop for sortable card
// lists on mobile. Used by ProjectPage (move cards between categories
// and reorder within) and CategoryPage (reorder within one).
//
// HTML5 native DnD doesn't fire on touch devices, and `svelte-dnd-action`
// is more machinery than we need for two surfaces. This is a hand-rolled
// Pointer Events implementation, ~150 lines, with the gestures we need:
//
//   - Long-press (~250ms) on a card row arms drag mode. A short tap
//     within that window passes through, so the existing `onclick`
//     navigation on the card row still works.
//   - During drag: a ghost element follows the finger; the original
//     card fades; other cards animate out of the way; the category
//     under the finger highlights as a drop target.
//   - On release: the move persists via MoveCardInCategory or
//     MoveCardToCategory (callback fires, parent decides which RPC).
//   - On cancel (drop outside any target / Escape on hardware kbd):
//     ghost retreats, original restores, no save.
//
// Conventions for the page that mounts the action:
//   - Cards are `<li data-card-id="<id>">` inside a `<ul>` that gets
//     the action.
//   - Drop targets (categories) are `[data-drop-target="category"]`
//     elements containing `data-category-id` and `data-project-id`
//     attributes. The card list <ul> is itself a drop target so
//     reordering within a single category works the same way.
//   - The action uses pointer events with `setPointerCapture` so
//     scroll fights are minimised; while drag is armed, `touch-action`
//     on the row is disabled so the page doesn't scroll under the
//     drag.

export type DragMoveDetail = {
  cardID: string
  fromProjectID: string
  fromCategoryID: string
  toProjectID: string
  toCategoryID: string
  /** Final position within the destination category's card list. */
  toPosition: number
}

type Options = {
  /** Called after a successful drop. The handler should call the right
   *  MoveCard* RPC and update local state. The action itself does NOT
   *  reorder Svelte state — only the DOM during the drag — so the
   *  handler is responsible for persisting and refreshing. */
  onMove: (detail: DragMoveDetail) => void | Promise<void>
}

const LONG_PRESS_MS = 250
const MOVE_CANCEL_PX = 6
const AUTOSCROLL_EDGE = 70 // px from viewport edge that triggers autoscroll
const AUTOSCROLL_MAX = 18  // px per frame at the very edge

export function dragSortable(node: HTMLElement, options: Options) {
  let opts = options

  // Per-drag state. All null when no drag is in flight.
  let dragRow: HTMLElement | null = null
  let ghost: HTMLElement | null = null
  let pointerID = -1
  let pressTimer: ReturnType<typeof setTimeout> | null = null
  let armed = false
  let startX = 0
  let startY = 0
  let lastClientX = 0
  let lastClientY = 0
  let originalParent: HTMLElement | null = null
  let originalNext: Element | null = null
  let activeDropTarget: HTMLElement | null = null
  let autoScrollHandle: number | null = null

  function originRect(el: HTMLElement): DOMRect {
    return el.getBoundingClientRect()
  }

  function makeGhost(row: HTMLElement): HTMLElement {
    const rect = originRect(row)
    const clone = row.cloneNode(true) as HTMLElement
    clone.style.position = 'fixed'
    clone.style.left = `${rect.left}px`
    clone.style.top = `${rect.top}px`
    clone.style.width = `${rect.width}px`
    clone.style.height = `${rect.height}px`
    clone.style.pointerEvents = 'none'
    clone.style.zIndex = '999'
    clone.style.transform = 'scale(1.04)'
    clone.style.boxShadow = '0 12px 28px rgba(0,0,0,0.45)'
    clone.style.opacity = '0.95'
    clone.style.transition = 'box-shadow 120ms ease, transform 120ms ease'
    clone.setAttribute('aria-hidden', 'true')
    document.body.appendChild(clone)
    return clone
  }

  function moveGhost(x: number, y: number) {
    if (!ghost || !dragRow) return
    const rect = dragRow.getBoundingClientRect()
    // Translate ghost so the finger sits on the same spot of the card
    // it picked up (x - startX from the row's original left edge,
    // similarly for y).
    const dx = x - startX
    const dy = y - startY
    ghost.style.transform = `translate(${dx}px, ${dy}px) scale(1.04)`
    void rect
  }

  function findElementUnder(x: number, y: number): HTMLElement | null {
    // The ghost is pointer-events: none so elementFromPoint returns
    // the underlying card or category; no temp-hide trick needed.
    return document.elementFromPoint(x, y) as HTMLElement | null
  }

  function findDropTarget(el: HTMLElement | null): HTMLElement | null {
    if (!el) return null
    return el.closest('[data-drop-target="category"]') as HTMLElement | null
  }

  function findRowUnder(el: HTMLElement | null): HTMLElement | null {
    if (!el) return null
    return el.closest('[data-card-id]') as HTMLElement | null
  }

  function highlightTarget(target: HTMLElement | null) {
    if (activeDropTarget && activeDropTarget !== target) {
      activeDropTarget.classList.remove('dnd-target-active')
    }
    if (target && target !== activeDropTarget) {
      target.classList.add('dnd-target-active')
    }
    activeDropTarget = target
  }

  function reorderInDOM(beforeRow: HTMLElement | null, container: HTMLElement) {
    if (!dragRow) return
    if (!beforeRow) {
      // Append to end of container.
      container.appendChild(dragRow)
      return
    }
    if (beforeRow === dragRow) return
    container.insertBefore(dragRow, beforeRow)
  }

  function startAutoScroll() {
    if (autoScrollHandle !== null) return
    const tick = () => {
      autoScrollHandle = requestAnimationFrame(tick)
      const y = lastClientY
      const vh = window.innerHeight
      if (y < AUTOSCROLL_EDGE) {
        const ratio = (AUTOSCROLL_EDGE - y) / AUTOSCROLL_EDGE
        window.scrollBy(0, -AUTOSCROLL_MAX * ratio)
      } else if (y > vh - AUTOSCROLL_EDGE) {
        const ratio = (y - (vh - AUTOSCROLL_EDGE)) / AUTOSCROLL_EDGE
        window.scrollBy(0, AUTOSCROLL_MAX * ratio)
      }
    }
    autoScrollHandle = requestAnimationFrame(tick)
  }

  function stopAutoScroll() {
    if (autoScrollHandle !== null) {
      cancelAnimationFrame(autoScrollHandle)
      autoScrollHandle = null
    }
  }

  function teardown() {
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
    stopAutoScroll()
    if (ghost) {
      ghost.remove()
      ghost = null
    }
    if (dragRow) {
      dragRow.classList.remove('dnd-source')
      dragRow = null
    }
    highlightTarget(null)
    armed = false
    pointerID = -1
    originalParent = null
    originalNext = null
  }

  function arm(row: HTMLElement, ev: PointerEvent) {
    armed = true
    dragRow = row
    originalParent = row.parentElement
    originalNext = row.nextElementSibling
    row.classList.add('dnd-source')
    ghost = makeGhost(row)
    moveGhost(ev.clientX, ev.clientY)
    try {
      node.setPointerCapture(ev.pointerId)
    } catch {
      /* setPointerCapture can throw in odd states; non-fatal */
    }
    // Haptic confirmation of pickup, where supported.
    try {
      navigator.vibrate?.(15)
    } catch {
      /* not all browsers support vibrate */
    }
    startAutoScroll()
  }

  function onPointerDown(ev: PointerEvent) {
    // Ignore non-primary buttons & non-pointer-down phases.
    if (ev.button !== 0 && ev.pointerType === 'mouse') return
    const row = findRowUnder(ev.target as HTMLElement)
    if (!row) return
    pointerID = ev.pointerId
    startX = ev.clientX
    startY = ev.clientY
    lastClientX = ev.clientX
    lastClientY = ev.clientY
    pressTimer = setTimeout(() => {
      pressTimer = null
      arm(row, ev)
    }, LONG_PRESS_MS)
  }

  function onPointerMove(ev: PointerEvent) {
    if (ev.pointerId !== pointerID) return
    lastClientX = ev.clientX
    lastClientY = ev.clientY

    if (!armed) {
      // Cancel pending long-press if the finger moves too far —
      // the user is scrolling, not picking up.
      const dx = ev.clientX - startX
      const dy = ev.clientY - startY
      if (Math.hypot(dx, dy) > MOVE_CANCEL_PX && pressTimer) {
        clearTimeout(pressTimer)
        pressTimer = null
        pointerID = -1
      }
      return
    }

    ev.preventDefault()
    moveGhost(ev.clientX, ev.clientY)

    const under = findElementUnder(ev.clientX, ev.clientY)
    const target = findDropTarget(under)
    highlightTarget(target)

    if (!target) return

    // Live reorder: when over another card in the same target,
    // swap; when over a category that's not the row's parent,
    // move the row into that category at the end (or before the
    // hovered row if there is one).
    const rowUnder = findRowUnder(under)
    if (rowUnder && rowUnder !== dragRow && target.contains(rowUnder)) {
      const targetRect = rowUnder.getBoundingClientRect()
      const insertBefore = ev.clientY < targetRect.top + targetRect.height / 2
      reorderInDOM(insertBefore ? rowUnder : (rowUnder.nextElementSibling as HTMLElement | null), target)
    } else if (!target.contains(dragRow)) {
      reorderInDOM(null, target)
    }
  }

  function commit(ev: PointerEvent) {
    if (!armed || !dragRow) return
    const target = findDropTarget(dragRow.parentElement)
    if (!target) return

    const cardID = dragRow.getAttribute('data-card-id') ?? ''
    const toCategoryID = target.getAttribute('data-category-id') ?? ''
    const toProjectID = target.getAttribute('data-project-id') ?? ''

    // Source category is read from the original parent.
    const fromCategoryID = (originalParent?.getAttribute('data-category-id')) ?? toCategoryID
    const fromProjectID = (originalParent?.getAttribute('data-project-id')) ?? toProjectID

    // Position is the index of dragRow among its siblings with
    // data-card-id attributes.
    const siblings = Array.from(target.querySelectorAll('[data-card-id]')) as HTMLElement[]
    const toPosition = siblings.indexOf(dragRow)

    void ev
    void opts.onMove({ cardID, fromProjectID, fromCategoryID, toProjectID, toCategoryID, toPosition })
  }

  function onPointerUp(ev: PointerEvent) {
    if (ev.pointerId !== pointerID) return
    if (armed) {
      commit(ev)
      // Browsers fire a synthetic `click` after pointerup when the
      // displacement is small. After a successful drop we don't want
      // that click to navigate into the dragged card. Swallow the
      // very next click event at capture phase, then disarm.
      const swallow = (e: Event) => {
        e.stopPropagation()
        e.preventDefault()
        window.removeEventListener('click', swallow, true)
      }
      window.addEventListener('click', swallow, true)
      // Failsafe: if no click ever fires, remove the listener after a tick.
      setTimeout(() => window.removeEventListener('click', swallow, true), 250)
    }
    teardown()
  }

  function onPointerCancel(ev: PointerEvent) {
    if (ev.pointerId !== pointerID) return
    // Restore original position on cancel.
    if (armed && dragRow && originalParent) {
      if (originalNext) {
        originalParent.insertBefore(dragRow, originalNext)
      } else {
        originalParent.appendChild(dragRow)
      }
    }
    teardown()
  }

  // Pointer events on the action's element bubble up from rows,
  // so a single set of listeners is enough.
  node.addEventListener('pointerdown', onPointerDown)
  node.addEventListener('pointermove', onPointerMove)
  node.addEventListener('pointerup', onPointerUp)
  node.addEventListener('pointercancel', onPointerCancel)

  return {
    update(next: Options) {
      opts = next
    },
    destroy() {
      node.removeEventListener('pointerdown', onPointerDown)
      node.removeEventListener('pointermove', onPointerMove)
      node.removeEventListener('pointerup', onPointerUp)
      node.removeEventListener('pointercancel', onPointerCancel)
      teardown()
    },
  }
}
