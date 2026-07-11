import { describe, it, expect } from 'vitest'
import {
  importCardFromJson,
  ImportError,
  type CardTransferApi,
  type TypeConflictResolution,
} from '@shared/cardTransfer'
import { CARD_JSON_FORMAT, CARD_JSON_VERSION } from '@shared/cardJson'
import type { Card } from '@shared/types'

// Replay-order tests for the import flow: a fake CardTransferApi records
// every call so the pre-flight → create → pin → type/blocks ordering
// (which encodes the verified ApplyTypeBlocks merge semantics) can't
// silently regress.

type Call = { method: string; args: unknown[] }

function makeApi(overrides: {
  acceptedTypes?: string[] | null
  pinFails?: boolean
} = {}): { api: CardTransferApi; calls: Call[] } {
  const calls: Call[] = []
  const rec = (method: string, ...args: unknown[]) => { calls.push({ method, args }) }
  const api: CardTransferApi = {
    createCard: async (cardType, title) => {
      rec('createCard', cardType, title)
      return { id: 'new-1', title, type: cardType, description: '', tags: [], due_date: null, created_at: '', blocks: [] } as unknown as Card
    },
    deleteCard: async (cardId) => { rec('deleteCard', cardId) },
    pinCard: async (cardId, categoryId) => {
      rec('pinCard', cardId, categoryId)
      if (overrides.pinFails) throw new Error('boom')
    },
    getCategoryAcceptedTypes: async (categoryId) => {
      rec('getCategoryAcceptedTypes', categoryId)
      return overrides.acceptedTypes ?? null
    },
    updateCardType: async (cardId, cardType) => { rec('updateCardType', cardId, cardType) },
    updateCardDescription: async (cardId, description) => { rec('updateCardDescription', cardId, description) },
    updateCardBlocks: async (cardId, blocks) => { rec('updateCardBlocks', cardId, blocks) },
    updateCardTags: async (cardId, tags) => { rec('updateCardTags', cardId, tags) },
    updateCardDueDate: async (cardId, dueDate) => { rec('updateCardDueDate', cardId, dueDate) },
    addCardAttachment: async (cardId, name) => { rec('addCardAttachment', cardId, name) },
    addCardComment: async (cardId, author, text) => { rec('addCardComment', cardId, author, text) },
    listCardComments: async () => [],
    signAttachmentURL: async () => '',
  }
  return { api, calls }
}

function envelope(card: Record<string, unknown> = {}, extra: Record<string, unknown> = {}): string {
  return JSON.stringify({
    format: CARD_JSON_FORMAT,
    version: CARD_JSON_VERSION,
    exported_at: '2026-07-10T00:00:00Z',
    card: { title: 'Hello', type: '', description: '', tags: [], due_date: null, blocks: [], ...card },
    attachments: [],
    comments: [],
    ...extra,
  })
}

const methods = (calls: Call[]) => calls.map(c => c.method)

describe('importCardFromJson — parse validation', () => {
  it('rejects a NaN version', async () => {
    const { api } = makeApi()
    await expect(importCardFromJson(api, envelope({}, { version: NaN }), 'cat-1', { fallbackTitle: 'F' }))
      .rejects.toMatchObject({ code: 'unsupported_version' })
  })

  it('rejects an unknown block type before any RPC', async () => {
    const { api, calls } = makeApi()
    await expect(importCardFromJson(
      api,
      envelope({ blocks: [{ id: 'b1', type: 'wormhole', label: '', key: '', value: null }] }),
      'cat-1',
      { fallbackTitle: 'F' },
    )).rejects.toMatchObject({ code: 'invalid_blocks' })
    expect(calls).toHaveLength(0)
  })
})

describe('importCardFromJson — common path (type accepted or unrestricted)', () => {
  it('replays create → pin → blocks → tags → due → attachments → comments', async () => {
    const { api, calls } = makeApi({ acceptedTypes: null })
    const result = await importCardFromJson(api, envelope({
      title: 'T', type: 'task', description: 'D', tags: ['a'], due_date: '2026-08-01',
      blocks: [{ id: 'b1', type: 'text', label: 'N', key: '', value: 'v' }],
    }, {
      attachments: [{ name: 'a.png', mime: 'image/png', size: 1, data: 'AA==' }],
      comments: [{ author: 'Alice', created_at: '2026-06-01T00:00:00Z', text: 'hi' }],
    }), 'cat-1', { fallbackTitle: 'F' })

    expect(result?.cardId).toBe('new-1')
    expect(methods(calls)).toEqual([
      'getCategoryAcceptedTypes',
      'createCard',
      'pinCard',
      'updateCardDescription',
      'updateCardBlocks',
      'updateCardTags',
      'updateCardDueDate',
      'addCardAttachment',
      'addCardComment',
    ])
    expect(calls[1].args).toEqual(['task', 'T'])
    expect(calls[2].args).toEqual(['new-1', 'cat-1'])
  })

  it('does not prompt when the type is in the accepted list', async () => {
    const { api, calls } = makeApi({ acceptedTypes: ['task', 'feature'] })
    let prompted = false
    const result = await importCardFromJson(api, envelope({ type: 'task' }), 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => { prompted = true; return null },
    })
    expect(prompted).toBe(false)
    expect(result).not.toBeNull()
    expect(methods(calls)).toContain('pinCard')
  })

  it('does not prompt for a typeless card even in a restricted category (Pin accepts typeless)', async () => {
    const { api } = makeApi({ acceptedTypes: ['task'] })
    let prompted = false
    const result = await importCardFromJson(api, envelope({ type: '' }), 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => { prompted = true; return null },
    })
    expect(prompted).toBe(false)
    expect(result).not.toBeNull()
  })

  it('always sends the blocks array, even when the export has none (template-leak fix)', async () => {
    const { api, calls } = makeApi()
    await importCardFromJson(api, envelope({ type: 'task', blocks: [] }), 'cat-1', { fallbackTitle: 'F' })
    const blocksCall = calls.find(c => c.method === 'updateCardBlocks')
    expect(blocksCall).toBeDefined()
    expect(blocksCall!.args[1]).toEqual([])
  })

  it('uses the fallback title when the export title is blank', async () => {
    const { api, calls } = makeApi()
    await importCardFromJson(api, envelope({ title: '   ' }), 'cat-1', { fallbackTitle: 'Imported!' })
    expect(calls.find(c => c.method === 'createCard')!.args[1]).toBe('Imported!')
  })
})

describe('importCardFromJson — type conflict', () => {
  const conflictEnvelope = envelope({
    type: 'episode',
    blocks: [{ id: 'b1', type: 'text', label: 'N', key: 'notes', value: 'v' }],
  })

  it('passes the conflict details to the hook', async () => {
    const { api } = makeApi({ acceptedTypes: ['task', 'feature'] })
    let seen: unknown[] = []
    await importCardFromJson(api, conflictEnvelope, 'cat-1', {
      fallbackTitle: 'F',
      categoryName: 'Backlog',
      resolveTypeConflict: async (cardType, categoryName, acceptedTypes) => {
        seen = [cardType, categoryName, acceptedTypes]
        return null
      },
    })
    expect(seen).toEqual(['episode', 'Backlog', ['task', 'feature']])
  })

  it('cancel creates NOTHING and resolves null', async () => {
    const { api, calls } = makeApi({ acceptedTypes: ['task'] })
    const result = await importCardFromJson(api, conflictEnvelope, 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => null,
    })
    expect(result).toBeNull()
    expect(methods(calls)).toEqual(['getCategoryAcceptedTypes'])
  })

  it('merge path: creates typeless, pins, writes imported blocks, THEN sets the type', async () => {
    const { api, calls } = makeApi({ acceptedTypes: ['task'] })
    const resolution: TypeConflictResolution = { type: 'task', merge: true }
    await importCardFromJson(api, conflictEnvelope, 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => resolution,
    })
    expect(calls.find(c => c.method === 'createCard')!.args[0]).toBe('')
    expect(methods(calls)).toEqual([
      'getCategoryAcceptedTypes', 'createCard', 'pinCard', 'updateCardBlocks', 'updateCardType',
    ])
    expect(calls.find(c => c.method === 'updateCardType')!.args[1]).toBe('task')
  })

  it('keep-exactly path: creates typeless, pins, sets the type, THEN overwrites with imported blocks', async () => {
    const { api, calls } = makeApi({ acceptedTypes: ['task'] })
    await importCardFromJson(api, conflictEnvelope, 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => ({ type: 'task', merge: false }),
    })
    expect(methods(calls)).toEqual([
      'getCategoryAcceptedTypes', 'createCard', 'pinCard', 'updateCardType', 'updateCardBlocks',
    ])
  })

  it('"no type" choice never calls updateCardType', async () => {
    const { api, calls } = makeApi({ acceptedTypes: ['task'] })
    await importCardFromJson(api, conflictEnvelope, 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => ({ type: '', merge: false }),
    })
    expect(methods(calls)).not.toContain('updateCardType')
    expect(methods(calls)).toContain('updateCardBlocks')
  })
})

describe('importCardFromJson — pin failure', () => {
  it('cleans up the bare card and throws pin_failed', async () => {
    const { api, calls } = makeApi({ pinFails: true })
    await expect(importCardFromJson(api, envelope({ type: 'task' }), 'cat-1', { fallbackTitle: 'F' }))
      .rejects.toSatisfy((e: unknown) => e instanceof ImportError && e.code === 'pin_failed')
    expect(methods(calls)).toEqual(['getCategoryAcceptedTypes', 'createCard', 'pinCard', 'deleteCard'])
  })
})
