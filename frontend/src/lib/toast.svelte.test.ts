import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { showToast, dismissToast, toasts } from './toast.svelte'

describe('toast state machine', () => {
  beforeEach(() => {
    toasts.list = []
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('showToast appends a toast with the given type and message', () => {
    showToast('saved', 'success')
    expect(toasts.list).toHaveLength(1)
    expect(toasts.list[0].message).toBe('saved')
    expect(toasts.list[0].type).toBe('success')
  })

  it('defaults to info when type is omitted', () => {
    showToast('hi')
    expect(toasts.list[0].type).toBe('info')
  })

  it('auto-dismisses after the given duration', () => {
    showToast('flash', 'info', 1000)
    expect(toasts.list).toHaveLength(1)
    vi.advanceTimersByTime(1000) // duration → animate-out starts
    vi.advanceTimersByTime(250)  // animation duration → removed
    expect(toasts.list).toHaveLength(0)
  })

  it('dismissToast removes the toast after its animation', () => {
    showToast('dismiss me', 'info', 10_000) // long auto-dismiss we won't reach
    const id = toasts.list[0].id
    dismissToast(id)
    expect(toasts.list[0].dismissing).toBe(true)
    vi.advanceTimersByTime(250)
    expect(toasts.list).toHaveLength(0)
  })

  it('dismissing an unknown id is a no-op', () => {
    showToast('keep me')
    const before = toasts.list.length
    dismissToast('does-not-exist')
    expect(toasts.list).toHaveLength(before)
  })

  it('multiple toasts coexist and dismiss independently', () => {
    showToast('first', 'info', 500)
    showToast('second', 'error', 1500)
    expect(toasts.list).toHaveLength(2)
    vi.advanceTimersByTime(500 + 250)
    expect(toasts.list.map(t => t.message)).toEqual(['second'])
    vi.advanceTimersByTime(1000 + 250)
    expect(toasts.list).toHaveLength(0)
  })
})
