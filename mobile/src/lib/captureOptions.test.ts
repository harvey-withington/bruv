import { describe, it, expect, vi } from 'vitest'
import type { CapturePreview } from '@shared/types'

// The rules the Capture Options sheet renders, tested without a DOM:
// what gets pre-selected, and what actually reaches the server. Both are
// where the silent decisions used to hide.

vi.mock('./auth', () => ({
  repoRPC: vi.fn(),
  readActiveRepoID: () => 'test-repo',
}))

const { defaultChoices, captureOptsWith, formatEstBytes, askReasonText } = await import(
  './captureOptions'
)
const { defaultCapturePrefs } = await import('./capturePrefs')

const MB = 1024 * 1024

function preview(over: Partial<CapturePreview> = {}): CapturePreview {
  return {
    url: 'https://x.com/a/status/1',
    platform: 'twitter',
    supported: true,
    blocked: false,
    title: 'A post',
    prefs: { videoMode: 'fit', imageMode: 'all', askMode: 'triggers', triggers: {} },
    shouldAsk: false,
    ...over,
  }
}

function withVideo(over: Partial<CapturePreview> = {}): CapturePreview {
  return preview({
    media: [
      {
        kind: 'video',
        url: 'https://cdn/v.mp4',
        defaultVariantId: 'sd',
        variants: [
          { id: 'sd', label: '480×270', url: 'https://cdn/sd.mp4', estBytes: 85 * MB },
          { id: 'hd', label: '1280×720', url: 'https://cdn/hd.mp4', estBytes: 725 * MB },
        ],
      },
    ],
    ...over,
  })
}

describe('formatEstBytes', () => {
  it('reads in MB up to a gigabyte and GB above it', () => {
    expect(formatEstBytes(85 * MB)).toBe('85 MB')
    expect(formatEstBytes(725 * MB)).toBe('725 MB')
    expect(formatEstBytes(3.5 * 1024 * MB)).toBe('3.5 GB')
    expect(formatEstBytes(4.2 * MB)).toBe('4.2 MB')
  })

  it('says nothing rather than "0 MB" when the size is unknown', () => {
    expect(formatEstBytes(undefined)).toBe('')
    expect(formatEstBytes(0)).toBe('')
  })
})

describe('defaultChoices', () => {
  it('pre-selects the rung the vault would take', () => {
    expect(defaultChoices(withVideo()).video).toEqual({ kind: 'variant', id: 'sd' })
  })

  it('treats "no rung fits your budget" as LINK, not as "store the biggest anyway"', () => {
    const p = withVideo()
    p.media![0].defaultVariantId = ''
    expect(defaultChoices(p).video).toEqual({ kind: 'link' })
  })

  it('honours a link-only or skip vault default over any ladder', () => {
    expect(defaultChoices(withVideo({ prefs: { videoMode: 'link', triggers: {} } })).video).toEqual({
      kind: 'link',
    })
    expect(defaultChoices(withVideo({ prefs: { videoMode: 'skip', triggers: {} } })).video).toEqual({
      kind: 'skip',
    })
  })

  it('still offers store-or-not for a video the platform gave no ladder for', () => {
    const p = preview({ media: [{ kind: 'video', url: 'https://cdn/v.mp4', estBytes: 12 * MB }] })
    expect(defaultChoices(p).video).toEqual({ kind: 'variant', id: '' })
  })

  it('leaves video/images null when the post has neither', () => {
    const c = defaultChoices(preview())
    expect(c.video).toBeNull()
    expect(c.imageMode).toBeNull()
    expect(c.title).toBe('A post')
  })

  it('pre-fills the image mode from the vault when there are images', () => {
    const p = preview({
      media: [
        { kind: 'image', url: 'a' },
        { kind: 'image', url: 'b' },
      ],
      prefs: { imageMode: 'first', triggers: {} },
    })
    expect(defaultChoices(p).imageMode).toBe('first')
  })
})

describe('captureOptsWith', () => {
  const prefs = defaultCapturePrefs()

  it('sends nothing extra when the user changed nothing (defaults still rule server-side)', () => {
    const p = preview()
    expect(captureOptsWith(prefs, defaultChoices(p), p)).toEqual({
      includeInDeck: false,
      deckCardID: '',
      deckBlockID: '',
      categoryID: '',
    })
  })

  it('sends a picked rung as videoVariantId, with a mode that cannot cancel it', () => {
    const p = withVideo({ prefs: { videoMode: 'link', triggers: {} } })
    const opts = captureOptsWith(prefs, { title: p.title, video: { kind: 'variant', id: 'hd' }, imageMode: null }, p)
    // The vault default is link-only; the explicit choice must win, so
    // the mode goes out as a storing one.
    expect(opts.videoVariantId).toBe('hd')
    expect(opts.videoMode).toBe('fit')
  })

  it('sends link and skip as modes, with no variant', () => {
    const p = withVideo()
    const link = captureOptsWith(prefs, { title: p.title, video: { kind: 'link' }, imageMode: null }, p)
    expect(link.videoMode).toBe('link')
    expect(link.videoVariantId).toBeUndefined()

    const skip = captureOptsWith(prefs, { title: p.title, video: { kind: 'skip' }, imageMode: null }, p)
    expect(skip.videoMode).toBe('skip')
  })

  it('sends the title only when the user actually changed it', () => {
    const p = preview()
    expect(captureOptsWith(prefs, { title: '  A post  ', video: null, imageMode: null }, p).title)
      .toBeUndefined()
    expect(
      captureOptsWith(prefs, { title: 'Something better', video: null, imageMode: null }, p).title,
    ).toBe('Something better')
  })

  it('keeps the sticky deck and pin targets', () => {
    const p = preview()
    const opts = captureOptsWith(
      {
        deckTarget: { cardID: 'deck-1', blockID: 'blk-1', name: 'Reading' },
        categoryID: '__deck__',
        categoryName: '',
        includeInDeck: true,
      },
      defaultChoices(p),
      p,
    )
    expect(opts).toMatchObject({
      includeInDeck: true,
      deckCardID: 'deck-1',
      deckBlockID: 'blk-1',
      categoryID: '__deck__',
    })
  })
})

describe('askReasonText', () => {
  it('explains each trigger that fired, once', () => {
    expect(askReasonText(preview({ askReasons: ['video_large'] }))).toBe('This video is large.')
    expect(askReasonText(preview({ askReasons: ['video_large', 'video_large'] }))).toBe(
      'This video is large.',
    )
    expect(askReasonText(preview({ askReasons: ['blocked', 'gallery_large'] }))).toBe(
      "The platform wouldn't let the server read this post. This post has a lot of images.",
    )
  })

  it('falls back to a plain line for "always ask" and for reasons it does not know', () => {
    const fallback = 'Check what BRUV will capture before it does.'
    expect(askReasonText(preview({ askReasons: [] }))).toBe(fallback)
    expect(askReasonText(preview({ askReasons: ['from_a_newer_server'] }))).toBe(fallback)
  })
})
