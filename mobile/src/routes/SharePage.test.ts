import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import type { CapturePreview, CaptureResult } from '@shared/types'

// THE BUGS THIS FILE EXISTS FOR (2026-08-02)
// ------------------------------------------------------------------
// 1. A YouTube link shared from the phone was saved as a plain LINK CARD.
//    Nothing errored: Save was tapped before the debounced preflight
//    answered, the page had no verdict, and "no verdict" was treated as
//    "no plugin" — the silent fallback.
// 2. Everything else that went wrong that week was BRUV deciding FOR the
//    user — which video rung, what "too large" means, whether a blocked
//    capture looked like a normal one. So a consequential capture now
//    opens the Capture Options sheet and captures NOTHING until the user
//    says so.
// The tests below pin both.

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

/** A clippable, unremarkable preview: capture goes ahead with defaults. */
function preview(over: Partial<CapturePreview> = {}): CapturePreview {
  return {
    url: YT,
    platform: 'youtube',
    supported: true,
    blocked: false,
    title: 'A video',
    prefs: { videoMode: 'fit', imageMode: 'all', askMode: 'triggers', triggers: {} },
    shouldAsk: false,
    ...over,
  }
}

/** The 44-minute tweet that started all this: a real ladder, real sizes. */
function bigVideoPreview(over: Partial<CapturePreview> = {}): CapturePreview {
  return preview({
    shouldAsk: true,
    askReasons: ['video_large'],
    media: [
      {
        kind: 'video',
        url: 'https://cdn/v.mp4',
        defaultVariantId: 'sd',
        variants: [
          { id: 'sd', label: '480×270', url: 'https://cdn/sd.mp4', estBytes: 89 * 1024 * 1024 },
          { id: 'hd', label: '1280×720', url: 'https://cdn/hd.mp4', estBytes: 760 * 1024 * 1024 },
        ],
      },
    ],
    ...over,
  })
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

/** The CaptureOpts of the first (usually only) capture. */
function captureOpts(): Record<string, unknown> {
  return (callsTo('CaptureFromURL')[0][1] as unknown[])[1] as Record<string, unknown>
}

const saveButton = () => screen.getByRole('button', { name: 'Save to inbox' })
const clipButton = () => screen.getByRole('button', { name: 'Clip it' })
const captureButton = () => screen.getByRole('button', { name: 'Capture' })

beforeEach(() => {
  repoRPC.mockReset()
  showToast.mockReset()
  gotoShare('')
})

afterEach(() => {
  window.history.replaceState({}, '', '/m/share')
})

// --- the regression test ---------------------------------------------

describe('SharePage — save racing the preview', () => {
  it('REGRESSION (2026-08-02): a Save tapped before the check answers still CLIPS, never saves a plain card', async () => {
    let resolvePreview: (p: CapturePreview) => void = () => {}
    const slowPreview = new Promise<CapturePreview>((r) => {
      resolvePreview = r
    })
    stubRPC({
      PreviewCapture: () => slowPreview,
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
    resolvePreview(preview())

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
      PreviewCapture: () => Promise.resolve(preview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(saveButton())
    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    // Save cancels the debounced check and runs its own, so the URL is
    // previewed once — not once by the timer and once by Save.
    expect(callsTo('PreviewCapture')).toHaveLength(1)
  })

  it('captures the URL the PREVIEW was about, not whatever is in the field', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')

    // The URL field changes while the sheet is open (a late paste, a
    // stray debounce). The choices were made about the previewed post.
    await fireEvent.input(screen.getByLabelText('URL'), {
      target: { value: 'https://youtu.be/other' },
    })
    await fireEvent.click(captureButton())

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')[0][1]).toHaveProperty('0', YT)
  })
})

// --- the two normal modes --------------------------------------------

describe('SharePage — clip mode', () => {
  it('shows the platform chip once a plugin claims the URL', async () => {
    stubRPC({ PreviewCapture: () => Promise.resolve(preview()) })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    expect(await screen.findByText('Youtube', undefined, { timeout: 2000 })).toBeInTheDocument()
    expect(screen.getByText('BRUV can capture this post for you.')).toBeInTheDocument()
    // Clip mode relabels the primary action.
    expect(clipButton()).toBeInTheDocument()
  })

  it('captures via the server and lands on the new card', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(preview()),
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
    // shouldAsk was false: one tap, no dialog in the way.
    expect(screen.queryByText('Capture options')).toBeNull()
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
      PreviewCapture: () => Promise.resolve(preview()),
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
      PreviewCapture: () => Promise.resolve(preview({ platform: '', supported: false })),
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
    stubRPC({ PreviewCapture: () => Promise.resolve(preview()) })
    gotoShare(`?text=${encodeURIComponent(`Check this out ${YT}`)}`)
    render(SharePage)

    expect(await screen.findByText('Youtube', undefined, { timeout: 2000 })).toBeInTheDocument()
    expect(callsTo('PreviewCapture')[0][1]).toEqual([YT])
  })
})

// --- the check itself failing ----------------------------------------

describe('SharePage — when the preview cannot run', () => {
  it('does NOT claim the platform is unsupported, and tries to capture anyway', async () => {
    stubRPC({
      PreviewCapture: () => Promise.reject(new Error('network down')),
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
      PreviewCapture: () => Promise.reject(new Error('network down')),
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
      PreviewCapture: () => Promise.reject(new Error('network down')),
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
      PreviewCapture: () =>
        Promise.resolve(preview({ platform: 'truthsocial', url: 'https://truthsocial.com/@a/1' })),
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
      PreviewCapture: () => Promise.resolve(preview()),
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

// --- the capture options sheet ---------------------------------------

describe('SharePage — Capture Options', () => {
  it('opens the sheet and captures NOTHING until the user confirms', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))

    expect(await screen.findByText('Capture options')).toBeInTheDocument()
    // Why they're seeing it, in the user's own trigger's words.
    expect(screen.getByText('This video is large.')).toBeInTheDocument()
    // THE point of the whole feature: nothing was captured yet.
    expect(callsTo('CaptureFromURL')).toHaveLength(0)

    await fireEvent.click(captureButton())
    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
  })

  it('shows every rung of the ladder with its size, pre-selecting the vault default', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')

    expect(screen.getByRole('radio', { name: '480×270 · ~89 MB' })).toBeChecked()
    expect(screen.getByRole('radio', { name: '1280×720 · ~760 MB' })).not.toBeChecked()
    expect(screen.getByRole('radio', { name: /Link only/ })).toBeInTheDocument()
  })

  it('passes the rung the user picked as videoVariantId (even the huge one)', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')
    await fireEvent.click(screen.getByRole('radio', { name: '1280×720 · ~760 MB' }))
    await fireEvent.click(captureButton())

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(captureOpts()).toMatchObject({ videoVariantId: 'hd', videoMode: 'fit' })
  })

  it('sends videoMode "link" when the user chooses link-only', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')
    await fireEvent.click(screen.getByRole('radio', { name: /^Link only/ }))
    await fireEvent.click(captureButton())

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(captureOpts()).toMatchObject({ videoMode: 'link' })
    expect(captureOpts()).not.toHaveProperty('videoVariantId')
  })

  it('sends an edited title, and leaves an untouched one to the server', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(bigVideoPreview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')

    const titleField = screen.getByLabelText('Title')
    expect(titleField).toHaveValue('A video')
    await fireEvent.input(titleField, { target: { value: 'The 44-minute one' } })
    await fireEvent.click(captureButton())

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(captureOpts()).toMatchObject({ title: 'The 44-minute one' })
  })

  it('offers the gallery choice with a real count', async () => {
    stubRPC({
      PreviewCapture: () =>
        Promise.resolve(
          preview({
            shouldAsk: true,
            askReasons: ['gallery_large'],
            media: [
              { kind: 'image', url: 'a' },
              { kind: 'image', url: 'b' },
              { kind: 'image', url: 'c' },
            ],
          }),
        ),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')
    expect(screen.getByText('This post has a lot of images.')).toBeInTheDocument()

    await fireEvent.click(screen.getByRole('radio', { name: 'First image only' }))
    await fireEvent.click(captureButton())

    await waitFor(() => expect(callsTo('CaptureFromURL')).toHaveLength(1))
    expect(captureOpts()).toMatchObject({ imageMode: 'first' })
  })

  it('says a blocked capture lands as a pending clip — never dresses it up as a normal one', async () => {
    stubRPC({
      PreviewCapture: () =>
        Promise.resolve(
          preview({
            platform: 'truthsocial',
            blocked: true,
            shouldAsk: true,
            askReasons: ['blocked'],
          }),
        ),
      CaptureFromURL: () => Promise.resolve(result({ pending: true })),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Clip it' }, { timeout: 2000 }))
    await screen.findByText('Capture options')

    expect(
      screen.getByText(
        "truthsocial wouldn't let the server read this post. Capturing saves a pending clip with the link — you finish it from the desktop clipper, not here.",
      ),
    ).toBeInTheDocument()
  })

  it('says an unsupported link will be a link card, and takes the plain path on confirm', async () => {
    stubRPC({
      PreviewCapture: () =>
        Promise.resolve(
          preview({
            platform: '',
            supported: false,
            shouldAsk: true,
            askReasons: ['unsupported'],
            title: 'example.com',
          }),
        ),
      CreateCard: () => Promise.resolve({ id: 'card-plain' }),
    })
    gotoShare('?url=https%3A%2F%2Fexample.com%2Fa')
    render(SharePage)

    await fireEvent.click(saveButton())
    await screen.findByText('Capture options')
    expect(
      screen.getByText(
        'No capture plugin claims this link, so it saves as a plain link card — no post text, no media.',
      ),
    ).toBeInTheDocument()

    await fireEvent.click(captureButton())
    await waitFor(() => expect(callsTo('CreateCard')).toHaveLength(1))
    expect(callsTo('CaptureFromURL')).toHaveLength(0)
  })

  it('can be opened by hand even when nothing triggered it', async () => {
    stubRPC({
      PreviewCapture: () => Promise.resolve(preview()),
      CaptureFromURL: () => Promise.resolve(result()),
    })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Options' }, { timeout: 2000 }))
    expect(await screen.findByText('Capture options')).toBeInTheDocument()
    expect(callsTo('CaptureFromURL')).toHaveLength(0)
  })

  it('surfaces a preview failure when Options is tapped instead of opening an empty sheet', async () => {
    stubRPC({ PreviewCapture: () => Promise.reject(new Error('network down')) })
    gotoShare(`?url=${encodeURIComponent(YT)}`)
    render(SharePage)

    await fireEvent.click(await screen.findByRole('button', { name: 'Options' }, { timeout: 2000 }))

    expect(await screen.findByRole('alert')).toHaveTextContent('network down')
    expect(screen.queryByText('Capture options')).toBeNull()
  })
})
