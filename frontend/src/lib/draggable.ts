/**
 * Svelte action: makes an element draggable by its header.
 * Usage: <div use:draggable={{ handle: '.modal-header' }}>
 *
 * The handle selector identifies which child element initiates the drag.
 * If no handle is given, the element itself is the handle.
 * Dragging is cancelled if the mousedown target is an input, textarea, button, or [contenteditable].
 */
export function draggable(node: HTMLElement, opts?: { handle?: string }) {
  let offsetX = 0
  let offsetY = 0
  let dragging = false
  const selector = opts?.handle

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

  function onMouseDown(e: MouseEvent) {
    if (e.button !== 0) return
    const target = e.target as HTMLElement
    if (isInteractive(target)) return
    if (!isInHandle(target)) return

    const rect = node.getBoundingClientRect()

    // On first drag, switch from centered layout to fixed positioning
    if (!node.style.left) {
      node.style.position = 'fixed'
      node.style.left = rect.left + 'px'
      node.style.top = rect.top + 'px'
      node.style.margin = '0'
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
