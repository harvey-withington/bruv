import { describe, it, expect } from 'vitest'
import {
  importCardFromJson,
  mergeCardFromJson,
  ImportError,
  type CardTransferApi,
  type TypeConflictResolution,
} from '@shared/cardTransfer'
import { collectAttachmentRefs, remapAttachmentRefs } from '@shared/attachmentRefs'
import { CARD_JSON_FORMAT, CARD_JSON_VERSION } from '@shared/cardJson'
import type { Card } from '@shared/types'

// Replay-order tests for the import flow: a fake CardTransferApi records
// every call so the pre-flight → create → pin → type/blocks ordering
// (which encodes the verified ApplyTypeBlocks merge semantics) can't
// silently regress.

type Call = { method: string; args: unknown[] }

const KNOWN_TYPES = ['brainstorm', 'task', 'reference', 'agent', 'feature', 'episode']

function makeApi(overrides: {
  acceptedTypes?: string[] | null
  knownTypes?: string[]
  pinFails?: boolean
} = {}): { api: CardTransferApi; calls: Call[] } {
  const calls: Call[] = []
  const rec = (method: string, ...args: unknown[]) => { calls.push({ method, args }) }
  const api: CardTransferApi = {
    getCard: async (cardId) => {
      rec('getCard', cardId)
      return { id: cardId, title: 'Target', type: '', description: '', tags: [], due_date: null, created_at: '', blocks: [], file_attachments: [] } as unknown as Card
    },
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
    // Read-only lookup, deliberately not recorded: the ordering
    // assertions below cover mutations and the category pre-flight.
    listCardTypeIds: async () => overrides.knownTypes ?? KNOWN_TYPES,
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
  it('replays create → pin → attachments → blocks → tags → due → comments', async () => {
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
      'addCardAttachment',
      'updateCardDescription',
      'updateCardBlocks',
      'updateCardTags',
      'updateCardDueDate',
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
      resolveTypeConflict: async (cardType, categoryName, acceptedTypes, reason) => {
        seen = [cardType, categoryName, acceptedTypes, reason]
        return null
      },
    })
    expect(seen).toEqual(['episode', 'Backlog', ['task', 'feature'], 'not_accepted'])
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

describe('importCardFromJson — type missing from this repo', () => {
  it('prompts with every existing type in an unrestricted category', async () => {
    const { api, calls } = makeApi({ acceptedTypes: null, knownTypes: ['brainstorm', 'task'] })
    let seen: unknown[] = []
    const result = await importCardFromJson(api, envelope({ type: 'idea' }), 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async (cardType, _cat, acceptedTypes, reason) => {
        seen = [cardType, acceptedTypes, reason]
        return null
      },
    })
    expect(seen).toEqual(['idea', ['brainstorm', 'task'], 'unknown'])
    expect(result).toBeNull()
    expect(methods(calls)).not.toContain('createCard')
  })

  it('offers only accepted types that exist in a restricted category', async () => {
    const { api } = makeApi({ acceptedTypes: ['task', 'ghost'], knownTypes: ['brainstorm', 'task'] })
    let offered: string[] = []
    await importCardFromJson(api, envelope({ type: 'idea' }), 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async (_t, _c, acceptedTypes) => { offered = acceptedTypes; return null },
    })
    expect(offered).toEqual(['task'])
  })

  it('never creates or retypes the card with the unknown type', async () => {
    const { api, calls } = makeApi({ knownTypes: ['brainstorm'] })
    await importCardFromJson(api, envelope({ type: 'idea' }), 'cat-1', {
      fallbackTitle: 'F',
      resolveTypeConflict: async () => ({ type: '', merge: false }),
    })
    const typesUsed = calls
      .filter(c => c.method === 'createCard' || c.method === 'updateCardType')
      .map(c => (c.method === 'createCard' ? c.args[0] : c.args[1]))
    expect(typesUsed).not.toContain('idea')
  })

  it('aborts with nothing created when the surface has no conflict handler', async () => {
    const { api, calls } = makeApi({ knownTypes: ['brainstorm'] })
    const result = await importCardFromJson(api, envelope({ type: 'idea' }), 'cat-1', { fallbackTitle: 'F' })
    expect(result).toBeNull()
    expect(methods(calls)).not.toContain('createCard')
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

// --- Attachment refs re-homed onto the copies (sweep 9.7) ----------------
//
// Import/merge re-upload attachments and the server mints NEW ids; block
// values and slide decks must follow, or media breaks cross-vault and when
// the source card is deleted.

describe('remapAttachmentRefs', () => {
  const remap = {
    attachments: new Map([['att-a', 'att-A'], ['att-b', 'att-B']]),
    cards: new Map([['src', 'dst']]),
    targetCardID: 'dst',
  }

  it('rewrites {url}, media items, gallery strings and slide links, and leaves the rest alone', () => {
    const value = {
      single: { url: 'attachment:src/att-a' },
      items: [{ id: 'm1', url: 'attachment:src/att-b', mime: 'video/mp4' }],
      gallery: 'attachment:src/att-a\nhttps://cdn.example/x.jpg\nattachment:src/att-b',
      foreign: 'attachment:other/att-z',
      plain: 'https://example.com',
      slides: [{ cardId: 'src', values: { image: 'attachment:src/att-a' } }, { cardId: 'elsewhere' }],
    }
    expect(remapAttachmentRefs(value, remap)).toEqual({
      single: { url: 'attachment:dst/att-A' },
      items: [{ id: 'm1', url: 'attachment:dst/att-B', mime: 'video/mp4' }],
      gallery: 'attachment:dst/att-A\nhttps://cdn.example/x.jpg\nattachment:dst/att-B',
      foreign: 'attachment:other/att-z',
      plain: 'https://example.com',
      slides: [{ cardId: 'dst', values: { image: 'attachment:dst/att-A' } }, { cardId: 'elsewhere' }],
    })
    // Pure: the input is untouched.
    expect(value.single.url).toBe('attachment:src/att-a')
  })

  it('collects refs from every shape', () => {
    const refs = collectAttachmentRefs([{ url: 'attachment:c/a1' }, 'attachment:c/a2\nattachment:d/a3', 'nope'])
    expect(refs.map((r) => `${r.cardID}/${r.attachmentID}`)).toEqual(['c/a1', 'c/a2', 'd/a3'])
  })
})

describe('import / merge re-home attachment refs', () => {
  // Fake backend that mints attachment ids like the server does and hands
  // back the updated card from AddCardAttachment.
  function makeAttApi(target?: Card): { api: CardTransferApi; blocksWritten: () => Card['blocks'] } {
    let written: Card['blocks'] = []
    let seq = 0
    const atts: Array<{ id: string; name: string; path: string; mime: string; size: number; added_at: string }> =
      [...(target?.file_attachments ?? [])]
    const cardId = target?.id ?? 'new-1'
    const snapshot = () => ({ id: cardId, file_attachments: [...atts] }) as unknown as Card
    const api: CardTransferApi = {
      getCard: async () => ({ ...(target ?? {}), ...snapshot() }) as Card,
      createCard: async (type, title) => ({ id: 'new-1', title, type, blocks: [], file_attachments: [] }) as unknown as Card,
      deleteCard: async () => {},
      pinCard: async () => {},
      getCategoryAcceptedTypes: async () => null,
      listCardTypeIds: async () => [],
      updateCardType: async () => {},
      updateCardDescription: async () => {},
      updateCardBlocks: async (_id, blocks) => { written = blocks },
      updateCardTags: async () => {},
      updateCardDueDate: async () => {},
      addCardAttachment: async (_id, name) => {
        atts.push({ id: `att-new-${++seq}`, name, path: '', mime: '', size: 1, added_at: '' })
        return snapshot()
      },
      addCardComment: async () => {},
      listCardComments: async () => [],
      signAttachmentURL: async () => '',
    }
    return { api, blocksWritten: () => written }
  }

  const blocks = [
    { id: 'b1', type: 'image', label: 'Media', key: 'media', value: { url: 'attachment:src-card/att-old-1' } },
    { id: 'b2', type: 'media', label: 'Video', key: 'video', value: [{ id: 'm1', url: 'attachment:src-card/att-old-2' }] },
    {
      id: 'b3', type: 'slide_deck', label: 'Slides', key: '',
      value: { slides: [{ id: 's1', cardId: 'src-card', values: { image: 'attachment:src-card/att-old-1\nattachment:src-card/att-old-2' } }] },
    },
  ]
  const attachments = [
    { id: 'att-old-1', name: 'a.jpg', mime: 'image/jpeg', size: 1, data: 'AA==' },
    { id: 'att-old-2', name: 'b.mp4', mime: 'video/mp4', size: 2, data: 'AAA=' },
  ]

  it('import writes blocks that point at the new card\'s copies', async () => {
    const { api, blocksWritten } = makeAttApi()
    await importCardFromJson(api, envelope({ blocks }, { attachments }), 'cat-1', { fallbackTitle: 'F' })
    const out = JSON.stringify(blocksWritten())
    expect(out).not.toContain('src-card')
    expect(out).not.toContain('att-old')
    expect(blocksWritten()[0].value).toEqual({ url: 'attachment:new-1/att-new-1' })
    expect(blocksWritten()[1].value).toEqual([{ id: 'm1', url: 'attachment:new-1/att-new-2' }])
    expect(blocksWritten()[2].value).toEqual({
      slides: [{ id: 's1', cardId: 'new-1', values: { image: 'attachment:new-1/att-new-1\nattachment:new-1/att-new-2' } }],
    })
  })

  it('import leaves refs alone when the export predates attachment ids', async () => {
    const { api, blocksWritten } = makeAttApi()
    const legacy = attachments.map((a) => ({ name: a.name, mime: a.mime, size: a.size, data: a.data }))
    await importCardFromJson(api, envelope({ blocks: blocks.slice(0, 1) }, { attachments: legacy }), 'cat-1', { fallbackTitle: 'F' })
    expect(blocksWritten()[0].value).toEqual({ url: 'attachment:src-card/att-old-1' })
  })

  it('merge maps refs onto new copies AND onto existing name+size matches', async () => {
    const target = {
      id: 'target', title: 'T', type: '', description: '', tags: [], due_date: null, created_at: '', blocks: [],
      file_attachments: [{ id: 'att-existing', name: 'A.JPG', path: '', mime: 'image/jpeg', size: 1, added_at: '' }],
    } as unknown as Card
    const { api, blocksWritten } = makeAttApi(target)
    const out = await mergeCardFromJson(api, envelope({ blocks: blocks.slice(0, 2) }, { attachments }), 'target', {
      mergedSuffix: '(Merged)', mergedHeading: 'Merged',
    })
    expect(out.attachmentsAdded).toBe(1)
    expect(blocksWritten().map((b) => b.value)).toEqual([
      { url: 'attachment:target/att-existing' },
      [{ id: 'm1', url: 'attachment:target/att-new-1' }],
    ])
  })
})
