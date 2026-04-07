/**
 * Svelte action: focuses the element when it mounts.
 * Pass `selectAll = true` to also select all text.
 * Pair with conditional rendering so the element only mounts when focus is needed:
 *   {#if condition}
 *     <input use:focusOnMount />
 *   {/if}
 */
export function focusOnMount(node: HTMLInputElement | HTMLTextAreaElement, selectAll = false) {
  node.focus()
  if (selectAll && 'select' in node) node.select()
}

/**
 * Svelte action: encapsulates the inline-edit commit/cancel pattern.
 * Commits on Enter or blur, cancels on Escape.
 * Prevents the classic double-fire bug where pressing Enter sets state that
 * removes the element, causing blur to call commit a second time.
 *
 * Usage:
 *   <input use:inlineEdit={{ onCommit: () => save(), onCancel: () => revert() }} />
 */
export function inlineEdit(
  node: HTMLInputElement | HTMLTextAreaElement,
  params: { onCommit: () => void; onCancel: () => void; container?: string },
) {
  let committed = false

  function commit() {
    if (committed) return
    committed = true
    params.onCommit()
  }

  function cancel() {
    if (committed) return
    committed = true
    params.onCancel()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Escape') {
      e.stopPropagation()
      cancel()
    }
  }

  function handleBlur(e: FocusEvent) {
    // If a container selector is set, only commit when focus leaves the container
    if (params.container) {
      const container = node.closest(params.container)
      const related = e.relatedTarget as HTMLElement | null
      if (container && related && container.contains(related)) return
    }
    commit()
  }

  node.addEventListener('keydown', handleKeydown as EventListener)
  node.addEventListener('blur', handleBlur as EventListener)

  return {
    update(newParams: typeof params) {
      params = newParams
      committed = false
    },
    destroy() {
      node.removeEventListener('keydown', handleKeydown as EventListener)
      node.removeEventListener('blur', handleBlur as EventListener)
    },
  }
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * Svelte action: traps Tab / Shift+Tab focus within the element.
 * Apply to the dialog container (not the backdrop).
 */
/**
 * Svelte action: moves the element to document.body so it escapes
 * any parent stacking context. Essential for nested modals.
 */
export function portal(node: HTMLElement) {
  document.body.appendChild(node)
  return { destroy() { node.remove() } }
}

/**
 * Svelte action: portals a dropdown to document.body and positions it fixed
 * relative to a trigger element. Repositions on scroll/resize and flips
 * above the trigger when there isn't enough space below.
 *
 * Usage:
 *   <div use:floatingDropdown={{ trigger: buttonEl }}>…</div>
 *   <div use:floatingDropdown={{ trigger: buttonEl, matchWidth: true }}>…</div>
 */
export function floatingDropdown(
  node: HTMLElement,
  options: { trigger: HTMLElement; matchWidth?: boolean },
) {
  let { trigger, matchWidth } = options
  document.body.appendChild(node)

  function reposition() {
    const rect = trigger.getBoundingClientRect()
    node.style.position = 'fixed'
    node.style.left = `${rect.left}px`
    node.style.top = `${rect.bottom + 4}px`
    node.style.zIndex = '10000'
    if (matchWidth) node.style.width = `${rect.width}px`

    // Flip above if overflowing viewport bottom
    requestAnimationFrame(() => {
      const nodeRect = node.getBoundingClientRect()
      if (nodeRect.bottom > window.innerHeight) {
        node.style.top = `${rect.top - nodeRect.height - 4}px`
      }
    })
  }

  reposition()
  window.addEventListener('scroll', reposition, true)
  window.addEventListener('resize', reposition)

  return {
    update(newOptions: { trigger: HTMLElement; matchWidth?: boolean }) {
      trigger = newOptions.trigger
      matchWidth = newOptions.matchWidth
      reposition()
    },
    destroy() {
      window.removeEventListener('scroll', reposition, true)
      window.removeEventListener('resize', reposition)
      node.remove()
    },
  }
}

export function focusTrap(node: HTMLElement) {
  function handleKeydown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return
    const focusable = Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(el => el.offsetParent !== null)
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
  node.addEventListener('keydown', handleKeydown)
  return { destroy() { node.removeEventListener('keydown', handleKeydown) } }
}
