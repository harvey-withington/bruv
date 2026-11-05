// Keeps a fixed, bottom-anchored overlay (a bottom sheet) inside the part
// of the screen the user can see. When the on-screen keyboard opens, the
// layout viewport stays the same size and the browser slides the whole
// page up to keep the focused field in view; the visual viewport is what
// shrinks. This sets two properties on the node, so its CSS can sit on top
// of the keyboard and shrink to fit instead:
//
//   --keyboard-inset   how much of the layout viewport's bottom is hidden
//                      (the keyboard, or the page having slid up): its bottom
//   --visible-height   the visual viewport's height: cap its height with it
//
// Without a visual viewport (old browsers) neither is set; CSS fallbacks
// keep the old layout.

export function keyboardInset(vv: Pick<VisualViewport, 'height' | 'offsetTop'>, layoutHeight: number): number {
  return Math.max(0, Math.round(layoutHeight - vv.height - vv.offsetTop))
}

export function fitVisualViewport(node: HTMLElement) {
  const vv = window.visualViewport
  if (!vv) return {}
  const update = () => {
    node.style.setProperty('--keyboard-inset', `${keyboardInset(vv, window.innerHeight)}px`)
    node.style.setProperty('--visible-height', `${Math.round(vv.height)}px`)
  }
  vv.addEventListener('resize', update)
  vv.addEventListener('scroll', update)
  update()
  return {
    destroy() {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    },
  }
}
