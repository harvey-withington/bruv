/**
 * Svelte action: makes an element draggable by its header.
 * Usage: <div use:draggable={{ handle: '.modal-header' }}>
 *
 * The handle selector identifies which child element initiates the drag.
 * If no handle is given, the element itself is the handle.
 * Dragging is cancelled if the mousedown target is an input, textarea, button, or [contenteditable].
 *
 * Options:
 *   handle     – CSS selector for the drag handle within the node
 *   persistKey – localStorage key to save/restore position across sessions
 */
export function draggable(node: HTMLElement, opts?: { handle?: string; persistKey?: string }) {
  let offsetX = 0
  let offsetY = 0
  let dragging = false
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

  function onMouseDown(e: MouseEvent) {
    if (e.button !== 0) return
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

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
    e.preventDefault()
  }

  function onMouseMove(e: MouseEvent) {
    if (!dragging) return
    node.style.left = (e.clientX - offsetX) + 'px'
    node.style.top = (e.clientY - offsetY) + 'px'
  }

  function onMouseUp() {
    dragging = false
    savePosition()
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
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

  // Pin to fixed positioning immediately so layout changes (e.g. chat panel resize)
  // don't re-center the modal via flexbox. Restore saved position if available.
  requestAnimationFrame(() => {
    if (!node.style.left) {
      if (persistKey) {
        try {
          const saved = JSON.parse(localStorage.getItem(persistKey) || '')
          if (saved && typeof saved.left === 'number' && typeof saved.top === 'number') {
            // Clamp to viewport so the dialog isn't off-screen after a resize
            const rect = node.getBoundingClientRect()
            const left = Math.max(0, Math.min(saved.left, window.innerWidth - rect.width))
            const top = Math.max(0, Math.min(saved.top, window.innerHeight - 100))
            pinToFixed(left, top)
            return
          }
        } catch { /* fall through to centre */ }
      }
      const rect = node.getBoundingClientRect()
      pinToFixed(rect.left, rect.top)
    }
  })

  applyCursor()
  const observer = new MutationObserver(applyCursor)
  observer.observe(node, { childList: true, subtree: true })

  node.addEventListener('mousedown', onMouseDown)

  return {
    destroy() {
      node.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      observer.disconnect()
    }
  }
}
