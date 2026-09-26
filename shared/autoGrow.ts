// Auto-size a textarea to its content, capped to the VISUAL viewport.
// Shared by both surfaces (mobile re-exports it from lib/actions/autoGrow).
//
// window.innerHeight is the LAYOUT viewport — it does not shrink when
// the virtual keyboard opens, so a "60% of innerHeight" textarea could
// swallow the entire visible area and push the ✓ Done row off screen
// (the bug this replaced: two hand-rolled copies capped on innerHeight).
// visualViewport tracks the keyboard; the cap also reserves room below
// the textarea for the commit affordance.

const DONE_RESERVE = 72 // px kept visible below the textarea (✓ row + gap)
const MIN_HEIGHT = 120 // px floor so tiny viewports still get a usable editor

export interface AutoGrowOptions {
  /**
   * Floor for the cap, in px. The default (120) suits a description or a
   * document; a list/checklist ITEM wants to start one line tall and grow
   * from there, so it passes 0 and the textarea is exactly as tall as its
   * text.
   */
  minHeight?: number
  /**
   * Ceiling in px, after which the textarea scrolls instead of growing.
   * List/checklist items pass ~8 lines; the viewport cap still applies.
   */
  maxHeight?: number
  /**
   * The bound value, passed only so a programmatic change (a serial add
   * input cleared after commit) re-sizes the textarea — `input` events
   * don't fire for those. Not read.
   */
  value?: string
}

export function growTextarea(el: HTMLTextAreaElement | null, options: AutoGrowOptions = {}): void {
  if (!el) return
  el.style.height = 'auto'
  const viewport = window.visualViewport?.height ?? window.innerHeight
  let cap = Math.max(options.minHeight ?? MIN_HEIGHT, viewport * 0.6 - DONE_RESERVE)
  if (options.maxHeight !== undefined) cap = Math.min(cap, options.maxHeight)
  el.style.height = `${Math.min(el.scrollHeight, cap)}px`
}

/**
 * Svelte action: keeps the textarea sized on input, focus, AND
 * visual-viewport resizes — the keyboard opens AFTER focus fires, so a
 * one-shot resize at focus time still overshoots; the viewport resize
 * event is what re-caps once the keyboard has settled.
 */
export function autoGrow(node: HTMLTextAreaElement, options: AutoGrowOptions = {}) {
  let current = options
  const grow = () => growTextarea(node, current)
  node.addEventListener('input', grow)
  node.addEventListener('focus', grow)
  window.visualViewport?.addEventListener('resize', grow)
  grow()
  return {
    update(next: AutoGrowOptions = {}) {
      current = next
      grow()
    },
    destroy() {
      node.removeEventListener('input', grow)
      node.removeEventListener('focus', grow)
      window.visualViewport?.removeEventListener('resize', grow)
    },
  }
}
