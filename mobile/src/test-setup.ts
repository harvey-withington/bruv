import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/svelte'

// Mirrors frontend/src/test-setup.ts. Auto-unmount rendered components
// so module-level stores (toast, prefs) don't leak between tests.
afterEach(() => {
  cleanup()
  localStorage.clear()
})

// jsdom implements neither of these, and the mobile UI uses both:
// bottom sheets animate, and pages read the visual viewport for
// keyboard-aware sizing.
if (typeof Element !== 'undefined' && !Element.prototype.animate) {
  Element.prototype.animate = function (): Animation {
    return {
      cancel() {},
      finish() {},
      play() {},
      pause() {},
      reverse() {},
      addEventListener() {},
      removeEventListener() {},
      onfinish: null,
      oncancel: null,
      onremove: null,
      playState: 'finished',
      finished: Promise.resolve({} as Animation),
      ready: Promise.resolve({} as Animation),
    } as unknown as Animation
  }
}

if (typeof window !== 'undefined' && !window.visualViewport) {
  Object.defineProperty(window, 'visualViewport', {
    value: {
      height: 800,
      width: 400,
      addEventListener() {},
      removeEventListener() {},
    },
    writable: true,
  })
}
