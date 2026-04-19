import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/svelte'

// Auto-unmount any components rendered in a test so shared stores
// (e.g., confirmState) don't leak between tests — otherwise multiple
// rendered copies all subscribe to the same state and queries hit
// duplicate elements.
afterEach(() => {
  cleanup()
})

// jsdom does not implement the Web Animations API. Svelte's built-in
// transitions (fade/slide/fly) call `element.animate(...)` which
// throws. Stub it to a no-op so transitions don't blow up in tests.
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
