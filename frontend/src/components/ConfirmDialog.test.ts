import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import ConfirmDialog from './ConfirmDialog.svelte'
import { confirmState, showConfirm } from '../lib/confirm.svelte'

// Catches: destructive-action regressions where the dialog stops
// gating (button → action fires without prompt), or where the
// confirm/cancel wiring gets swapped. CLAUDE.md hard rule: all
// destructive actions must route through ConfirmDialog.
describe('ConfirmDialog', () => {
  beforeEach(() => {
    confirmState.visible = false
    confirmState.message = ''
    confirmState.resolve = null
  })

  afterEach(() => {
    // Drain any pending prompt so it doesn't leak into the next test
    if (confirmState.resolve) confirmState.resolve(false)
    confirmState.visible = false
    confirmState.resolve = null
  })

  it('does not render the dialog until showConfirm is called', () => {
    render(ConfirmDialog)
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('renders the message when showConfirm fires', async () => {
    render(ConfirmDialog)
    const promise = showConfirm('delete card?')
    await Promise.resolve()
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
    expect(screen.getByText('delete card?')).toBeInTheDocument()
    // Clean up the pending promise
    confirmState.resolve?.(false)
    await promise
  })

  it('Confirm button resolves the promise with true and hides the dialog', async () => {
    render(ConfirmDialog)
    const promise = showConfirm('really delete?')
    await Promise.resolve()
    const confirmBtn = screen.getByRole('button', { name: /confirm/i })
    await fireEvent.click(confirmBtn)
    await expect(promise).resolves.toBe(true)
    expect(confirmState.visible).toBe(false)
  })

  it('Cancel button resolves with false without taking the action', async () => {
    render(ConfirmDialog)
    const promise = showConfirm('really delete?')
    await Promise.resolve()
    const cancelBtn = screen.getByRole('button', { name: /cancel/i })
    await fireEvent.click(cancelBtn)
    await expect(promise).resolves.toBe(false)
    expect(confirmState.visible).toBe(false)
  })

  it('backdrop click cancels (equivalent to Cancel)', async () => {
    const { container } = render(ConfirmDialog)
    const promise = showConfirm('really delete?')
    await Promise.resolve()
    const backdrop = container.querySelector('.confirm-backdrop') as HTMLElement
    expect(backdrop).toBeTruthy()
    await fireEvent.click(backdrop)
    await expect(promise).resolves.toBe(false)
  })

  it('Escape key cancels, Enter key confirms', async () => {
    render(ConfirmDialog)

    const escapePromise = showConfirm('first?')
    await Promise.resolve()
    await fireEvent.keyDown(window, { key: 'Escape' })
    await expect(escapePromise).resolves.toBe(false)

    const enterPromise = showConfirm('second?')
    await Promise.resolve()
    await fireEvent.keyDown(window, { key: 'Enter' })
    await expect(enterPromise).resolves.toBe(true)
  })

  // Regression guard: a caller that forgets to await showConfirm and
  // instead calls the destructive action unconditionally would mean
  // the dialog appears AFTER the action fires — a catastrophic bug.
  // This test proves the promise does not resolve until the user
  // clicks a button, so callers that `await` it cannot bypass it.
  it('does not auto-resolve — caller must await user interaction', async () => {
    render(ConfirmDialog)
    let resolved = false
    const promise = showConfirm('slow delete?').then(result => {
      resolved = true
      return result
    })
    // Give the microtask queue a chance to flush
    await Promise.resolve()
    await Promise.resolve()
    expect(resolved).toBe(false)
    expect(confirmState.visible).toBe(true)
    // Clean up
    confirmState.resolve?.(false)
    await promise
  })
})
