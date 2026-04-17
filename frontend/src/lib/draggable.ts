/**
 * Svelte action: makes an element draggable by its header.
 * Usage: <div use:draggable={{ handle: '.modal-header' }}>
 *
 * The handle selector identifies which child element initiates the drag.
 * If no handle is given, the element itself is the handle.
 * Dragging is cancelled if the pointerdown target is an input, textarea, button, or [contenteditable].
 *
 * Options:
 *   handle     – CSS selector for the drag handle within the node
 *   persistKey – localStorage key to save/restore position across sessions
 *
 * Uses pointer events so mouse, touch, and stylus all work — essential
 * for tablet/touch viewport support. Mouse-only left-click filtering
 * is preserved via PointerEvent.button.
 */
export function draggable(node: HTMLElement, opts?: { handle?: string; persistKey?: string }) {
  let offsetX = 0
  let offsetY = 0
  let dragging = false
  let activePointerId: number | null = null
  const selector = opts?.handle
  const persistKey = opts?.persistKey

  function isInteractive(el: HTMLElement): boolean {
    const tag = el.tagName
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'BUTTON' || tag === 'SELECT' || tag === 'A') return true
    if (el.isContentEditable) return true
    if (el.closest('button, input, textarea, select, a, [contenteditable]')) return true
    return false
  }

  function isInHandle(el: HTMLElement): boolean {
    if (!selector) return true
    const handle = node.querySelector(selector)
    return !!handle && (handle === el || handle.contains(el))
  }

  function pinToFixed(left: number, top: number) {
    node.style.position = 'fixed'
    node.style.left = left + 'px'
    node.style.top = top + 'px'
    node.style.margin = '0'
  }

  function savePosition() {
    if (persistKey) {
      localStorage.setItem(persistKey, JSON.stringify({
        left: parseFloat(node.style.left),
        top: parseFloat(node.style.top),
      }))
    }
  }

  function onPointerDown(e: PointerEvent) {
    // Mouse: only left button drags. Touch and pen report button=0 too,
    // so this doesn't interfere with those input sources.
    if (e.pointerType === 'mouse' && e.button !== 0) return
    const target = e.target as HTMLElement
    if (isInteractive(target)) return
    if (!isInHandle(target)) return

    const rect = node.getBoundingClientRect()

    if (!node.style.left) {
      pinToFixed(rect.left, rect.top)
    }

    offsetX = e.clientX - rect.left
    offsetY = e.clientY - rect.top
    dragging = true
    activePointerId = e.pointerId

    // Capture on the node so move/up events keep firing if the pointer
    // leaves the element — especially important for touch where losing
    // contact with the handle mid-drag would otherwise end the gesture.
    try { node.setPointerCapture(e.pointerId) } catch { /* capture may fail during rapid re-entry; drag still works */ }

    document.addEventListener('pointermove', onPointerMove)
    document.addEventListener('pointerup', onPointerUp)
    document.addEventListener('pointercancel', onPointerUp)
    e.preventDefault()
  }

  function onPointerMove(e: PointerEvent) {
    if (!dragging) return
    if (activePointerId !== null && e.pointerId !== activePointerId) return
    node.style.left = (e.clientX - offsetX) + 'px'
    node.style.top = (e.clientY - offsetY) + 'px'
  }

  function onPointerUp(e: PointerEvent) {
    if (activePointerId !== null && e.pointerId !== activePointerId) return
    dragging = false
    savePosition()
    if (activePointerId !== null) {
      try { node.releasePointerCapture(activePointerId) } catch { /* already released */ }
      activePointerId = null
    }
    document.removeEventListener('pointermove', onPointerMove)
    document.removeEventListener('pointerup', onPointerUp)
    document.removeEventListener('pointercancel', onPointerUp)
  }

  // Apply grab cursor to handle via CSS observer
  function applyCursor() {
    if (selector) {
      const handle = node.querySelector(selector) as HTMLElement
      if (handle) handle.style.cursor = 'grab'
    } else {
      node.style.cursor = 'grab'
    }
  }

  // Restore a saved position if persistKey is set. Otherwise leave the dialog
  // flexbox-centred — pinToFixed is called lazily on the first pointerdown so that
  // we never read getBoundingClientRect() before the browser has finished layout.
  if (persistKey) {
    requestAnimationFrame(() => {
      if (!node.style.left) {
        try {
          const saved = JSON.parse(localStorage.getItem(persistKey) || '')
          if (saved && typeof saved.left === 'number' && typeof saved.top === 'number') {
            const rect = node.getBoundingClientRect()
            const left = Math.max(0, Math.min(saved.left, window.innerWidth - rect.width))
            const top = Math.max(0, Math.min(saved.top, window.innerHeight - 100))
            pinToFixed(left, top)
          }
        } catch { /* fall through — stay flexbox-centred */ }
      }
    })
  }

  applyCursor()
  const observer = new MutationObserver(applyCursor)
  observer.observe(node, { childList: true, subtree: true })

  node.addEventListener('pointerdown', onPointerDown)

  return {
    destroy() {
      node.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('pointermove', onPointerMove)
      document.removeEventListener('pointerup', onPointerUp)
      document.removeEventListener('pointercancel', onPointerUp)
      observer.disconnect()
    }
  }
}
