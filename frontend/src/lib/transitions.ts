/**
 * Shared transition helpers for Svelte components.
 *
 * These are CSS-animation-based utilities that can be applied as Svelte actions
 * or via plain CSS classes. They read the global animation design tokens from
 * style.css so durations/easings stay consistent app-wide.
 *
 * Usage:
 *   <div use:animateIn={{ name: 'fade-in-up', duration: 200 }}>…</div>
 *   <div use:animateOut={{ name: 'fade-in', duration: 150 }} on:outroend={…}>…</div>
 */

/** Svelte action: plays a CSS @keyframes animation when the element mounts. */
export function animateIn(
  node: HTMLElement,
  opts: { name?: string; duration?: number; delay?: number; easing?: string } = {},
) {
  const {
    name = 'fade-in-up',
    duration = 200,
    delay = 0,
    easing = 'cubic-bezier(0.16, 1, 0.3, 1)',
  } = opts

  node.style.animation = `${name} ${duration}ms ${easing} ${delay}ms both`

  function handleEnd() {
    node.style.animation = ''
    node.removeEventListener('animationend', handleEnd)
  }
  node.addEventListener('animationend', handleEnd)

  return {
    destroy() {
      node.removeEventListener('animationend', handleEnd)
    },
  }
}

/** Plays an exit animation; resolves when done so you can await before unmounting. */
export function animateOutEl(
  node: HTMLElement,
  opts: { name?: string; duration?: number; easing?: string } = {},
): Promise<void> {
  const {
    name = 'fade-in-up',
    duration = 150,
    easing = 'cubic-bezier(0.16, 1, 0.3, 1)',
  } = opts

  return new Promise((resolve) => {
    node.style.animation = `${name} ${duration}ms ${easing} reverse forwards`
    function handleEnd() {
      node.removeEventListener('animationend', handleEnd)
      resolve()
    }
    node.addEventListener('animationend', handleEnd)
  })
}

/**
 * Svelte action: staggers fade-in-up on direct children.
 * Applies a small delay increment to each child to create
 * a cascading entrance effect (like Trello column card load).
 *
 * Usage:
 *   <div use:staggerChildren={{ stagger: 30, duration: 200 }}>
 *     {#each items as item}
 *       <div class="child">…</div>
 *     {/each}
 *   </div>
 */
export function staggerChildren(
  node: HTMLElement,
  opts: { stagger?: number; duration?: number; name?: string; easing?: string; selector?: string } = {},
) {
  const {
    stagger = 25,
    duration = 250,
    name = 'fade-in-up',
    easing = 'cubic-bezier(0.16, 1, 0.3, 1)',
    selector,
  } = opts

  const children = selector
    ? Array.from(node.querySelectorAll<HTMLElement>(selector))
    : Array.from(node.children) as HTMLElement[]

  // Cap stagger so even long lists finish in a reasonable time
  const maxDelay = 400
  const effectiveStagger = children.length > 0
    ? Math.min(stagger, maxDelay / children.length)
    : stagger

  for (let i = 0; i < children.length; i++) {
    const child = children[i]
    const delay = i * effectiveStagger
    child.style.animation = `${name} ${duration}ms ${easing} ${delay}ms both`
  }

  // Clean up after all animations finish
  const totalTime = (children.length - 1) * effectiveStagger + duration
  const timer = setTimeout(() => {
    for (const child of children) {
      child.style.animation = ''
    }
  }, totalTime + 50)

  return {
    destroy() {
      clearTimeout(timer)
      for (const child of children) {
        child.style.animation = ''
      }
    },
  }
}
