import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import Board from './Board.svelte'
import { dnd, board, nav, cardTypes } from '../lib/store.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'

// Board-level orchestration: one column per category in store order,
// and the cross-column drop wiring — Column's onCardDrop must reach
// Board's handleCardDrop, which routes to MoveCardToCategory (cross-
// column) vs MoveCardInCategory (same-column reorder). Complements
// Column.test.ts, which covers the single-column half.
describe('Board smoke', () => {
  const makeCard = (id: string, title: string, type = '') => ({
    id,
    type,
    title,
    tags: [] as string[],
    due_date: null,
    checklist_total: 0,
    checklist_done: 0,
  })

  const seedBoard = () => {
    board.categories = [
      {
        id: 'cat-1',
        name: 'Todo',
        slug: 'todo',
        position: 0,
        cards: [makeCard('card-a', 'Alpha'), makeCard('card-b', 'Bravo')],
      },
      {
        id: 'cat-2',
        name: 'Done',
        slug: 'done',
        position: 1,
        cards: [makeCard('card-c', 'Charlie')],
      },
    ]
  }

  let adapter: ReturnType<typeof createMockAdapter>

  beforeEach(() => {
    adapter = createMockAdapter()
    adapter.MoveCardToCategory = vi.fn(async () => {})
    adapter.MoveCardInCategory = vi.fn(async () => {})
    setBackend(adapter)

    // Board renders its columns branch only when a project is selected.
    nav.brandSlug = 'test-brand'
    nav.streamSlug = 'test-stream'
    nav.projectSlug = 'test-project'
    board.loading = false
    board.agentCardStates = {}
    board.runningAgentIds = {}
    cardTypes.list = []
    seedBoard()

    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
    dnd.copyMode = false
  })

  it('renders one column per category, in store order', () => {
    const { container } = render(Board)
    const slots = Array.from(container.querySelectorAll('.col-slot'))
    expect(slots.length).toBe(2)
    expect(slots[0].textContent).toContain('Todo')
    expect(slots[1].textContent).toContain('Done')
  })

  it('cross-column drop routes to MoveCardToCategory with ids, not indices', async () => {
    const { container } = render(Board)

    // Simulate dragging card-a (from cat-1) over cat-2 at index 1.
    dnd.dragging = { type: 'card', cardId: 'card-a', fromCategoryId: 'cat-1', cardType: '' }
    dnd.overCategoryId = 'cat-2'
    dnd.overCardIndex = 1

    const lists = container.querySelectorAll('.card-list')
    expect(lists.length).toBe(2)
    await fireEvent.drop(lists[1])

    expect(adapter.MoveCardToCategory).toHaveBeenCalledTimes(1)
    expect(adapter.MoveCardToCategory).toHaveBeenCalledWith('card-a', 'cat-1', 'cat-2', 1)
    // Optimistic update: the card left cat-1 and landed in cat-2.
    expect(board.categories[0].cards.map(c => c.id)).toEqual(['card-b'])
    expect(board.categories[1].cards.map(c => c.id)).toEqual(['card-c', 'card-a'])
  })

  it('same-column drop reorders via MoveCardInCategory only', async () => {
    const { container } = render(Board)

    // Drag card-a to position 2 within cat-1 (after Bravo).
    dnd.dragging = { type: 'card', cardId: 'card-a', fromCategoryId: 'cat-1', cardType: '' }
    dnd.overCategoryId = 'cat-1'
    dnd.overCardIndex = 2

    const lists = container.querySelectorAll('.card-list')
    await fireEvent.drop(lists[0])

    expect(adapter.MoveCardToCategory).not.toHaveBeenCalled()
    // Positions re-persisted for every card in the column.
    const calls = (adapter.MoveCardInCategory as ReturnType<typeof vi.fn>).mock.calls
    expect(calls).toEqual([
      ['card-b', 'cat-1', 0],
      ['card-a', 'cat-1', 1],
    ])
    expect(board.categories[0].cards.map(c => c.id)).toEqual(['card-b', 'card-a'])
  })

  // REGRESSION (2026-08-09): cancelRenameCategory was gated on the draft
  // being unchanged — typing into a fresh-create's name prompt and then
  // pressing Escape left a default-named "New Category" behind. The
  // create-then-rename ruling (UI-CONVENTIONS §12.5) requires Escape on a
  // fresh create to delete the object UNCONDITIONALLY.
  it('Escape on a fresh-created category deletes it even after typing', async () => {
    const cat = (id: string, name: string, slug: string, position: number) => ({
      id, name, slug, position,
      project_id: 'proj-1', created_at: '2026-08-09T00:00:00Z', updated_at: '2026-08-09T00:00:00Z',
    })
    const newCat = cat('cat-new', 'New Category', 'new-category', 2)
    adapter.CreateCategory = vi.fn(async () => newCat)
    adapter.DeleteCategory = vi.fn(async () => {})
    adapter.ListCategories = vi.fn(async () => [
      cat('cat-1', 'Todo', 'todo', 0),
      cat('cat-2', 'Done', 'done', 1),
      newCat,
    ])
    adapter.ListCardIDsInCategory = vi.fn(async () => [])

    const { container, findByDisplayValue } = render(Board)
    await fireEvent.click(container.querySelector('.add-column-btn')!)

    // The fresh category's rename input is auto-armed after the refresh.
    const input = await findByDisplayValue('New Category')
    await fireEvent.input(input, { target: { value: 'Half-typed na' } })
    await fireEvent.keyDown(input, { key: 'Escape' })

    expect(adapter.DeleteCategory).toHaveBeenCalledTimes(1)
    expect(adapter.DeleteCategory).toHaveBeenCalledWith(
      'test-brand', 'test-stream', 'test-project', 'new-category',
    )
  })

  it('Escape on an EXISTING category rename never deletes', async () => {
    adapter.DeleteCategory = vi.fn(async () => {})
    const { container } = render(Board)

    // Clicking a column's title enters rename (Column.onStartRename).
    await fireEvent.click(container.querySelector('.column-title-btn')!)
    const input = container.querySelector('.column-rename-input')
    expect(input).not.toBeNull()
    await fireEvent.input(input!, { target: { value: 'Renamed' } })
    await fireEvent.keyDown(input!, { key: 'Escape' })

    expect(adapter.DeleteCategory).not.toHaveBeenCalled()
  })

  it('drop onto a type-restricted category that rejects the card is a no-op', async () => {
    board.categories[1].accepted_types = ['story']
    board.categories[0].cards[0] = makeCard('card-a', 'Alpha', 'task')
    const { container } = render(Board)

    dnd.dragging = { type: 'card', cardId: 'card-a', fromCategoryId: 'cat-1', cardType: 'task' }
    dnd.overCategoryId = 'cat-2'
    dnd.overCardIndex = 1

    const lists = container.querySelectorAll('.card-list')
    await fireEvent.drop(lists[1])

    expect(adapter.MoveCardToCategory).not.toHaveBeenCalled()
    expect(adapter.MoveCardInCategory).not.toHaveBeenCalled()
    expect(board.categories[0].cards.map(c => c.id)).toEqual(['card-a', 'card-b'])
  })
})
