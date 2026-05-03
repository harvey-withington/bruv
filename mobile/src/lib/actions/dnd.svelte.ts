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
  /** The drop-target element the drag started from. Captured at arm
   *  time so it survives a mid-drag DOM unmount of the source (e.g.
   *  single-expand accordion collapsing the source category). May be
   *  detached from the document by commit time — the host should read
   *  attributes off it, not insert into it. Null only if arm couldn't
   *  find a containing drop target (shouldn't happen). */
  fromTarget: HTMLElement | null
  /** The drop-target element the drag ended over. Always live and in
   *  the document. */
  toTarget: HTMLElement
  /** True when a second pointer touched the screen at any point during
   *  the armed drag — the touch equivalent of holding Ctrl on desktop.
   *  Hosts use this to route to a Copy / Duplicate RPC instead of Move. */
  isCopy: boolean
}

type Options = {
  /** Called after a successful drop. The handler should call the right
   *  MoveCard* RPC and update local state. The action itself does NOT
   *  reorder Svelte state — only the DOM during the drag — so the
   *  handler is responsible for persisting and refreshing. */
  onMove: (detail: DragMoveDetail) => void | Promise<void>
  /** Called when the user holds a dragged item over a drop target
   *  marked `data-collapsed="true"` for HOVER_EXPAND_MS. The handler
   *  should expand whatever the target represents (e.g. an accordion
   *  category) so the drop can land. The drag continues; the next
   *  pointermove finds the now-revealed inner card list as the drop
   *  container automatically. */
  onHoverExpand?: (target: HTMLElement) => void
  /** CSS selector matching draggable rows. Defaults to the cards-page
   *  convention. Generalised so the same action can drive tree-level
   *  reorder lists (brands / streams / projects) — see BrowsePage. */
  rowSelector?: string
  /** CSS selector matching drop-target containers. Drag-overs walk up
   *  via closest(). Defaults to the cards-page convention. */
  dropTargetSelector?: string
  /** Attribute on rows that holds the item identifier emitted in
   *  DragMoveDetail.cardID. Defaults to `data-card-id`. For tree
   *  lists this is `data-brand-slug` etc. */
  rowIdAttribute?: string
  /** Optional selector for "expand on hover" targets that aren't
   *  drop targets themselves. When the finger holds over an element
   *  matching this selector AND carrying `data-collapsed="true"`,
   *  onHoverExpand fires after HOVER_EXPAND_MS. Use this for tree
   *  navigation: drag-hover a collapsed parent row to reveal its
   *  child list as a valid drop target. By default, hover-expand
   *  only fires on the action's own drop targets — this widens the
   *  set without affecting the drop-target hit-test. */
  expandOnHoverSelector?: string
}

const LONG_PRESS_MS = 250
// Tight cancel threshold so the action gives up arming BEFORE the
// browser's scroll-commit threshold (~5–10px on Android Chrome). Hosts
// that want scroll on draggable rows set the row's `touch-action: pan-y`
// instead of `none` — the browser then handles scroll from the moment
// the user moves, and our action cancels its press timer when the
// movement exceeds this threshold (so we don't end up "armed" while
// the page is already scrolling). 4px is enough to count as
// "intentional move" without being so tight that natural finger drift
// during a hold cancels the press.
const MOVE_CANCEL_PX = 4
const AUTOSCROLL_EDGE = 70 // px from viewport edge that triggers autoscroll
const AUTOSCROLL_MAX = 18  // px per frame at the very edge
const HOVER_EXPAND_MS = 500

export function dragSortable(node: HTMLElement, options: Options) {
  let opts = options

  function rowSel(): string {
    return opts.rowSelector ?? '[data-card-id]'
  }
  function targetSel(): string {
    return opts.dropTargetSelector ?? '[data-drop-target="category"]'
  }
  function idAttr(): string {
    return opts.rowIdAttribute ?? 'data-card-id'
  }

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
  // Source category IDs captured at arm time. Stored separately from
  // originalParent because in single-expand accordion mode the source
  // can collapse mid-drag (auto-expanding the destination collapses
  // the source), unmounting originalParent and detaching it from the
  // DOM. closest() on a detached element returns null, so we'd lose
  // the IDs by commit time. Capturing eagerly is the simpler fix.
  let originalCategoryID = ''
  let originalProjectID = ''
  // Source drop-target captured at arm time for the same reason —
  // hosts that need additional context (data-brand-slug etc. for
  // tree-level cross-parent moves) read attributes off this element.
  let originalTarget: HTMLElement | null = null
  // Copy mode: ratcheted once a second pointer joins during an armed
  // drag, the touch-equivalent of holding Ctrl on desktop. Stays true
  // for the duration of the drag — releasing the second finger doesn't
  // un-engage copy.
  let isCopyMode = false
  let activeDropTarget: HTMLElement | null = null
  let autoScrollHandle: number | null = null
  // Hover-expand: when a drag stalls over a collapsed drop target,
  // fire opts.onHoverExpand so the host can reveal the contents.
  let hoverExpandTarget: HTMLElement | null = null
  let hoverExpandTimer: ReturnType<typeof setTimeout> | null = null
  // Saved values so teardown restores the page's scroll behaviour
  // exactly as it was. We freeze body scroll for the duration of a
  // drag — without this, Android Chrome commits to a vertical scroll
  // gesture before setPointerCapture / preventDefault take effect,
  // and the drag visually "pops" but the page scrolls underneath
  // until release.
  let prevBodyTouchAction = ''
  let prevBodyOverflow = ''

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
    return el.closest(targetSel()) as HTMLElement | null
  }

  function findRowUnder(el: HTMLElement | null): HTMLElement | null {
    if (!el) return null
    return el.closest(rowSel()) as HTMLElement | null
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

  /** Find the container we should insert dragRow into. When the
   *  drop target is the outer wrapper of an accordion (a `<section>`),
   *  the actual card-list `<ul>` lives inside and is the right place
   *  to put the row. data-card-list marks it explicitly; absent that,
   *  we fall back to the drop target itself (the original layout). */
  function dropContainer(target: HTMLElement): HTMLElement {
    const inner = target.querySelector('[data-card-list]') as HTMLElement | null
    return inner ?? target
  }

  function reorderInDOM(beforeRow: HTMLElement | null, dropTarget: HTMLElement) {
    if (!dragRow) return
    const container = dropContainer(dropTarget)
    if (!beforeRow) {
      // Append to end of container.
      container.appendChild(dragRow)
      return
    }
    if (beforeRow === dragRow) return
    container.insertBefore(dragRow, beforeRow)
  }

  function clearHoverExpand() {
    if (hoverExpandTimer) {
      clearTimeout(hoverExpandTimer)
      hoverExpandTimer = null
    }
    hoverExpandTarget = null
  }

  /** Find the element (if any) the host wants expanded after a hover-
   *  hold. Two paths:
   *
   *    1. The current drop target itself, if it's marked collapsed.
   *       This is the project-page accordion's "drop on a collapsed
   *       category to open it" pattern.
   *    2. Any element matching `expandOnHoverSelector` (BrowsePage tree
   *       wires brand / stream rows here) that's marked collapsed.
   *       This makes parent rows in a tree act as expand-targets so
   *       the user can drag through nested levels.
   *
   *  Drop-target match takes priority over expand-on-hover match — if
   *  the user is hovering directly on a collapsed drop container, that
   *  container is what we want to expand, not its parent. */
  function findExpandTarget(under: HTMLElement | null, dropTarget: HTMLElement | null): HTMLElement | null {
    if (dropTarget && dropTarget.getAttribute('data-collapsed') === 'true') {
      return dropTarget
    }
    const sel = opts.expandOnHoverSelector
    if (!sel || !under) return null
    const candidate = under.closest(sel) as HTMLElement | null
    if (candidate && candidate.getAttribute('data-collapsed') === 'true') {
      return candidate
    }
    return null
  }

  function maybeStartHoverExpand(target: HTMLElement | null) {
    if (!target) {
      clearHoverExpand()
      return
    }
    if (hoverExpandTarget === target) return // already armed for this target
    clearHoverExpand()
    hoverExpandTarget = target
    hoverExpandTimer = setTimeout(() => {
      const t = hoverExpandTarget
      hoverExpandTarget = null
      hoverExpandTimer = null
      if (t) opts.onHoverExpand?.(t)
    }, HOVER_EXPAND_MS)
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
    clearHoverExpand()
    if (ghost) {
      ghost.remove()
      ghost = null
    }
    if (dragRow) {
      dragRow.classList.remove('dnd-source')
      dragRow = null
    }
    highlightTarget(null)
    if (armed) {
      // Restore the page's scroll behaviour. Only undo our overrides
      // if we actually armed — short taps that never armed never
      // touched these styles.
      document.body.style.touchAction = prevBodyTouchAction
      document.body.style.overflow = prevBodyOverflow
      node.style.touchAction = ''
    }
    armed = false
    pointerID = -1
    originalParent = null
    originalNext = null
    originalCategoryID = ''
    originalProjectID = ''
    originalTarget = null
    isCopyMode = false
  }

  function arm(row: HTMLElement, ev: PointerEvent) {
    armed = true
    dragRow = row
    originalParent = row.parentElement
    originalNext = row.nextElementSibling
    // Capture source attributes BEFORE the drag mutates the DOM (or
    // before single-expand mode collapses the source category and
    // unmounts its <ul>). Reading these at commit time would race.
    const sourceTarget = row.closest(targetSel()) as HTMLElement | null
    originalTarget = sourceTarget
    originalCategoryID = sourceTarget?.getAttribute('data-category-id') ?? ''
    originalProjectID = sourceTarget?.getAttribute('data-project-id') ?? ''
    isCopyMode = false
    row.classList.add('dnd-source')
    // Lock the page's touch-action + overflow so the browser stops
    // interpreting subsequent pointer movement as scroll. setPointer-
    // Capture alone redirects pointer EVENTS to our element, but
    // doesn't stop the browser's parallel scroll handler — it has
    // already committed by the time our pointermove fires preventDefault.
    // Locking body is the surest cross-browser way to guarantee our
    // drag wins.
    prevBodyTouchAction = document.body.style.touchAction
    prevBodyOverflow = document.body.style.overflow
    document.body.style.touchAction = 'none'
    document.body.style.overflow = 'hidden'
    // The action's node also needs touch-action: none so any pointer
    // event delivered to it (after setPointerCapture) doesn't get
    // re-interpreted as a scroll attempt.
    node.style.touchAction = 'none'
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
    // Second-pointer-during-armed-drag = engage copy mode. Once
    // engaged, ratchets — releasing the second finger doesn't revert.
    // Visual: ghost gets the dnd-ghost-copy class, hosts get isCopy
    // in the emitted detail.
    if (armed && ev.pointerId !== pointerID) {
      if (!isCopyMode) {
        isCopyMode = true
        ghost?.classList.add('dnd-ghost-copy')
        try {
          navigator.vibrate?.(10)
        } catch {
          /* not all browsers support vibrate */
        }
      }
      ev.stopPropagation()
      return
    }
    // Ignore non-primary buttons & non-pointer-down phases.
    if (ev.button !== 0 && ev.pointerType === 'mouse') return
    const row = findRowUnder(ev.target as HTMLElement)
    if (!row) return
    // Make sure the row belongs to THIS action's container — when
    // multiple dragSortable instances are nested (e.g. brand-list
    // contains stream-lists contains project-lists in BrowsePage),
    // event bubbling reaches every ancestor's listener and each one
    // would otherwise try to arm independently. node.contains catches
    // the deepest action that owns this row.
    if (!node.contains(row)) return
    // Stop propagation so outer actions in a nested layout don't
    // also see this pointerdown and start their own press timer.
    ev.stopPropagation()
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
    maybeStartHoverExpand(findExpandTarget(under, target))

    if (!target) return

    // Collapsed drop targets defer to onHoverExpand — moving rows
    // into a hidden container produces a confusing visual state.
    // Once the host expands the target (data-collapsed flips off
    // and a [data-card-list] appears), the next pointermove drops
    // through to the normal reorder path.
    if (target.getAttribute('data-collapsed') === 'true') return

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

    const cardID = dragRow.getAttribute(idAttr()) ?? ''
    const toCategoryID = target.getAttribute('data-category-id') ?? ''
    const toProjectID = target.getAttribute('data-project-id') ?? ''

    // Source IDs were captured at arm time to survive a possible
    // mid-drag DOM unmount of the source <section>. Fall back to
    // destination IDs only if arm() couldn't find them (no closest
    // drop target — shouldn't happen in practice).
    const fromCategoryID = originalCategoryID || toCategoryID
    const fromProjectID = originalProjectID || toProjectID

    // Position is the index of dragRow among its siblings matching
    // the row selector — read BEFORE we restore dragRow to source,
    // otherwise the index reflects the source list.
    const siblings = Array.from(target.querySelectorAll(rowSel())) as HTMLElement[]
    const toPosition = siblings.indexOf(dragRow)

    // Restore dragRow to its original position before letting Svelte
    // see the state change. The action moves dragRow during the drag
    // for visual feedback, but Svelte's keyed {#each} reconciler is
    // blind to manual DOM mutation — leaving dragRow in the destination
    // while Svelte also CREATES a new <li> for the moved card produces
    // a duplicate that lingers until refresh. Snapping back gives
    // Svelte a clean baseline (source has dragRow, destination doesn't)
    // and the upcoming state update remaps cleanly.
    if (originalParent && originalParent.isConnected) {
      if (originalNext && originalNext.parentNode === originalParent) {
        originalParent.insertBefore(dragRow, originalNext)
      } else {
        originalParent.appendChild(dragRow)
      }
    } else {
      // Source unmounted mid-drag (e.g. single-expand collapsed it).
      // Detach dragRow rather than leaving it in the destination —
      // Svelte will re-create it cleanly when its state update for
      // the destination renders.
      dragRow.remove()
    }

    void ev
    void opts.onMove({
      cardID,
      fromProjectID,
      fromCategoryID,
      toProjectID,
      toCategoryID,
      toPosition,
      fromTarget: originalTarget,
      toTarget: target,
      isCopy: isCopyMode,
    })
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

  // Document-level non-passive touchmove. Pointer events use a separate
  // event family from touch events; touch-action only governs touch
  // events (and the gesture-recognition path the browser feeds them
  // into). When `touch-action: pan-y` is set on a row, the browser
  // commits to a vertical scroll on the first touchmove with material
  // movement — and once committed, our pointer events stop firing.
  //
  // The fix the major DnD libraries (svelte-dnd-action, react-beautiful-
  // dnd, dnd-kit) all converge on: register a non-passive touchmove
  // listener that calls preventDefault while a drag is armed. The
  // browser checks preventDefault on the FIRST touchmove BEFORE
  // committing to scroll, so getting in there early — synchronously,
  // from a non-passive listener — wins. Any later than that and
  // commitment is irreversible.
  //
  // Why on document and not on node: setPointerCapture redirects
  // pointer events to the captured element, but TOUCH events still
  // dispatch to the originally-touched element and bubble through the
  // DOM. We want the preventDefault to fire regardless of where the
  // finger is now, so the listener has to be on a high-enough ancestor
  // to catch every touch. document is the safest place.
  const blockTouchScroll = (e: TouchEvent) => {
    if (armed && e.cancelable) e.preventDefault()
  }

  // Pointer events on the action's element bubble up from rows,
  // so a single set of listeners is enough.
  node.addEventListener('pointerdown', onPointerDown)
  node.addEventListener('pointermove', onPointerMove)
  node.addEventListener('pointerup', onPointerUp)
  node.addEventListener('pointercancel', onPointerCancel)
  document.addEventListener('touchmove', blockTouchScroll, { passive: false })

  return {
    update(next: Options) {
      opts = next
    },
    destroy() {
      node.removeEventListener('pointerdown', onPointerDown)
      node.removeEventListener('pointermove', onPointerMove)
      node.removeEventListener('pointerup', onPointerUp)
      node.removeEventListener('pointercancel', onPointerCancel)
      document.removeEventListener('touchmove', blockTouchScroll)
      teardown()
    },
  }
}
