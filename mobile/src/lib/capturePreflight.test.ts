import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { CapturePreview } from '@shared/types'

// The bug this file exists for (2026-08-02): a YouTube link was saved as
// a plain link card because the share page couldn't tell "no plugin"
// apart from "haven't checked yet" or "the check failed". Every
// assertion below is a distinct way that confusion can come back.
//
// The verdict is a full CapturePreview now (the capture-options work of
// the same day) — richer facts, identical discipline.

const repoRPC = vi.fn()
vi.mock('./auth', () => ({ repoRPC: (...args: unknown[]) => repoRPC(...args) }))

const { createPreflight } = await import('./capturePreflight.svelte')

function preview(over: Partial<CapturePreview> = {}): CapturePreview {
  return {
    url: 'https://youtu.be/abc',
    platform: 'youtube',
    supported: true,
    blocked: false,
    title: 'A video',
    prefs: { triggers: {} },
    shouldAsk: false,
    ...over,
  }
}

beforeEach(() => {
  repoRPC.mockReset()
})

describe('createPreflight', () => {
  it('records a verdict tied to the URL it was made about', async () => {
    repoRPC.mockResolvedValue(preview())
    const pf = createPreflight()

    await pf.check('https://youtu.be/p9qlWDoH7zE?si=7RliB4Qrgrdv-sEL')

    expect(pf.platform).toBe('youtube')
    expect(pf.failed).toBe(false)
    expect(pf.preview?.title).toBe('A video')
    expect(pf.hasVerdictFor('https://youtu.be/p9qlWDoH7zE?si=7RliB4Qrgrdv-sEL')).toBe(true)
    // A verdict about one URL says nothing about another.
    expect(pf.hasVerdictFor('https://example.com/other')).toBe(false)
  })

  it('asks the server what capture WOULD do (a preview writes nothing)', async () => {
    repoRPC.mockResolvedValue(preview())
    const pf = createPreflight()
    await pf.check('https://youtu.be/abc')
    expect(repoRPC).toHaveBeenCalledWith('PreviewCapture', ['https://youtu.be/abc'])
  })

  it('tolerates surrounding whitespace when matching the verdict URL', async () => {
    repoRPC.mockResolvedValue(preview({ platform: 'twitter' }))
    const pf = createPreflight()
    await pf.check('  https://x.com/a/status/1  ')
    expect(pf.hasVerdictFor('https://x.com/a/status/1')).toBe(true)
  })

  it('reports no-match as a REAL verdict (so the UI may say "unsupported")', async () => {
    repoRPC.mockResolvedValue(preview({ platform: '', supported: false }))
    const pf = createPreflight()

    await pf.check('https://www.instagram.com/reel/abc/')

    expect(pf.platform).toBe('')
    expect(pf.failed).toBe(false)
    expect(pf.preview?.supported).toBe(false)
    expect(pf.hasVerdictFor('https://www.instagram.com/reel/abc/')).toBe(true)
  })

  it('keeps clip mode for a BLOCKED platform (it is still a capture, just a pending one)', async () => {
    repoRPC.mockResolvedValue(preview({ platform: 'truthsocial', blocked: true }))
    const pf = createPreflight()
    await pf.check('https://truthsocial.com/@a/1')
    expect(pf.platform).toBe('truthsocial')
    expect(pf.preview?.blocked).toBe(true)
  })

  it('reports a failed check as UNKNOWN, never as a no-match', async () => {
    repoRPC.mockRejectedValue(new Error('network down'))
    const pf = createPreflight()

    await pf.check('https://youtu.be/abc')

    expect(pf.failed).toBe(true)
    expect(pf.platform).toBe('')
    expect(pf.preview).toBeNull()
    // The load-bearing assertion: no verdict, so the UI must not claim
    // the platform is unsupported and Save must not assume plain mode.
    expect(pf.hasVerdictFor('https://youtu.be/abc')).toBe(false)
  })

  it('treats an empty answer as UNKNOWN too, not as a no-match', async () => {
    repoRPC.mockResolvedValue(null)
    const pf = createPreflight()
    await pf.check('https://youtu.be/abc')
    expect(pf.failed).toBe(true)
    expect(pf.hasVerdictFor('https://youtu.be/abc')).toBe(false)
  })

  it('clears state for an empty URL without calling the server', async () => {
    const pf = createPreflight()
    await pf.check('   ')
    expect(repoRPC).not.toHaveBeenCalled()
    expect(pf.platform).toBe('')
    expect(pf.hasVerdictFor('')).toBe(false)
  })

  it('ignores a slow answer for an older URL (stale-response guard)', async () => {
    const pf = createPreflight()
    let resolveFirst: (v: CapturePreview) => void = () => {}
    repoRPC
      .mockImplementationOnce(() => new Promise<CapturePreview>((r) => (resolveFirst = r)))
      .mockResolvedValueOnce(preview())

    const first = pf.check('https://x.com/a/status/1') // will answer LAST
    await pf.check('https://youtu.be/abc') // answers first, newer
    resolveFirst(preview({ platform: 'twitter', url: 'https://x.com/a/status/1' }))
    await first

    expect(pf.platform).toBe('youtube')
    expect(pf.hasVerdictFor('https://youtu.be/abc')).toBe(true)
    expect(pf.hasVerdictFor('https://x.com/a/status/1')).toBe(false)
  })

  it('debounces scheduled checks and can be cancelled', async () => {
    vi.useFakeTimers()
    repoRPC.mockResolvedValue(preview())
    const pf = createPreflight()

    pf.schedule('https://youtu.be/a', 300)
    pf.schedule('https://youtu.be/b', 300)
    expect(repoRPC).not.toHaveBeenCalled() // still inside the debounce

    await vi.advanceTimersByTimeAsync(300)
    expect(repoRPC).toHaveBeenCalledTimes(1)
    expect(repoRPC).toHaveBeenCalledWith('PreviewCapture', ['https://youtu.be/b'])

    pf.schedule('https://youtu.be/c', 300)
    pf.cancel()
    await vi.advanceTimersByTimeAsync(600)
    expect(repoRPC).toHaveBeenCalledTimes(1) // cancelled, never fired
    vi.useRealTimers()
  })
})
