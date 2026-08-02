import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import type { CaptureResult } from '@shared/types'

// THE BUG THIS FILE EXISTS FOR (2026-08-02)
// ------------------------------------------------------------------
// A YouTube link shared from the phone was saved as a plain LINK CARD.
// Nothing errored: Save was tapped before the debounced MatchCaptureURL
// answered, the page had no verdict, and "no verdict" was treated as
// "no plugin" — the silent fallback. The first test below is the
// regression test for exactly that sequence; the rest lock the other
// branches of the clip-vs-plain decision so the same confusion can't
// creep back in through a different door.

const repoRPC = vi.fn()
const showToast = vi.fn()

vi.mock('../lib/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/auth')>()
  return {
    ...actual,
    repoRPC: (...args: unknown[]) => repoRPC(...args),
    // Prefs are stored per vault; pin one so the storage key is stable.
    readActiveRepoID: () => 'test-repo',
  }
})

vi.mock('../lib/toast.svelte', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/toast.svelte')>()
  return { ...actual, showToast: (...args: unknown[]) => showToast(...args) }
})

const SharePage = (await import('./SharePage.svelte')).default

// --- helpers ---------------------------------------------------------

const YT = 'https://youtu.be/abc'

/** Put the share params where SharePage reads them: the real URL. */
function gotoShare(query: string): void {
  window.history.replaceState({}, '', `/m/share${query}`)
}

function result(over: Partial<CaptureResult> = {}): CaptureResult {
  return { cardId: 'card-clip', slideAppended: true, platform: 'youtube', pending: false, ...over }
}

type Handler = (params: unknown[]) => Promise<unknown>

/** Route RPCs by method name; anything unnamed resolves empty. */
function stubRPC(handlers: Record<string, Handler>): void {
  repoRPC.mockImplementation((method: string, params: unknown[] = []) => {
    const h = handlers[method]
    if (h) return h(params)
    if (method === 'CreateCard') return Promise.resolve({ id: 'card-plain' })
    return Promise.resolve(undefined)
  })
}

function callsTo(method: string): unknown[][] {
  return repoRPC.mock.calls.filter((c) => c[0] === method)
}

const saveButton = () => screen.getByRole('button', { name: 'Save to inbox' })
const clipButton = () => screen.getByRole('button', { name: 'Clip it' })

beforeEach(() => {
  repoRPC.mockReset()
  showToast.mockReset()
  gotoShare('')
})

afterEach(() => {
  window.history.replaceState({}, '', '/m/share')
})

// --- the regression test ---------------------------------------------

describe('SharePage — save racing the plugin check', () => {
  it('REGRESSION (2026-08-02): a Save tapped before the check answers still CLIPS, never saves a plain card', async () => {
    let resolveMatch: (platform: string) => void = () => {}
    const slowMatch = new Promise<string>((r) => {
      resolveMatch = r
    })
    stubRPC({
      MatchCaptureURL: () => slowMatch,
      CaptureFromURL: () => Promise.resolve(result()),
    })

    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    // Tap Save at once — before the 300ms debounce has even fired, so
    // the page holds no verdict about this URL.
    await fireEvent.click(saveButton())
    expect(callsTo('CreateCard')).toHaveLength(0)
    expect(callsTo('CaptureFromURL')).toHaveLength(0)

    // The answer finally arrives: YouTube IS clippable.
    resolveMatch('youtube')

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    // The whole point: no plain card was created behind the user's back.
    expect(callsTo('CreateCard')).toHaveLength(0)
    expect(callsTo('CaptureFromURL')[0][1]).toEqual([
      YT,
      { includeInDeck: false, deckCardID: '', deckBlockID: '', categoryID: '' },
    ])
  })

  it('asks the server exactly once when Save beats the debounce (the pending check is cancelled)', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.resolve('youtube'),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(saveButton())
    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    // Save cancels the debounced check and runs its own, so the URL is
    // matched once — not once by the timer and once by Save.
    expect(callsTo('MatchCaptureURL')).toHaveLength(1)
  })
})

// --- the two normal modes --------------------------------------------

describe('SharePage — clip mode', () => {
  it('shows the platform chip once a plugin claims the URL', async () => {
    stubRPC({ MatchCaptureURL: () => Promise.resolve('youtube') })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    expect(await screen.findByText('Youtube', undefined, { timeout: 2000 })).toBeInTheDocument()
    expect(screen.getByText('BRUV can capture this post for you.')).toBeInTheDocument()
    // Clip mode relabels the primary action.
    expect(clipButton()).toBeInTheDocument()
  })

  it('captures via the server and lands on the new card', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.resolve('youtube'),
      CaptureFromURL: () => Promise.resolve(result({ cardId: 'card-9' })),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')[0][1]).toEqual([
      YT,
      { includeInDeck: false, deckCardID: '', deckBlockID: '', categoryID: '' },
    ])
    expect(callsTo('CreateCard')).toHaveLength(0)
    expect(showToast).toHaveBeenCalledWith('Clipped.', 'success')
    await waitFor(() => expect(window.location.pathname).toBe('/m/c/card-9'))
  })

  it('sends the sticky deck target with the capture and warns when the slide is dropped', async () => {
    localStorage.setItem(
      'bruv:capture_prefs:test-repo',
      JSON.stringify({
        deckTarget: { cardID: 'deck-card', blockID: 'blk-1', name: 'Reading' },
        categoryID: '__deck__',
        categoryName: '',
        includeInDeck: true,
      }),
    )
    stubRPC({
      MatchCaptureURL: () => Promise.resolve('youtube'),
      CaptureFromURL: () => Promise.resolve(result({ slideAppended: false })),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')[0][1]).toEqual([
      YT,
      {
        includeInDeck: true,
        deckCardID: 'deck-card',
        deckBlockID: 'blk-1',
        categoryID: '__deck__',
      },
    ])
    // A deck append that silently didn't happen is exactly the kind of
    // fallback the user must hear about.
    expect(showToast).toHaveBeenCalledWith("Card saved, but the deck slide wasn't added.", 'warning')
  })
})

describe('SharePage — plain mode', () => {
  it('says up front that no plugin claims the URL, and saves a plain card', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.resolve(''),
      CreateCard: () => Promise.resolve({ id: 'card-plain' }),
    })
    gotoShare('?url=https%3A%2F%2Fwww.instagram.com%2Freel%2Fabc%2F')
    render(SharePage)

    expect(await screen.findByText('Link', undefined, { timeout: 2000 })).toBeInTheDocument()
    expect(
      screen.getByText('No capture plugin for instagram.com yet — saving as a link card.'),
    ).toBeInTheDocument()

    await fireEvent.click(saveButton())

    await waitFor(() => expect(callsTo('CreateCard')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')).toHaveLength(0)
    await waitFor(() => expect(window.location.pathname).toBe('/m/c/card-plain'))
  })

  it('lifts a URL buried in the shared TEXT so it can still clip (YouTube app share)', async () => {
    stubRPC({ MatchCaptureURL: () => Promise.resolve('youtube') })
    gotoShare(`?text=${encodeURIComponent(`Check this out ${YT}`)}`)
    render(SharePage)

    expect(await screen.findByText('Youtube', undefined, { timeout: 2000 })).toBeInTheDocument()
    expect(callsTo('MatchCaptureURL')[0][1]).toEqual([YT])
  })
})

// --- the check itself failing ----------------------------------------

describe('SharePage — when the plugin check cannot run', () => {
  it('does NOT claim the platform is unsupported, and tries to capture anyway', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.reject(new Error('network down')),
      CaptureFromURL: () => Promise.resolve(result({ cardId: 'card-9' })),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    expect(
      await screen.findByText(
        "Couldn't check this link — saving will still try to capture it.",
        undefined,
        { timeout: 2000 },
      ),
    ).toBeInTheDocument()
    // Unknown is not the same as unsupported.
    expect(screen.queryByText('Link')).toBeNull()

    await fireEvent.click(saveButton())

    // The server is the authority — ask it rather than guessing plain.
    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(callsTo('CreateCard')).toHaveLength(0)
  })

  it('falls back to a plain card ONLY when the server says it has no capture support', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.reject(new Error('network down')),
      CaptureFromURL: () => Promise.reject(new Error('no capture support for example.com')),
      CreateCard: () => Promise.resolve({ id: 'card-plain' }),
    })
    gotoShare('?url=https%3A%2F%2Fexample.com%2Fa')
    render(SharePage)

    await fireEvent.click(saveButton())

    await waitFor(() => expect(callsTo('CreateCard')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')).toHaveLength(1)
  })

  it('surfaces an unrelated capture failure instead of quietly making a link card', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.reject(new Error('network down')),
      CaptureFromURL: () => Promise.reject(new Error('server exploded')),
    })
    gotoShare('?url=https%3A%2F%2Fexample.com%2Fa')
    render(SharePage)

    await fireEvent.click(saveButton())

    expect(await screen.findByRole('alert')).toHaveTextContent('server exploded')
    expect(callsTo('CreateCard')).toHaveLength(0)
  })
})

// --- outcomes the user must not miss ---------------------------------

describe('SharePage — half-done and bounced captures', () => {
  it('shows the pending panel and does NOT navigate away like a success', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.resolve('truthsocial'),
      CaptureFromURL: () =>
        Promise.resolve(
          result({ cardId: 'card-9', pending: true, slideAppended: false, platform: 'truthsocial' }),
        ),
    })
    gotoShare('?url=https%3A%2F%2Ftruthsocial.com%2F%40a%2F1')
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))

    expect(await screen.findByText('Finish this clip on desktop')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'View card' })).toBeInTheDocument()
    // Still on the share page — a half-done clip never silently leaves.
    expect(window.location.pathname).toBe('/m/share')
    expect(showToast).not.toHaveBeenCalledWith('Clipped.', 'success')
  })

  it('warns with the server reason when the pin bounced and the card went to the Inbox', async () => {
    stubRPC({
      MatchCaptureURL: () => Promise.resolve('youtube'),
      CaptureFromURL: () =>
        Promise.resolve(result({ pinFailed: true, pinError: 'category no longer exists' })),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))

    await waitFor(() =>
      expect(showToast).toHaveBeenCalledWith(
        "Couldn't pin the card — it's in the Inbox. category no longer exists",
        'warning',
        7000,
      ),
    )
  })
})
