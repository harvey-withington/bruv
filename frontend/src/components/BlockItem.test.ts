import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import BlockItem from './BlockItem.svelte'
import { createMockAdapter } from '../lib/adapters/mock'
import { setBackend } from '@shared/adapters'
import type { Block, Card } from '@shared/types'

// Catches: the CLAUDE.md hard rule "ID-based state, Not Index-based".
// Every callback BlockItem invokes must carry the block.id, not the
// blockIdx — otherwise reordering or deleting siblings silently
// corrupts editing state. If anyone refactors a callback to pass the
// index, these assertions fire.
describe('BlockItem — ID-keyed callbacks', () => {
  beforeEach(() => {
    setBackend(createMockAdapter())
  })

  const makeTextBlock = (id: string, overrides: Partial<Block> = {}): Block => ({
    id,
    type: 'text',
    key: '',
    label: 'Notes',
    value: 'hello',
    ...overrides,
  } as Block)

  const mountBlock = (block: Block, overrides: Record<string, unknown> = {}) => {
    const callbacks = {
      onDragStart: vi.fn(),
      onDragEnd: vi.fn(),
      onDragOver: vi.fn(),
      onKeydown: vi.fn(),
      onToggleCollapse: vi.fn(),
      onRenameLabel: vi.fn(),
      onOpenOptionsEditor: vi.fn(),
      onClearValue: vi.fn(),
      onDelete: vi.fn(),
      onTextKeydown: vi.fn(),
      onTextInput: vi.fn(),
      onSaveText: vi.fn(),
      onSaveUrl: vi.fn(),
      onToggleTextExpand: vi.fn(),
    }
    const { container, rerender } = render(BlockItem, {
      props: {
        block,
        blockIdx: 7, // Non-zero so index-bugs would surface as stale indices
        card: null as unknown as Card,
        cardId: 'card-1',
        currentCategoryId: 'cat-1',
        editingBlockId: null,
        editingBlockLabelId: null,
        blockLabelDraft: '',
        blockDrafts: {},
        collapsedBlocks: new Set<string>(),
        expandedTextBlocks: new Set<string>(),
        draggingBlockId: null,
        mentionVisible: false,
        textBlockOverflows: new Set<string>(),
        blockTextareaEls: {},
        textBlockEls: {},
        tracked: async <T,>(p: Promise<T>) => p,
        isBlockEmpty: () => false,
        ...callbacks,
        ...overrides,
      },
    })
    return { container, rerender, callbacks }
  }

  it('delete button calls onDelete with block.id', async () => {
    const block = makeTextBlock('blk-xyz')
    const { container, callbacks } = mountBlock(block)
    const deleteBtn = container.querySelector('button[title*="Delete" i]') as HTMLButtonElement
    expect(deleteBtn).toBeTruthy()
    await fireEvent.click(deleteBtn)
    expect(callbacks.onDelete).toHaveBeenCalledWith('blk-xyz')
  })

  it('drag handle calls onDragStart with the block object (carrying id)', async () => {
    const block = makeTextBlock('blk-drag-test')
    const { container, callbacks } = mountBlock(block)
    const handle = container.querySelector('.block-drag-handle') as HTMLElement
    await fireEvent.dragStart(handle)
    expect(callbacks.onDragStart).toHaveBeenCalledOnce()
    const [, passedBlock] = callbacks.onDragStart.mock.calls[0]
    expect(passedBlock.id).toBe('blk-drag-test')
  })

  it('collapse button calls onToggleCollapse with block.id', async () => {
    const block = makeTextBlock('blk-col')
    const { container, callbacks } = mountBlock(block)
    const collapseBtn = container.querySelector('.block-collapse-btn') as HTMLButtonElement
    expect(collapseBtn).toBeTruthy()
    await fireEvent.click(collapseBtn)
    expect(callbacks.onToggleCollapse).toHaveBeenCalledWith('blk-col')
  })

  it('text editing binds textarea value to blockDrafts[block.id], not blockDrafts[blockIdx]', async () => {
    const block = makeTextBlock('blk-editable')
    const blockDrafts: Record<string, string> = { 'blk-editable': 'initial draft' }
    const { container } = mountBlock(block, {
      editingBlockId: 'blk-editable',
      blockDrafts,
    })
    const textarea = container.querySelector('textarea.desc-textarea') as HTMLTextAreaElement
    expect(textarea).toBeTruthy()
    expect(textarea.value).toBe('initial draft')
    // Index-keyed bug would have drafted at drafts[7] and left this empty
    expect(blockDrafts[7 as unknown as string]).toBeUndefined()
  })

  it('text save (blur) invokes onSaveText with block.id', async () => {
    const block = makeTextBlock('blk-save')
    const { container, callbacks } = mountBlock(block, {
      editingBlockId: 'blk-save',
      blockDrafts: { 'blk-save': 'typing' },
    })
    const textarea = container.querySelector('textarea.desc-textarea') as HTMLTextAreaElement
    await fireEvent.blur(textarea)
    expect(callbacks.onSaveText).toHaveBeenCalledWith('blk-save')
  })

  it('data-block-id attribute on the wrapper matches block.id (regression guard for ID-based query selectors)', () => {
    const block = makeTextBlock('blk-dataset')
    const { container } = mountBlock(block)
    const wrapper = container.querySelector('.block-wrapper') as HTMLElement
    expect(wrapper.dataset.blockId).toBe('blk-dataset')
  })

  it('dragging-state visual is triggered by block.id match, not by blockIdx', () => {
    // When the parent's draggingBlockId equals this block.id, the
    // wrapper gets the .block-dragging class. If the logic ever
    // regressed to `draggingBlockIdx === blockIdx`, this breaks.
    const block = makeTextBlock('blk-drag-match')
    const { container } = mountBlock(block, { draggingBlockId: 'blk-drag-match' })
    const wrapper = container.querySelector('.block-wrapper') as HTMLElement
    expect(wrapper.classList.contains('block-dragging')).toBe(true)
  })

  it('two sibling BlockItems with the same blockIdx but different IDs stay isolated', async () => {
    // Contrived but important: two mounts with the SAME blockIdx (7)
    // but different block.ids. Clicking delete on one must only invoke
    // that one's onDelete with its own id. Regression: if any callback
    // used blockIdx as the identifier, we'd get cross-talk.
    const blockA = makeTextBlock('blk-A', { label: 'A' })
    const blockB = makeTextBlock('blk-B', { label: 'B' })
    const mountA = mountBlock(blockA)
    const mountB = mountBlock(blockB)

    const delA = mountA.container.querySelector('button[title*="Delete" i]') as HTMLButtonElement
    const delB = mountB.container.querySelector('button[title*="Delete" i]') as HTMLButtonElement

    await fireEvent.click(delA)
    expect(mountA.callbacks.onDelete).toHaveBeenCalledWith('blk-A')
    expect(mountB.callbacks.onDelete).not.toHaveBeenCalled()

    await fireEvent.click(delB)
    expect(mountB.callbacks.onDelete).toHaveBeenCalledWith('blk-B')
  })
})
