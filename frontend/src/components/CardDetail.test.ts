import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent, screen, waitFor } from '@testing-library/svelte'
import CardDetail from './CardDetail.svelte'
import { cardTypes } from '../lib/store.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'
import type { Card } from '@shared/types'

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
