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
 * inlineEdit moved to shared/inlineEdit.ts (both surfaces implement the
 * same keyboard entry contract — see UI-CONVENTIONS "Keyboard entry").
 * Re-exported here so existing `from '../lib/actions'` imports keep
 * working unchanged.
 */
export { inlineEdit, type InlineEditParams } from '@shared/inlineEdit'

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

/**
 * Svelte action: calls `onOutsideClick` on any click that lands outside
 * the node and outside every element in `exclude` (typically the
 * trigger button that opens the popover — excluding it means the
 * trigger's own click can toggle the popover without this action
 * immediately firing a close as well).
 *
 * Attach directly to the popover/menu content and pair with
 * conditional rendering so it only listens while open:
 *   {#if open}
 *     <div use:clickOutside={{ onOutsideClick: () => open = false, exclude: [triggerEl] }}>…</div>
 *   {/if}
 */
export function clickOutside(
  node: HTMLElement,
  options: { onOutsideClick: (event: MouseEvent) => void; exclude?: (HTMLElement | null | undefined)[] },
) {
  let { onOutsideClick, exclude } = options

  function handleClick(e: MouseEvent) {
    const target = e.target as Node
    if (node.contains(target)) return
    if (exclude?.some(el => el?.contains(target))) return
    onOutsideClick(e)
  }

  document.addEventListener('click', handleClick)

  return {
    update(newOptions: { onOutsideClick: (event: MouseEvent) => void; exclude?: (HTMLElement | null | undefined)[] }) {
      onOutsideClick = newOptions.onOutsideClick
      exclude = newOptions.exclude
    },
    destroy() {
      document.removeEventListener('click', handleClick)
    },
  }
}

/**
 * Keyboard resizing for a splitter (`role="separator"` with aria-value*),
 * the twin of dragging it: arrows move it 16 px, Shift+arrows 48 px.
 * `axis` 'x' listens to ←/→, 'y' to ↑/↓. `onMove(delta)` gets the
 * handle's movement in px (+ = right/down), exactly like a drag's pointer
 * delta, so each splitter keeps its own sizing rule; `onCommit` saves, as
 * at the end of a drag.
 *
 * The action makes the handle focusable itself: ARIA treats a focusable
 * separator as a widget, but svelte-check's a11y rule doesn't know that and
 * flags `tabindex="0"` on it in markup.
 */
export function keyboardResize(
  node: HTMLElement,
  params: { axis: 'x' | 'y'; onMove: (delta: number) => void; onCommit: () => void },
) {
  let p = params
  node.tabIndex = 0
  function onKeydown(e: KeyboardEvent) {
    const [back, forward] = p.axis === 'x' ? ['ArrowLeft', 'ArrowRight'] : ['ArrowUp', 'ArrowDown']
    if (e.key !== back && e.key !== forward) return
    e.preventDefault()
    const step = e.shiftKey ? 48 : 16
    p.onMove(e.key === forward ? step : -step)
    p.onCommit()
  }
  node.addEventListener('keydown', onKeydown)
  return {
    update(next: typeof params) { p = next },
    destroy() { node.removeEventListener('keydown', onKeydown) },
  }
}

/**
 * Keeps Tab inside a dialog, moves focus into it on open and gives it back
 * on close. On open (after children mount, so their own focusOnMount wins):
 * if focus is already inside, nothing changes; otherwise `[data-autofocus]`
 * is focused, or else the dialog itself when it is focusable (tabindex), so
 * keyboard and screen-reader users land in the dialog without a button
 * being pre-selected. On close, focus returns to what had it before, unless
 * something else has taken it meanwhile.
 */
export function focusTrap(node: HTMLElement) {
  const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
  queueMicrotask(() => {
    if (!node.isConnected || node.contains(document.activeElement)) return
    const target = node.querySelector<HTMLElement>('[data-autofocus]') ?? (node.hasAttribute('tabindex') ? node : null)
    target?.focus({ preventScroll: true })
  })

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
  return {
    destroy() {
      node.removeEventListener('keydown', handleKeydown)
      // Only when focus was inside (or fell to <body> as the dialog left):
      // a dialog that closed because the user clicked elsewhere keeps that.
      const active = document.activeElement
      const lost = !active || active === document.body || node.contains(active)
      if (lost && previous?.isConnected) previous.focus({ preventScroll: true })
    },
  }
}
