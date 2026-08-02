import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CapturePrefs } from './capturePrefs'

// seedShareParams exists because of a real share that went wrong
// (2026-08-02): the YouTube Android app shares "Check this out
// https://youtu.be/x" as TEXT with an empty url slot, so the clip
// preflight never saw a URL and a clippable post landed as a bare card.
// Every case below is a way that silent downgrade can come back.

const repoRPC = vi.fn()
vi.mock('./auth', () => ({ repoRPC: (...args: unknown[]) => repoRPC(...args) }))

const { seedShareParams, savePlainShare } = await import('./shareCapture')

beforeEach(() => {
  repoRPC.mockReset()
})

describe('seedShareParams', () => {
  it('lifts an http(s) URL out of the shared text when the url slot is empty', () => {
    // The exact shape of a YouTube app share.
    const out = seedShareParams('', 'Check this out https://youtu.be/dQw4w9WgXcQ', '')
    expect(out.url).toBe('https://youtu.be/dQw4w9WgXcQ')
    expect(out.text).toBe('Check this out')
    expect(out.title).toBe('')
  })

  it('lifts the URL when the url slot holds only whitespace', () => {
    const out = seedShareParams('', 'see https://example.com/a', '   ')
    expect(out.url).toBe('https://example.com/a')
  })

  it('strips trailing prose punctuation from the lifted URL', () => {
    const cases: Array<[string, string]> = [
      ['Look https://example.com/a.', 'https://example.com/a'],
      ['Look https://example.com/a,', 'https://example.com/a'],
      ['Look (https://example.com/a)', 'https://example.com/a'],
      ['Look [https://example.com/a]', 'https://example.com/a'],
      ['Look https://example.com/a!', 'https://example.com/a'],
      ['Look https://example.com/a?', 'https://example.com/a'],
      ["Look 'https://example.com/a'", 'https://example.com/a'],
      ['Look "https://example.com/a"', 'https://example.com/a'],
      ['Look https://example.com/a).', 'https://example.com/a'],
    ]
    for (const [text, expected] of cases) {
      expect(seedShareParams('', text, '').url, text).toBe(expected)
    }
  })

  it('keeps a query string intact while stripping the sentence-ending dot', () => {
    const out = seedShareParams('', 'watch https://youtu.be/p9q?si=7RliB4Qrgrdv.', '')
    expect(out.url).toBe('https://youtu.be/p9q?si=7RliB4Qrgrdv')
  })

  it('leaves everything alone when the url slot is already filled', () => {
    // Chrome promotes a pure-link share to `url`; the text may still
    // mention another link, and we must not second-guess the browser.
    const out = seedShareParams('T', 'and also https://other.example/b', 'https://youtu.be/abc')
    expect(out).toEqual({
      title: 'T',
      text: 'and also https://other.example/b',
      url: 'https://youtu.be/abc',
    })
  })

  it('leaves a text-only share (no URL anywhere) untouched', () => {
    const out = seedShareParams('Note', 'just some thoughts, no links', '')
    expect(out).toEqual({ title: 'Note', text: 'just some thoughts, no links', url: '' })
  })

  it('leaves an empty share untouched', () => {
    expect(seedShareParams('', '', '')).toEqual({ title: '', text: '', url: '' })
  })

  it('ignores non-http schemes (only http/https are clippable)', () => {
    const out = seedShareParams('', 'mail me at mailto:a@b.com or ftp://host/f', '')
    expect(out.url).toBe('')
    expect(out.text).toBe('mail me at mailto:a@b.com or ftp://host/f')
  })

  it('takes the FIRST URL when the text holds several, leaving the rest in the text', () => {
    const out = seedShareParams('', 'first https://a.example/1 then https://b.example/2', '')
    expect(out.url).toBe('https://a.example/1')
    expect(out.text).toBe('first then https://b.example/2')
  })

  it('tidies the whitespace left behind where the URL was', () => {
    const out = seedShareParams('', 'Watch https://a.example/x now', '')
    // Two spaces collapse to one; no leading/trailing space survives.
    expect(out.text).toBe('Watch now')
  })

  it('reduces to an empty text when the share was nothing but the URL', () => {
    const out = seedShareParams('', 'https://a.example/x', '')
    expect(out.url).toBe('https://a.example/x')
    expect(out.text).toBe('')
  })

  it('never touches the title', () => {
    const out = seedShareParams('My Title', 'see https://a.example/x', '')
    expect(out.title).toBe('My Title')
  })
})

// --- savePlainShare -------------------------------------------------
//
// The plain path is the fallback path, so its own fallbacks matter: a
// failed description or deck append must not lose the card.

const prefs = (over: Partial<CapturePrefs> = {}): CapturePrefs => ({
  deckTarget: null,
  categoryID: '',
  categoryName: '',
  includeInDeck: false,
  ...over,
})

const deck = { cardID: 'deck-card', blockID: 'blk-deck', name: 'Reading' }

describe('savePlainShare', () => {
  it('writes the URL and the text into the description, separated', async () => {
    repoRPC.mockResolvedValueOnce({ id: 'card-1' }).mockResolvedValueOnce(undefined)
    const res = await savePlainShare({
      title: 'T',
      text: 'some words',
      url: 'https://a.example/x',
      prefs: prefs(),
    })
    expect(res).toEqual({ cardID: 'card-1', deckFailed: false })
    expect(repoRPC).toHaveBeenCalledWith('CreateCard', ['', 'T'])
    expect(repoRPC).toHaveBeenCalledWith('UpdateCardDescription', [
      'card-1',
      'https://a.example/x\n\nsome words',
    ])
  })

  it('does not repeat the URL when the text IS the URL (Brave share-this-page)', async () => {
    repoRPC.mockResolvedValueOnce({ id: 'card-1' }).mockResolvedValueOnce(undefined)
    await savePlainShare({
      title: 'T',
      text: 'https://a.example/x',
      url: 'https://a.example/x',
      prefs: prefs(),
    })
    expect(repoRPC).toHaveBeenCalledWith('UpdateCardDescription', ['card-1', 'https://a.example/x'])
  })

  it('keeps the URL out of the description when it is bound to a deck slide instead', async () => {
    repoRPC.mockResolvedValue(undefined)
    repoRPC.mockResolvedValueOnce({ id: 'card-1' })
    await savePlainShare({
      title: 'T',
      text: 'some words',
      url: 'https://a.example/x',
      prefs: prefs({ deckTarget: deck, includeInDeck: true }),
    })
    expect(repoRPC).toHaveBeenCalledWith('UpdateCardDescription', ['card-1', 'some words'])
    expect(repoRPC).toHaveBeenCalledWith('AppendDeckSlide', [
      'deck-card',
      'blk-deck',
      expect.objectContaining({ contentTypeId: 'post', templateId: 'auto', cardId: 'card-1' }),
    ])
  })

  it('reports deckFailed instead of throwing when the slide append fails', async () => {
    repoRPC.mockImplementation((method: string) => {
      if (method === 'AppendDeckSlide') return Promise.reject(new Error('deck gone'))
      if (method === 'CreateCard') return Promise.resolve({ id: 'card-1' })
      return Promise.resolve(undefined)
    })
    const res = await savePlainShare({
      title: 'T',
      text: '',
      url: 'https://a.example/x',
      prefs: prefs({ deckTarget: deck, includeInDeck: true }),
    })
    // The card survives a failed deck append — the caller warns.
    expect(res).toEqual({ cardID: 'card-1', deckFailed: true })
  })

  it('skips the deck entirely when the toggle is off, even with a target set', async () => {
    repoRPC.mockResolvedValueOnce({ id: 'card-1' }).mockResolvedValueOnce(undefined)
    const res = await savePlainShare({
      title: 'T',
      text: '',
      url: 'https://a.example/x',
      prefs: prefs({ deckTarget: deck, includeInDeck: false }),
    })
    expect(res.deckFailed).toBe(false)
    expect(repoRPC).not.toHaveBeenCalledWith('AppendDeckSlide', expect.anything())
  })

  it('still returns the card when the description write fails', async () => {
    repoRPC.mockImplementation((method: string) => {
      if (method === 'CreateCard') return Promise.resolve({ id: 'card-1' })
      return Promise.reject(new Error('disk full'))
    })
    await expect(
      savePlainShare({ title: 'T', text: 'words', url: '', prefs: prefs() }),
    ).resolves.toEqual({ cardID: 'card-1', deckFailed: false })
  })

  it('propagates a failure to create the card (nothing was saved)', async () => {
    repoRPC.mockRejectedValueOnce(new Error('offline'))
    await expect(
      savePlainShare({ title: 'T', text: 'words', url: '', prefs: prefs() }),
    ).rejects.toThrow('offline')
  })
})
