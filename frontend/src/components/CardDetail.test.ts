import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, screen, waitFor } from '@testing-library/svelte'
import CardDetail from './CardDetail.svelte'
import ConfirmDialog from './ConfirmDialog.svelte'
import { confirmState, showConfirm } from '../lib/confirm.svelte'
import { cardTypes } from '../lib/store.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'
import type { BackendEvent, Block, Card, EventCallback } from '@shared/types'

// Smoke test for the card dialog's core loop: open → card loads →
// edit the title → Enter → the adapter receives the mutation. Catches
// regressions in the GetCard wiring, the CardHeader bindable plumbing
// (editingTitle/titleDraft), and the saveTitle no-op guards.
describe('CardDetail smoke', () => {
  const testCard = (overrides: Partial<Card> = {}): Card => ({
    id: 'card-1',
    title: 'Original Title',
    description: '',
    type: '',
    tags: [],
    due_date: null,
    created_at: '2026-06-01T00:00:00Z',
    blocks: [],
    file_attachments: [],
    ...overrides,
  })

  let adapter: ReturnType<typeof createMockAdapter>

  beforeEach(() => {
    adapter = createMockAdapter()
    adapter.GetCard = vi.fn(async () => testCard())
    adapter.UpdateCardTitle = vi.fn(async (_id: string, title: string) => testCard({ title }))
    setBackend(adapter)
    cardTypes.list = []
  })

  it('fetches the card on open and renders its title', async () => {
    render(CardDetail, { props: { cardId: 'card-1', onClose: () => {} } })
    expect(await screen.findByText('Original Title')).toBeInTheDocument()
    expect(adapter.GetCard).toHaveBeenCalledWith('card-1')
  })

  it('click title → type → Enter persists via UpdateCardTitle', async () => {
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose: () => {} } })
    await screen.findByText('Original Title')

    const h2 = container.querySelector('.modal-title') as HTMLElement
    expect(h2).toBeTruthy()
    await fireEvent.click(h2)

    const input = await waitFor(() => {
      const el = container.querySelector('input.title-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    await fireEvent.input(input, { target: { value: 'Renamed Card' } })
    await fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(adapter.UpdateCardTitle).toHaveBeenCalledWith('card-1', 'Renamed Card')
    })
  })

  it('saving an unchanged title is a no-op (no adapter call)', async () => {
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose: () => {} } })
    await screen.findByText('Original Title')

    await fireEvent.click(container.querySelector('.modal-title') as HTMLElement)
    const input = await waitFor(() => {
      const el = container.querySelector('input.title-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    // Draft equals the current title — Enter must exit edit mode
    // without issuing a mutation.
    await fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(container.querySelector('input.title-input')).toBeNull()
    })
    expect(adapter.UpdateCardTitle).not.toHaveBeenCalled()
  })
})

// Escape layering: the dialog's own Escape closes it via onClose({ escaped:
// true }), but an active inline edit (e.g. the title) must consume its own
// Escape first — cancel-in-place, no close — so a second, "clean" Escape is
// needed to actually leave the card. This is the EditScope contract
// (shared/editScope.ts) exercised end-to-end through the real component tree.
describe('CardDetail — Escape layering (EditScope)', () => {
  const testCard = (overrides: Partial<Card> = {}): Card => ({
    id: 'card-1',
    title: 'Original Title',
    description: '',
    type: '',
    tags: [],
    due_date: null,
    created_at: '2026-06-01T00:00:00Z',
    blocks: [],
    file_attachments: [],
    ...overrides,
  })

  let adapter: ReturnType<typeof createMockAdapter>

  beforeEach(() => {
    adapter = createMockAdapter()
    adapter.GetCard = vi.fn(async () => testCard())
    adapter.UpdateCardTitle = vi.fn(async (_id: string, title: string) => testCard({ title }))
    setBackend(adapter)
    cardTypes.list = []
  })

  it('window-level Escape with no active edit closes via onClose({ escaped: true })', async () => {
    const onClose = vi.fn()
    render(CardDetail, { props: { cardId: 'card-1', onClose } })
    await screen.findByText('Original Title')

    await fireEvent.keyDown(window, { key: 'Escape' })

    expect(onClose).toHaveBeenCalledWith({ escaped: true })
  })

  it('Escape on the title input cancels the edit without closing; a second window Escape then closes', async () => {
    const onClose = vi.fn()
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose } })
    await screen.findByText('Original Title')

    await fireEvent.click(container.querySelector('.modal-title') as HTMLElement)
    const input = await waitFor(() => {
      const el = container.querySelector('input.title-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    await fireEvent.input(input, { target: { value: 'Discarded edit' } })

    await fireEvent.keyDown(input, { key: 'Escape' })

    // The title edit is cancelled in place (input reverts to display mode)...
    await waitFor(() => {
      expect(container.querySelector('input.title-input')).toBeNull()
    })
    expect(screen.getByText('Original Title')).toBeInTheDocument()
    // ...and the dialog itself must NOT have closed, nor the draft persisted.
    expect(onClose).not.toHaveBeenCalled()
    expect(adapter.UpdateCardTitle).not.toHaveBeenCalled()

    // A second, window-level Escape — nothing left registered — closes it.
    await fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledWith({ escaped: true })
  })
})

// Pre-release data-loss sweep (2026-09-29): overlay Escape layering, the
// Ctrl+Enter close after a failed save, the silent-reload title re-open,
// and the block-save lost update.
describe('CardDetail — data-loss guards', () => {
  const testCard = (overrides: Partial<Card> = {}): Card => ({
    id: 'card-1',
    title: 'Original Title',
    description: '',
    type: '',
    tags: [],
    due_date: null,
    created_at: '2026-06-01T00:00:00Z',
    blocks: [],
    file_attachments: [],
    ...overrides,
  })

  let adapter: ReturnType<typeof createMockAdapter>
  let subscribers: EventCallback[]
  const emit = (ev: BackendEvent) => { for (const cb of [...subscribers]) cb(ev) }

  beforeEach(() => {
    adapter = createMockAdapter()
    subscribers = []
    adapter.subscribe = vi.fn((cb: EventCallback) => { subscribers.push(cb) })
    adapter.unsubscribe = vi.fn((cb: EventCallback) => { subscribers = subscribers.filter(s => s !== cb) })
    adapter.GetCard = vi.fn(async () => testCard())
    adapter.UpdateCardTitle = vi.fn(async (_id: string, title: string) => testCard({ title }))
    setBackend(adapter)
    cardTypes.list = []
    confirmState.visible = false
    confirmState.resolve = null
  })

  it('Escape answering a confirm raised over the card does not close the card', async () => {
    const onClose = vi.fn()
    render(CardDetail, { props: { cardId: 'card-1', onClose } })
    render(ConfirmDialog)
    await screen.findByText('Original Title')

    const answer = showConfirm('sure?')
    await Promise.resolve()
    await fireEvent.keyDown(document.body, { key: 'Escape' })

    await expect(answer).resolves.toBe(false)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('Ctrl+Enter on the title keeps the card open with the draft when the save fails', async () => {
    adapter.UpdateCardTitle = vi.fn(async () => { throw new Error('offline') })
    const onClose = vi.fn()
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose } })
    await screen.findByText('Original Title')

    await fireEvent.click(container.querySelector('.modal-title') as HTMLElement)
    const input = await waitFor(() => {
      const el = container.querySelector('input.title-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    await fireEvent.input(input, { target: { value: 'Unsaved rename' } })
    await fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true })

    await waitFor(() => expect(adapter.UpdateCardTitle).toHaveBeenCalled())
    await new Promise(r => setTimeout(r, 0))
    expect(onClose).not.toHaveBeenCalled()
    expect((container.querySelector('input.title-input') as HTMLInputElement).value).toBe('Unsaved rename')
  })

  it('a silent reload after a cancelled auto title edit does not re-open the editor', async () => {
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose: () => {}, autoEditTitle: true } })
    const input = await waitFor(() => {
      const el = container.querySelector('input.title-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })
    await fireEvent.keyDown(input, { key: 'Escape' })
    await waitFor(() => expect(container.querySelector('input.title-input')).toBeNull())

    emit({ type: 'card:updated', cardID: 'card-1' } as BackendEvent)
    await waitFor(() => expect(adapter.GetCard).toHaveBeenCalledTimes(2))
    await new Promise(r => setTimeout(r, 0))
    expect(container.querySelector('input.title-input')).toBeNull()
  })

  it('serializes block saves and defers a silent reload until they drain', async () => {
    adapter.GetCard = vi.fn(async () => testCard({
      blocks: [{ id: 'blk-1', type: 'checkbox', key: '', label: 'Done', value: false }],
    }))
    const pending: { blocks: Block[]; resolve: () => void }[] = []
    adapter.UpdateCardBlocks = vi.fn((_id: string, blocks: Block[]) =>
      new Promise<Card>((resolve) => { pending.push({ blocks, resolve: () => resolve(testCard({ blocks })) }) }))
    const { container } = render(CardDetail, { props: { cardId: 'card-1', onClose: () => {} } })
    await screen.findByText('Original Title')
    const toggle = await waitFor(() => {
      const el = container.querySelector('input.toggle-input') as HTMLInputElement
      expect(el).toBeTruthy()
      return el
    })

    await fireEvent.change(toggle) // → true, save #1 in flight
    emit({ type: 'card:updated', cardID: 'card-1' } as BackendEvent) // e.g. the watcher echo
    await fireEvent.change(toggle) // → false, queued behind #1
    expect(adapter.UpdateCardBlocks).toHaveBeenCalledTimes(1)
    expect(adapter.GetCard).toHaveBeenCalledTimes(1) // reload deferred

    pending[0].resolve()
    await waitFor(() => expect(adapter.UpdateCardBlocks).toHaveBeenCalledTimes(2))
    expect(pending.map(p => p.blocks[0].value)).toEqual([true, false])
    expect(adapter.GetCard).toHaveBeenCalledTimes(1)

    pending[1].resolve()
    await waitFor(() => expect(adapter.GetCard).toHaveBeenCalledTimes(2))
  })
})
