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
  params: { onCommit: () => void; onCancel: () => void },
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

  function handleBlur() {
    commit()
  }

  node.addEventListener('keydown', handleKeydown as EventListener)
  node.addEventListener('blur', handleBlur)

  return {
    update(newParams: typeof params) {
      params = newParams
      committed = false
    },
    destroy() {
      node.removeEventListener('keydown', handleKeydown as EventListener)
      node.removeEventListener('blur', handleBlur)
    },
  }
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * Svelte action: traps Tab / Shift+Tab focus within the element.
 * Apply to the dialog container (not the backdrop).
 */
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
