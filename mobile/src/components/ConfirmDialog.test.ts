import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import ConfirmDialog from './ConfirmDialog.svelte'

// The dialog owns Escape and Enter: Enter activates the FOCUSED button
// (Cancel focused + Enter must cancel), and neither key may reach the
// page underneath — CardPage closes the card on window Ctrl+Enter.

function setup(destructive = false) {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(ConfirmDialog, { title: 'Delete block?', body: 'Gone for good.', destructive, onConfirm, onCancel })
  return { onConfirm, onCancel }
}

const pageKeydown = vi.fn()
window.addEventListener('keydown', pageKeydown)
afterEach(() => pageKeydown.mockReset())

describe('ConfirmDialog', () => {
  it('focuses Cancel on a destructive dialog, so a stray Enter cancels', async () => {
    const { onConfirm, onCancel } = setup(true)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus())

    await fireEvent.keyDown(document.activeElement!, { key: 'Enter' })

    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('Enter activates whichever button is focused', async () => {
    const { onConfirm, onCancel } = setup()
    const cancel = screen.getByRole('button', { name: 'Cancel' })
    cancel.focus()
    await fireEvent.keyDown(cancel, { key: 'Enter' })
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()

    const confirm = screen.getByRole('button', { name: 'Confirm' })
    confirm.focus()
    await fireEvent.keyDown(confirm, { key: 'Enter' })
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('keeps Escape and Ctrl+Enter from reaching the page underneath', async () => {
    const { onCancel } = setup(true)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus())

    await fireEvent.keyDown(document.activeElement!, { key: 'Enter', ctrlKey: true })
    await fireEvent.keyDown(document.activeElement!, { key: 'Escape' })

    expect(pageKeydown).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(2)
  })
})
