import { describe, it, expect } from 'vitest'
import { planCardMerge, isMergeNoop, type MergeOptions } from '@shared/cardMerge'
import { mergeCardFromJson, type CardTransferApi } from '@shared/cardTransfer'
import { CARD_JSON_FORMAT, CARD_JSON_VERSION, type ExportedCard } from '@shared/cardJson'
import type { Block, Card } from '@shared/types'

// Rules under test (Harvey, 2026-09-06): non-destructive merge of an
// exported card into an existing one. Single-value blocks with the same
// name are kept on both sides with a "(Merged)" suffix; multi-item blocks
// append only novel items; identical values are skipped so a repeat
// merge is a no-op.

let seq = 0
const opts: MergeOptions = {
  mergedSuffix: '(Merged)',
  mergedHeading: 'Merged from imported card',
  newId: (prefix) => `${prefix}-${++seq}`,
}

function block(partial: Partial<Block> & Pick<Block, 'type'>): Block {
  return { id: partial.id ?? `blk-t${++seq}`, label: '', key: '', value: null, ...partial }
}

function card(partial: Partial<Card> = {}): Card {
  return {
    id: 'target', title: 'Target', type: '', description: '', tags: [], due_date: null,
    created_at: '', blocks: [], file_attachments: [], ...partial,
  } as Card
}

function exported(partial: Partial<ExportedCard> = {}): ExportedCard {
  return { title: 'Source', description: '', type: '', tags: [], due_date: null, blocks: [], ...partial }
}

describe('planCardMerge — single-value blocks', () => {
  it('keeps both sides: appends a suffixed copy without the key and with a fresh id', () => {
    const target = card({ blocks: [block({ id: 'b1', type: 'text', label: 'Notes', key: 'notes', value: 'ours' })] })
    const source = exported({ blocks: [block({ id: 'b1', type: 'text', label: 'Notes', key: 'notes', value: 'theirs' })] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toHaveLength(2)
    expect(plan.blocks![0].value).toBe('ours')
    const copy = plan.blocks![1]
    expect(copy.label).toBe('Notes (Merged)')
    expect(copy.key).toBe('')
    expect(copy.id).not.toBe('b1')
    expect(copy.value).toBe('theirs')
    expect(plan.summary.blocksCopied).toBe(1)
  })

  it('skips identical and empty source values, so a repeat merge is a no-op', () => {
    const target = card({ blocks: [block({ type: 'text', label: 'Notes', value: 'same ' })] })
    const source = exported({ blocks: [
      block({ type: 'text', label: 'Notes', value: 'same' }),
      block({ type: 'text', label: 'Empty one', value: '   ' }),
    ] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toBeNull()
    expect(isMergeNoop(plan.summary)).toBe(true)
  })

  it('matches labels case-insensitively and treats a type mismatch as single-value', () => {
    const target = card({ blocks: [block({ type: 'text', label: 'Cast', value: 'prose' })] })
    const source = exported({ blocks: [block({ type: 'list', label: 'cast', value: [{ id: 'x', text: 'Alice' }] })] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toHaveLength(2)
    expect(plan.blocks![1].type).toBe('list')
    expect(plan.blocks![1].label).toBe('cast (Merged)')
    expect(plan.summary.blocksCopied).toBe(1)
    expect(plan.summary.blocksMerged).toBe(0)
  })
})

describe('planCardMerge — multi-item blocks', () => {
  it('appends novel checklist items with fresh ids and keeps the target done state', () => {
    const target = card({ blocks: [block({ id: 'b1', type: 'checklist', label: 'Traits', value: [
      { id: 'ck-a', text: 'Brave', done: true },
    ] })] })
    const source = exported({ blocks: [block({ type: 'checklist', label: 'Traits', value: [
      { id: 'ck-a', text: 'brave', done: false },   // identical after normalisation
      { id: 'ck-b', text: ' Loyal  ', done: true },
      { id: 'ck-c', text: '', done: false },        // empty, skipped
    ] })] })
    const plan = planCardMerge(target, source, opts)
    const items = plan.blocks![0].value as { id: string; text: string; done: boolean }[]
    expect(items).toHaveLength(2)
    expect(items[0]).toEqual({ id: 'ck-a', text: 'Brave', done: true })
    expect(items[1].text).toBe(' Loyal  ')
    expect(items[1].id).toMatch(/^ck-/)
    expect(items[1].id).not.toBe('ck-b')
    expect(plan.summary).toMatchObject({ blocksMerged: 1, itemsAdded: 1, blocksCopied: 0 })
  })

  it('matches by key before label', () => {
    const target = card({ blocks: [
      block({ type: 'list', label: 'Old label', key: 'goals', value: [{ id: 'li-1', text: 'Win' }] }),
    ] })
    const source = exported({ blocks: [
      block({ type: 'list', label: 'Goals', key: 'goals', value: [{ id: 'li-9', text: 'Survive' }] }),
    ] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toHaveLength(1)
    expect((plan.blocks![0].value as { text: string }[]).map((i) => i.text)).toEqual(['Win', 'Survive'])
  })

  it('unions checkbox_group options and selections', () => {
    const target = card({ blocks: [block({ type: 'checkbox_group', label: 'Genres', value: ['Drama'], meta: { options: ['Drama', 'Comedy'] } })] })
    const source = exported({ blocks: [block({ type: 'checkbox_group', label: 'Genres', value: ['Comedy', 'Horror'], meta: { options: ['Horror'] } })] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks![0].meta?.options).toEqual(['Drama', 'Comedy', 'Horror'])
    expect(plan.blocks![0].value).toEqual(['Drama', 'Comedy', 'Horror'])
    expect(plan.summary.itemsAdded).toBe(1)
  })

  it('a multi-item source with nothing new leaves the block untouched', () => {
    const target = card({ blocks: [block({ type: 'list', label: 'L', value: [{ id: 'li-1', text: 'A' }] })] })
    const source = exported({ blocks: [block({ type: 'list', label: 'L', value: [{ id: 'li-2', text: 'a' }] })] })
    expect(planCardMerge(target, source, opts).blocks).toBeNull()
  })
})

describe('planCardMerge — unmatched blocks and card-level fields', () => {
  it('appends unmatched blocks with content, keeps their keys, and skips empties', () => {
    const target = card({ blocks: [block({ type: 'text', label: 'Summary', key: 'summary', value: 'x' })] })
    const source = exported({ blocks: [
      block({ type: 'date', label: 'Deadline', key: 'deadline', value: '2026-10-01' }),
      block({ type: 'text', label: 'Blank field', key: 'blank', value: '' }),   // empty → skipped
      block({ type: 'divider', label: '', key: '', value: null }),              // empty → skipped
    ] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toHaveLength(2)
    expect(plan.blocks![1]).toMatchObject({ label: 'Deadline', key: 'deadline' })
    expect(plan.blocks![1].id).not.toBe(source.blocks[0].id)
    expect(plan.summary.blocksAdded).toBe(1)
  })

  it('a key match beats a label mismatch, and a type mismatch under that key falls to the copy rule', () => {
    const target = card({ blocks: [block({ type: 'text', label: 'Summary', key: 'summary', value: 'x' })] })
    const source = exported({ blocks: [block({ type: 'number', label: 'Budget', key: 'summary', value: 5 })] })
    const plan = planCardMerge(target, source, opts)
    expect(plan.blocks).toHaveLength(2)
    expect(plan.blocks![1]).toMatchObject({ label: 'Budget (Merged)', key: '', type: 'number', value: 5 })
    expect(plan.summary).toMatchObject({ blocksCopied: 1, blocksAdded: 0 })
  })

  it('unions tags, fills an empty due date, and appends the description under a heading', () => {
    const target = card({ tags: ['film'], description: 'Ours.', blocks: [] })
    const source = exported({ tags: ['Film', 'noir'], due_date: '2026-12-01T00:00:00Z', description: 'Theirs.' })
    const plan = planCardMerge(target, source, opts)
    expect(plan.tags).toEqual(['film', 'noir'])
    expect(plan.dueDate).toBe('2026-12-01')
    expect(plan.description).toBe('Ours.\n\n---\n\n**Merged from imported card**\n\nTheirs.')
    expect(plan.summary).toMatchObject({ tagsAdded: 1, dueDateSet: true, descriptionChanged: true })
  })

  it('never overwrites: existing due date and an already-merged description are left alone', () => {
    const target = card({ due_date: '2026-01-01', description: 'Ours.\n\n---\n\n**Merged from imported card**\n\nTheirs.' })
    const source = exported({ due_date: '2026-12-01', description: 'Theirs.' })
    const plan = planCardMerge(target, source, opts)
    expect(plan.dueDate).toBeNull()
    expect(plan.description).toBeNull()
  })
})

describe('mergeCardFromJson', () => {
  type Call = { method: string; args: unknown[] }
  function makeApi(target: Card): { api: CardTransferApi; calls: Call[] } {
    const calls: Call[] = []
    const rec = (method: string, ...args: unknown[]) => { calls.push({ method, args }) }
    const api: CardTransferApi = {
      getCard: async (id) => { rec('getCard', id); return target },
      createCard: async () => { throw new Error('merge must not create cards') },
      deleteCard: async () => { throw new Error('merge must not delete cards') },
      pinCard: async () => { throw new Error('merge must not pin cards') },
      getCategoryAcceptedTypes: async () => null,
      updateCardType: async () => { throw new Error('merge must not change the type') },
      updateCardDescription: async (id, d) => { rec('updateCardDescription', id, d) },
      updateCardBlocks: async (id, b) => { rec('updateCardBlocks', id, b) },
      updateCardTags: async (id, tg) => { rec('updateCardTags', id, tg) },
      updateCardDueDate: async (id, d) => { rec('updateCardDueDate', id, d) },
      addCardAttachment: async (id, name) => { rec('addCardAttachment', id, name) },
      addCardComment: async (id, author, text) => { rec('addCardComment', id, author, text) },
      listCardComments: async () => [{ id: 'c1', author: 'Harvey', text: 'existing', created_at: '', updated_at: '' }],
      signAttachmentURL: async () => '',
    }
    return { api, calls }
  }

  function envelope(cardPart: Partial<ExportedCard>, extra: Record<string, unknown> = {}): string {
    return JSON.stringify({
      format: CARD_JSON_FORMAT, version: CARD_JSON_VERSION, exported_at: '2026-09-06T00:00:00Z',
      card: exported(cardPart), attachments: [], comments: [], ...extra,
    })
  }

  it('re-fetches the target, writes only what changed, and dedupes attachments and comments', async () => {
    const target = card({
      blocks: [block({ type: 'text', label: 'Notes', value: 'ours' })],
      file_attachments: [{ id: 'a1', name: 'Spec.md', path: '', mime: 'text/markdown', size: 3, added_at: '' }],
    })
    const { api, calls } = makeApi(target)
    const out = await mergeCardFromJson(api, envelope(
      { blocks: [block({ type: 'text', label: 'Notes', value: 'theirs' })] },
      {
        attachments: [
          { name: 'spec.md', mime: 'text/markdown', size: 3, data: 'YWJj' },   // same name+size → skipped
          { name: 'new.txt', mime: 'text/plain', size: 1, data: 'YQ==' },
        ],
        comments: [
          { author: 'Harvey', created_at: '', text: 'existing ' },            // same author+text → skipped
          { author: 'Bob', created_at: '', text: 'fresh' },
        ],
      },
    ), 'target', { mergedSuffix: '(Merged)', mergedHeading: 'Merged' })

    expect(calls.map((c) => c.method)).toEqual(['getCard', 'updateCardBlocks', 'addCardAttachment', 'addCardComment'])
    expect(calls[2].args[1]).toBe('new.txt')
    expect(calls[3].args).toEqual(['target', 'Bob', 'fresh'])
    expect(out).toMatchObject({ cardId: 'target', attachmentsAdded: 1, commentsAdded: 1, failedAttachments: [], failedComments: [] })
    expect(out.summary.blocksCopied).toBe(1)
  })

  it('a no-op merge performs no writes beyond the fetch', async () => {
    const target = card({ blocks: [block({ type: 'text', label: 'Notes', value: 'same' })] })
    const { api, calls } = makeApi(target)
    const out = await mergeCardFromJson(api, envelope({ blocks: [block({ type: 'text', label: 'Notes', value: 'same' })] }), 'target', {
      mergedSuffix: '(Merged)', mergedHeading: 'Merged',
    })
    expect(calls.map((c) => c.method)).toEqual(['getCard'])
    expect(isMergeNoop(out.summary)).toBe(true)
  })
})
