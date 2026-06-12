import { describe, it, expect } from 'vitest'
import { parseCardImport, CARD_JSON_FORMAT, CARD_JSON_VERSION } from '@shared/cardJson'

function envelope(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    format: CARD_JSON_FORMAT,
    version: CARD_JSON_VERSION,
    exported_at: '2026-06-12T00:00:00Z',
    card: { title: 'Hello' },
    attachments: [],
    comments: [],
    ...overrides,
  })
}

describe('parseCardImport — validation', () => {
  it('accepts a minimal valid envelope and normalises optional fields', () => {
    const result = parseCardImport(envelope())
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.value.card.title).toBe('Hello')
    expect(result.value.card.description).toBe('')
    expect(result.value.card.tags).toEqual([])
    expect(result.value.card.blocks).toEqual([])
    expect(result.value.card.due_date).toBeNull()
  })

  it('rejects non-JSON input', () => {
    expect(parseCardImport('not json {')).toEqual({ ok: false, error: 'not_json' })
  })

  it('rejects a wrong format marker', () => {
    expect(parseCardImport(envelope({ format: 'trello-card' })))
      .toEqual({ ok: false, error: 'wrong_format' })
  })

  it('rejects newer, NaN, fractional, and sub-1 versions', () => {
    for (const version of [CARD_JSON_VERSION + 1, NaN, 1.5, 0, -3]) {
      expect(parseCardImport(envelope({ version })))
        .toEqual({ ok: false, error: 'unsupported_version' })
    }
  })

  it('rejects a missing or non-object card', () => {
    expect(parseCardImport(envelope({ card: undefined })))
      .toEqual({ ok: false, error: 'missing_card' })
    expect(parseCardImport(envelope({ card: 'oops' })))
      .toEqual({ ok: false, error: 'missing_card' })
  })

  it('rejects a card without a string title', () => {
    expect(parseCardImport(envelope({ card: { title: 42 } })))
      .toEqual({ ok: false, error: 'invalid_title' })
  })

  it('drops malformed blocks, attachments, and comments while keeping valid ones', () => {
    const result = parseCardImport(envelope({
      card: {
        title: 'Hello',
        blocks: [
          { id: 'b1', type: 'text', label: '', key: '', value: 'keep' },
          { id: 'b2' },          // no type — dropped
          'garbage',             // not an object — dropped
        ],
      },
      attachments: [
        { name: 'a.png', mime: 'image/png', size: 1, data: 'AA==' },
        { name: 'broken.png' },  // missing fields — dropped
      ],
      comments: [
        { author: 'Alice', created_at: '2026-06-01T00:00:00Z', text: 'hi' },
        { author: 'Bob' },       // missing fields — dropped
      ],
    }))
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.value.card.blocks).toHaveLength(1)
    expect(result.value.attachments).toHaveLength(1)
    expect(result.value.comments).toHaveLength(1)
  })
})
