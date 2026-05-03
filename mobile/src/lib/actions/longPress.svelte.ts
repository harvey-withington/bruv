// Long-press detection: fires `handler` after the user has held a
// pointer on the node for `ms` milliseconds without significant movement.
// Suppresses the synthetic click that follows pointerup so the
// short-tap handler on the same element doesn't fire too. Pairs well
// with: short-tap navigates / opens, long-press enters a selection
// mode, drag (caught by a separate action) reorders.
//
// MOVE_CANCEL_PX matches the DnD action's threshold so the same
// finger-drift tolerance applies across all hold-style gestures.

export type LongPressOptions = {
  /** Milliseconds before the handler fires. Default 500 — slightly
   *  longer than a drag long-press (~250) so the gestures stay distinct
   *  if both apply to the same element. */
  ms?: number
}

const MOVE_CANCEL_PX = 6

export function longPress(
  node: HTMLElement,
  handler: (ev: PointerEvent) => void,
  options: LongPressOptions = {},
) {
  const opts = { ms: 500, ...options }
  let timer: ReturnType<typeof setTimeout> | null = null
  let startX = 0
  let startY = 0
  let armed = false
  let savedClickRemove: (() => void) | null = null

  function swallowNextClick() {
    const fn = (e: Event) => {
      e.stopPropagation()
      e.preventDefault()
      window.removeEventListener('click', fn, true)
      savedClickRemove = null
    }
    window.addEventListener('click', fn, true)
    savedClickRemove = () => window.removeEventListener('click', fn, true)
    setTimeout(() => savedClickRemove?.(), 250)
  }

  function onDown(ev: PointerEvent) {
    if (ev.button !== 0 && ev.pointerType === 'mouse') return
    armed = false
    startX = ev.clientX
    startY = ev.clientY
    timer = setTimeout(() => {
      timer = null
      armed = true
      swallowNextClick()
      try {
        navigator.vibrate?.(15)
      } catch {
        /* not all browsers support vibrate */
      }
      handler(ev)
    }, opts.ms)
  }

  function onMove(ev: PointerEvent) {
    if (!timer) return
    if (Math.hypot(ev.clientX - startX, ev.clientY - startY) > MOVE_CANCEL_PX) {
      clearTimeout(timer)
      timer = null
    }
  }

  function onUp() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    armed = false
  }

  node.addEventListener('pointerdown', onDown)
  node.addEventListener('pointermove', onMove)
  node.addEventListener('pointerup', onUp)
  node.addEventListener('pointercancel', onUp)

  return {
    update(next: (ev: PointerEvent) => void) {
      handler = next
    },
    destroy() {
      if (timer) clearTimeout(timer)
      savedClickRemove?.()
      node.removeEventListener('pointerdown', onDown)
      node.removeEventListener('pointermove', onMove)
      node.removeEventListener('pointerup', onUp)
      node.removeEventListener('pointercancel', onUp)
    },
  }
}
