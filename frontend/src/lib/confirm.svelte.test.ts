import { describe, it, expect, beforeEach } from 'vitest'
import { showConfirm, resolveConfirm, confirmState } from './confirm.svelte'

describe('confirm state machine', () => {
  beforeEach(() => {
    // Reset in case a prior test left state around
    confirmState.visible = false
    confirmState.message = ''
    confirmState.resolve = null
  })

  it('showConfirm surfaces the message and becomes visible', async () => {
    const p = showConfirm('delete this card?')
    expect(confirmState.visible).toBe(true)
    expect(confirmState.message).toBe('delete this card?')
    // Drain the promise so it doesn't leak between tests
    resolveConfirm(false)
    await p
  })

  it('resolveConfirm(true) resolves with true and hides the dialog', async () => {
    const p = showConfirm('ok?')
    resolveConfirm(true)
    await expect(p).resolves.toBe(true)
    expect(confirmState.visible).toBe(false)
    expect(confirmState.resolve).toBe(null)
  })

  it('resolveConfirm(false) resolves with false', async () => {
    const p = showConfirm('cancel?')
    resolveConfirm(false)
    await expect(p).resolves.toBe(false)
    expect(confirmState.visible).toBe(false)
  })

  it('guards against a stale resolveConfirm when no prompt is active', () => {
    // No promise pending — should not throw
    expect(() => resolveConfirm(true)).not.toThrow()
    expect(confirmState.visible).toBe(false)
  })
})
