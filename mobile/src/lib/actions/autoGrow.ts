// Auto-size a textarea to its content, capped to the VISUAL viewport.
//
// window.innerHeight is the LAYOUT viewport — it does not shrink when
// the virtual keyboard opens, so a "60% of innerHeight" textarea could
// swallow the entire visible area and push the ✓ Done row off screen
// (the bug this replaced: two hand-rolled copies capped on innerHeight).
// visualViewport tracks the keyboard; the cap also reserves room below
// the textarea for the commit affordance.

const DONE_RESERVE = 72 // px kept visible below the textarea (✓ row + gap)
const MIN_HEIGHT = 120 // px floor so tiny viewports still get a usable editor

export function growTextarea(el: HTMLTextAreaElement | null): void {
  if (!el) return
  el.style.height = 'auto'
  const viewport = window.visualViewport?.height ?? window.innerHeight
  const cap = Math.max(MIN_HEIGHT, viewport * 0.6 - DONE_RESERVE)
  el.style.height = `${Math.min(el.scrollHeight, cap)}px`
}

/**
 * Svelte action: keeps the textarea sized on input, focus, AND
 * visual-viewport resizes — the keyboard opens AFTER focus fires, so a
 * one-shot resize at focus time still overshoots; the viewport resize
 * event is what re-caps once the keyboard has settled.
 */
export function autoGrow(node: HTMLTextAreaElement) {
  const grow = () => growTextarea(node)
  node.addEventListener('input', grow)
  node.addEventListener('focus', grow)
  window.visualViewport?.addEventListener('resize', grow)
  grow()
  return {
    destroy() {
      node.removeEventListener('input', grow)
      node.removeEventListener('focus', grow)
      window.visualViewport?.removeEventListener('resize', grow)
    },
  }
}
