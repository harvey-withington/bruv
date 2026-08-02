import { describe, it, expect, vi, beforeEach } from 'vitest'

// Capture prefs are "just convenience", which is exactly why they're
// dangerous: every failure mode here is silent. A dropped deck ID
// downgrades "pin with the deck" to Inbox, and corrupt storage could
// take a capture down with it. Both are locked below.

let activeRepo: string | null = 'repo-a'
vi.mock('./auth', () => ({ readActiveRepoID: () => activeRepo }))

const {
  loadCapturePrefs,
  saveCapturePrefs,
  captureOptsFrom,
  defaultCapturePrefs,
  DECK_PIN_SENTINEL,
} = await import('./capturePrefs')

const deck = { cardID: 'deck-card', blockID: 'blk-1', name: 'Reading' }

beforeEach(() => {
  activeRepo = 'repo-a'
  localStorage.clear()
})

describe('loadCapturePrefs / saveCapturePrefs', () => {
  it('returns the defaults when nothing has been saved', () => {
    expect(loadCapturePrefs()).toEqual({
      deckTarget: null,
      categoryID: '',
      categoryName: '',
      includeInDeck: false,
    })
  })

  it('round-trips a full set of prefs', () => {
    const prefs = {
      deckTarget: deck,
      categoryID: 'cat-7',
      categoryName: 'Research',
      includeInDeck: true,
    }
    saveCapturePrefs(prefs)
    expect(loadCapturePrefs()).toEqual(prefs)
  })

  it('scopes prefs per vault — another repo sees its own, not this one', () => {
    // A deck card ID from one vault is meaningless in another.
    saveCapturePrefs({ ...defaultCapturePrefs(), deckTarget: deck, includeInDeck: true })
    activeRepo = 'repo-b'
    expect(loadCapturePrefs()).toEqual(defaultCapturePrefs())

    saveCapturePrefs({ ...defaultCapturePrefs(), categoryID: 'cat-b' })
    activeRepo = 'repo-a'
    expect(loadCapturePrefs().deckTarget).toEqual(deck)
    expect(loadCapturePrefs().categoryID).toBe('')
  })

  it('stores under a repo-keyed storage key', () => {
    saveCapturePrefs({ ...defaultCapturePrefs(), categoryID: 'cat-7' })
    expect(localStorage.getItem('bruv:capture_prefs:repo-a')).toContain('cat-7')
  })

  it('falls back to the defaults with no active repo, and saving is a no-op', () => {
    activeRepo = null
    expect(loadCapturePrefs()).toEqual(defaultCapturePrefs())
    expect(() => saveCapturePrefs({ ...defaultCapturePrefs(), categoryID: 'cat-7' })).not.toThrow()
    expect(localStorage.length).toBe(0)
  })

  it('falls back to the defaults on corrupt JSON instead of throwing', () => {
    localStorage.setItem('bruv:capture_prefs:repo-a', '{not json at all')
    expect(() => loadCapturePrefs()).not.toThrow()
    expect(loadCapturePrefs()).toEqual(defaultCapturePrefs())
  })

  it('falls back to the defaults when the stored value is not an object', () => {
    for (const raw of ['"a string"', 'null', '42', 'true']) {
      localStorage.setItem('bruv:capture_prefs:repo-a', raw)
      expect(loadCapturePrefs(), raw).toEqual(defaultCapturePrefs())
    }
  })

  it('drops a half-written deck target rather than trusting it', () => {
    // A target missing either ID can't address a deck block, so it is
    // no target at all — never a partially-addressed append.
    const bad: unknown[] = [
      { cardID: 'c', name: 'x' },
      { blockID: 'b' },
      { cardID: '', blockID: 'b' },
      { cardID: 'c', blockID: '' },
      { cardID: 1, blockID: 2 },
      'not-an-object',
      null,
    ]
    for (const deckTarget of bad) {
      localStorage.setItem('bruv:capture_prefs:repo-a', JSON.stringify({ deckTarget }))
      expect(loadCapturePrefs().deckTarget, JSON.stringify(deckTarget)).toBeNull()
    }
  })

  it('tolerates a deck target with no name', () => {
    localStorage.setItem(
      'bruv:capture_prefs:repo-a',
      JSON.stringify({ deckTarget: { cardID: 'c', blockID: 'b' } }),
    )
    expect(loadCapturePrefs().deckTarget).toEqual({ cardID: 'c', blockID: 'b', name: '' })
  })

  it('coerces non-string category fields and non-boolean toggles to the defaults', () => {
    localStorage.setItem(
      'bruv:capture_prefs:repo-a',
      JSON.stringify({ categoryID: 5, categoryName: null, includeInDeck: 'yes' }),
    )
    expect(loadCapturePrefs()).toEqual(defaultCapturePrefs())
  })
})

describe('captureOptsFrom', () => {
  it('sends the deck IDs even when includeInDeck is FALSE', () => {
    // The load-bearing one. The server resolves the "__deck__" pin
    // sentinel from deckCardID, so dropping the IDs when the toggle is
    // off silently downgrades "pin with the deck" to Inbox — the exact
    // silent fallback this suite exists for (2026-08-02).
    const opts = captureOptsFrom({
      deckTarget: deck,
      categoryID: DECK_PIN_SENTINEL,
      categoryName: '',
      includeInDeck: false,
    })
    expect(opts).toEqual({
      includeInDeck: false,
      deckCardID: 'deck-card',
      deckBlockID: 'blk-1',
      categoryID: DECK_PIN_SENTINEL,
    })
  })

  it('sends the deck IDs and the flag when includeInDeck is true', () => {
    const opts = captureOptsFrom({
      deckTarget: deck,
      categoryID: '',
      categoryName: '',
      includeInDeck: true,
    })
    expect(opts).toEqual({
      includeInDeck: true,
      deckCardID: 'deck-card',
      deckBlockID: 'blk-1',
      categoryID: '',
    })
  })

  it('forces includeInDeck false when there is no deck target to append to', () => {
    const opts = captureOptsFrom({
      deckTarget: null,
      categoryID: '',
      categoryName: '',
      includeInDeck: true,
    })
    expect(opts.includeInDeck).toBe(false)
    expect(opts.deckCardID).toBe('')
    expect(opts.deckBlockID).toBe('')
  })

  it('degrades the deck pin sentinel to Inbox when there is no deck target', () => {
    // The sentinel means "wherever the deck card is pinned" — with no
    // deck, that resolves to nothing, so ask for the Inbox explicitly
    // rather than sending a sentinel the server can't resolve.
    const opts = captureOptsFrom({
      deckTarget: null,
      categoryID: DECK_PIN_SENTINEL,
      categoryName: 'Reading',
      includeInDeck: false,
    })
    expect(opts.categoryID).toBe('')
  })

  it('passes a real category ID through untouched, deck or no deck', () => {
    const base = { categoryID: 'cat-7', categoryName: 'Research', includeInDeck: false }
    expect(captureOptsFrom({ ...base, deckTarget: null }).categoryID).toBe('cat-7')
    expect(captureOptsFrom({ ...base, deckTarget: deck }).categoryID).toBe('cat-7')
  })

  it('sends an empty category (Inbox) for the default prefs', () => {
    expect(captureOptsFrom(defaultCapturePrefs())).toEqual({
      includeInDeck: false,
      deckCardID: '',
      deckBlockID: '',
      categoryID: '',
    })
  })
})
