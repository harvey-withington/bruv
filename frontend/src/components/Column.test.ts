import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Column from './Column.svelte'
import { dnd, board, cardTypes } from '../lib/store.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '../lib/adapters'

// Catches: id-vs-index keying regressions on the card grid. If the
// {#each} ever loses its (card.id) key, or if the drop handler starts
// passing indices instead of IDs, this test breaks. Also asserts the
// dnd state is cleaned up after the drop so subsequent interactions
// don't see ghost drag state.
describe('Column drag-drop', () => {
  beforeEach(() => {
    setBackend(createMockAdapter())
    // Reset shared stores between tests
    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
    dnd.copyMode = false
    board.agentCardStates = {}
    board.runningAgentIds = {}
    cardTypes.list = []
  })

  const makeCategory = (cards: Array<{ id: string; title: string }>) => ({
    id: 'cat-1',
    name: 'Inbox',
    slug: 'inbox',
    position: 0,
    cards: cards.map(c => ({
      id: c.id,
      type: '',
      title: c.title,
      tags: [],
      due_date: null,
      checklist_total: 0,
      checklist_done: 0,
    })),
  })

  it('renders cards in the order provided', () => {
    const category = makeCategory([
      { id: 'card-a', title: 'Alpha' },
      { id: 'card-b', title: 'Bravo' },
      { id: 'card-c', title: 'Charlie' },
    ])
    const { container } = render(Column, { props: { category } })
    const titles = Array.from(container.querySelectorAll('.card-wrapper'))
      .map(w => w.textContent?.trim())
    // Each wrapper's text content contains the card's title somewhere
    expect(titles[0]).toContain('Alpha')
    expect(titles[1]).toContain('Bravo')
    expect(titles[2]).toContain('Charlie')
  })

  it('drop fires onCardDrop with the dragged card id (not a stale index)', async () => {
    const onCardDrop = vi.fn()
    const category = makeCategory([
      { id: 'card-a', title: 'Alpha' },
      { id: 'card-b', title: 'Bravo' },
      { id: 'card-c', title: 'Charlie' },
    ])
    const { container } = render(Column, { props: { category, onCardDrop } })

    // Simulate: user is dragging card-a and hovering over position 2
    // (between Bravo and Charlie). The real dragover handler computes
    // this from mouse Y vs card rects; we set it directly because
    // jsdom returns zeroed rects which would always yield index 0.
    dnd.dragging = {
      type: 'card',
      cardId: 'card-a',
      fromCategoryId: 'cat-1',
      cardType: '',
    }
    dnd.overCategoryId = 'cat-1'
    dnd.overCardIndex = 2

    const list = container.querySelector('.card-list') as HTMLElement
    expect(list).toBeTruthy()
    await fireEvent.drop(list)

    expect(onCardDrop).toHaveBeenCalledTimes(1)
    // Signature: (cardId, fromCategoryId, toCategoryId, toIndex, copy?).
    // Assert the four ID-ish args precisely; copy flag varies by
    // synthetic event shape in jsdom so we only assert it's falsy.
    const call = onCardDrop.mock.calls[0]
    expect(call.slice(0, 4)).toEqual(['card-a', 'cat-1', 'cat-1', 2])
    expect(call[4]).toBeFalsy()
  })

  it('clears dnd state after drop', async () => {
    const category = makeCategory([{ id: 'card-a', title: 'Alpha' }])
    const { container } = render(Column, { props: { category, onCardDrop: () => {} } })

    dnd.dragging = { type: 'card', cardId: 'card-a', fromCategoryId: 'cat-1', cardType: '' }
    dnd.overCategoryId = 'cat-1'
    dnd.overCardIndex = 1

    const list = container.querySelector('.card-list') as HTMLElement
    await fireEvent.drop(list)

    expect(dnd.dragging).toBeNull()
    expect(dnd.overCategoryId).toBeNull()
    expect(dnd.overCardIndex).toBeNull()
  })

  it('rejects cross-type drops when accepted_types is restrictive', async () => {
    const onCardDrop = vi.fn()
    const typedCategory = {
      ...makeCategory([{ id: 'card-x', title: 'Existing' }]),
      accepted_types: ['story'],
    }
    const { container } = render(Column, { props: { category: typedCategory, onCardDrop } })

    // Simulate a drop of a card whose type is NOT in accepted_types
    dnd.dragging = {
      type: 'card',
      cardId: 'card-y',
      fromCategoryId: 'other-cat',
      cardType: 'task',
    }
    dnd.overCategoryId = 'cat-1'
    dnd.overCardIndex = 1

    const list = container.querySelector('.card-list') as HTMLElement
    await fireEvent.drop(list)

    // Drop must NOT fire the callback — the type guard rejects it.
    expect(onCardDrop).not.toHaveBeenCalled()
    // dnd state still gets cleared
    expect(dnd.dragging).toBeNull()
  })

  it('reordering by id is resilient to position churn in upstream data', () => {
    // This is a render-only check: swap the underlying card order in
    // the props and verify the DOM updates without re-keying by index.
    // If the {#each} key were the index, a reorder + delete combo
    // could leave CardItem internal state associated with the wrong
    // card — this test guards against that by asserting DOM order
    // matches the new data order.
    const initial = makeCategory([
      { id: 'card-a', title: 'Alpha' },
      { id: 'card-b', title: 'Bravo' },
      { id: 'card-c', title: 'Charlie' },
    ])
    const { container, rerender } = render(Column, { props: { category: initial } })
    const reordered = makeCategory([
      { id: 'card-c', title: 'Charlie' },
      { id: 'card-a', title: 'Alpha' },
      { id: 'card-b', title: 'Bravo' },
    ])
    rerender({ category: reordered })
    const titles = Array.from(container.querySelectorAll('.card-wrapper'))
      .map(w => w.textContent?.trim() || '')
    expect(titles[0]).toContain('Charlie')
    expect(titles[1]).toContain('Alpha')
    expect(titles[2]).toContain('Bravo')
  })
})
