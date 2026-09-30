import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import type { WorkspaceSaveResult } from '@shared/types'
import type { DocumentSource } from '../lib/documentSource'
import DocumentSheet from './DocumentSheet.svelte'

// The sheet's save contract (pre-release sweep 10.4): one save in flight,
// each presenting the stamp its predecessor wrote (no false "changed
// elsewhere" for the user's own edit), and a close whose save failed can
// still be dismissed — after asking.

interface PendingSave {
  content: string
  expectedHash: string
  resolve: (r: WorkspaceSaveResult) => void
  reject: (err: unknown) => void
}

function makeSource() {
  const saves: PendingSave[] = []
  const source: DocumentSource = {
    path: 'notes.md',
    open: () => Promise.resolve({ content: 'hello', stamp: { hash: 'h0', size: 5 } }),
    stat: () => Promise.resolve({ hash: 'h0', size: 5 }),
    save: (content, expectedHash) =>
      new Promise<WorkspaceSaveResult>((resolve, reject) => {
        saves.push({ content, expectedHash, resolve, reject })
      }),
  }
  return { source, saves }
}

async function openEditor(source: DocumentSource, onClose = vi.fn()) {
  render(DocumentSheet, { source, onClose })
  await fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
  return screen.getByRole('textbox') as HTMLTextAreaElement
}

describe('DocumentSheet', () => {
  // An unmounted sheet pops its history entry; jsdom runs that traversal
  // on a later task — let it land before the next test pushes its own.
  beforeEach(() => new Promise<void>((r) => setTimeout(r, 20)))

  it('serializes saves and presents the stamp the previous save wrote', async () => {
    const { source, saves } = makeSource()
    const onClose = vi.fn()
    const editor = await openEditor(source, onClose)

    await fireEvent.input(editor, { target: { value: 'hello world' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() => expect(saves).toHaveLength(1))

    // Re-edit and close while the first save is still in flight.
    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    await fireEvent.input(screen.getByRole('textbox'), { target: { value: 'hello world!' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(saves).toHaveLength(1)

    saves[0].resolve({ diverged: false, stamp: { hash: 'h1', size: 11 } })
    await waitFor(() => expect(saves).toHaveLength(2))
    expect(saves[1]).toMatchObject({ content: 'hello world!', expectedHash: 'h1' })

    saves[1].resolve({ diverged: false, stamp: { hash: 'h2', size: 12 } })
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
    expect(screen.queryByText('Changed elsewhere')).toBeNull()
  })

  it('offers to discard when the save on close fails', async () => {
    const { source, saves } = makeSource()
    const onClose = vi.fn()
    const editor = await openEditor(source, onClose)

    await fireEvent.input(editor, { target: { value: 'offline edit' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(saves).toHaveLength(1))
    saves[0].reject(new Error('network down'))

    expect(await screen.findByText('Discard unsaved changes?')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()

    // Keep editing leaves the sheet open.
    await fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    expect(onClose).not.toHaveBeenCalled()

    await fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(saves).toHaveLength(2))
    saves[1].reject(new Error('network down'))
    await fireEvent.click(await screen.findByRole('button', { name: 'Discard' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('keeps its history entry when Back is refused by a failed save', async () => {
    const { source, saves } = makeSource()
    const onClose = vi.fn()
    const editor = await openEditor(source, onClose)
    await fireEvent.input(editor, { target: { value: 'offline edit' } })

    history.back()
    await waitFor(() => expect(saves).toHaveLength(1))
    saves[0].reject(new Error('network down'))

    expect(await screen.findByText('Discard unsaved changes?')).toBeInTheDocument()
    // Re-pushed: the next Back must not leave the page under the sheet.
    await waitFor(() => expect(history.state?.documentSheet).toBe(true))
    expect(onClose).not.toHaveBeenCalled()
  })
})
