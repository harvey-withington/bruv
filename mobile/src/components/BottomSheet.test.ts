import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { createRawSnippet } from 'svelte'
import BottomSheet from './BottomSheet.svelte'

// REGRESSIONS (2026-08-09): BottomSheet's popstate handler used to call
// onClose() on EVERY popstate — including the one produced by a nested
// descendant sheet's own cleanup history.back(). Picking a deck target
// inside Capture Options therefore closed Capture Options too, eating
// two history entries and discarding the user's choices. Escape had the
// same shape: every mounted sheet heard the window keydown, so one press
// closed the whole stack. Both fixes discriminate on history.state — a
// sheet acts only when it is the TOPMOST entry (its own key is current).
//
// These tests drive history.state + synthetic popstate directly instead
// of real jsdom traversals (history.back() is async in jsdom and would
// make the assertions racy). The state transitions modelled are exactly
// the ones the browser produces.

const body = createRawSnippet(() => ({ render: () => '<div>sheet body</div>' }))

function mountSheet(historyKey: string) {
  const onClose = vi.fn()
  const utils = render(BottomSheet, {
    props: { title: 'Test sheet', historyKey, onClose, children: body },
  })
  return { onClose, ...utils }
}

beforeEach(() => {
  // Neutral baseline entry so a previous test's state can't leak in.
  window.history.replaceState({}, '', '/m/')
})

describe('BottomSheet history entry', () => {
  it('pushes its own entry on mount', () => {
    mountSheet('outer')
    expect(window.history.state?.outer).toBe(true)
  })

  it('ignores a popstate that lands back ON its own entry (descendant closed)', () => {
    const { onClose } = mountSheet('outer')
    // A nested sheet above us closed: its cleanup popped ITS entry, so
    // the popstate fires with OUR entry current again.
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes when its OWN entry is popped', () => {
    const { onClose } = mountSheet('outer')
    // Hardware Back popped our entry: current state no longer has our key.
    window.history.replaceState({}, '', '/m/')
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('cleanup pops its still-current entry exactly once, and leaves foreign entries alone', () => {
    const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})

    // Programmatic close with our entry still current → cleanup pops it.
    const first = mountSheet('outer')
    first.unmount()
    expect(back).toHaveBeenCalledTimes(1)

    // Entry already consumed (state moved on) → cleanup must NOT pop
    // someone else's entry.
    back.mockClear()
    const second = mountSheet('outer')
    window.history.replaceState({ somebodyElse: true }, '', '/m/')
    second.unmount()
    expect(back).not.toHaveBeenCalled()

    back.mockRestore()
  })
})

describe('BottomSheet Escape', () => {
  it('closes the sheet whose entry is topmost', () => {
    const { onClose } = mountSheet('outer')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('does NOT close a sheet buried under another sheet\'s entry', () => {
    const { onClose } = mountSheet('outer')
    // An inner sheet mounted above us — its entry is now current.
    window.history.pushState({ inner: true }, '', '/m/')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).not.toHaveBeenCalled()
  })

  it('with two sheets mounted, one Escape closes only the inner one', () => {
    const outer = mountSheet('outer')
    const inner = mountSheet('inner')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(inner.onClose).toHaveBeenCalledTimes(1)
    expect(outer.onClose).not.toHaveBeenCalled()
  })
})
