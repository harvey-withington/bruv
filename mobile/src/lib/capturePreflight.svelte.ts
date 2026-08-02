// "What would capturing this URL actually do?" — the state machine behind
// the share page's clip-vs-plain decision, and now the source of the
// facts the Capture Options sheet presents.
//
// Extracted because getting this wrong is silent and expensive: on
// 2026-08-02 a YouTube link was saved as a plain link card because Save
// was tapped before the debounced check returned, and the page had no way
// to tell "no plugin" apart from "haven't checked yet" or "the check
// failed". A verdict is therefore always tied to the URL it was made
// about, and failure is its own state — never mistaken for a no-match.
//
// The verdict is a full CapturePreview now (PreviewCapture writes
// nothing): same discipline, more facts — the media inventory, the video
// quality ladder with estimated sizes, and the vault's own answer to
// "should we ask the user about this one?".

import { repoRPC } from './auth'
import type { CapturePreview } from '@shared/types'

export type Preflight = {
  /** What capture WOULD do, or null when there's no verdict. Only ever
   *  about the URL hasVerdictFor() agrees with. */
  readonly preview: CapturePreview | null
  /** Matched platform id, '' when there's no verdict or no plugin. */
  readonly platform: string
  readonly checking: boolean
  /** The check itself errored — we do NOT know whether it's clippable. */
  readonly failed: boolean
  /** Why the last check failed, for callers that must show a reason
   *  rather than a shrug. '' when nothing has failed. */
  readonly error: string
  /** True only when the current verdict was made about exactly this URL. */
  hasVerdictFor(url: string): boolean
  /** Run the check now and await it (callers that must not race). */
  check(url: string): Promise<void>
  /** Schedule a debounced check (typing/pasting). */
  schedule(url: string, delayMs?: number): void
  cancel(): void
}

export function createPreflight(): Preflight {
  let preview = $state<CapturePreview | null>(null)
  let checking = $state(false)
  let failed = $state(false)
  let error = $state('')
  let verdictFor = $state('')
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  function clear() {
    preview = null
    verdictFor = ''
  }

  async function check(raw: string): Promise<void> {
    const url = raw.trim()
    const mine = ++seq
    if (!url) {
      clear()
      failed = false
      error = ''
      checking = false
      return
    }
    checking = true
    try {
      const result = await repoRPC<CapturePreview | null>('PreviewCapture', [url])
      if (mine !== seq) return // a newer URL already won
      if (!result) throw new Error('empty preview')
      preview = result
      verdictFor = url
      failed = false
      error = ''
    } catch (err) {
      if (mine !== seq) return
      // Unknown, NOT "unsupported" — the caller decides what to do with
      // that, and the UI must not claim the platform isn't supported.
      clear()
      failed = true
      error = err instanceof Error ? err.message : String(err)
    } finally {
      if (mine === seq) checking = false
    }
  }

  return {
    get preview() {
      return preview
    },
    get platform() {
      // A preview for a URL no plugin claims still carries the platform
      // field empty; supported is the flag that decides clip mode.
      return preview?.supported ? preview.platform : ''
    },
    get checking() {
      return checking
    },
    get failed() {
      return failed
    },
    get error() {
      return error
    },
    hasVerdictFor(url: string) {
      return verdictFor !== '' && verdictFor === url.trim()
    },
    check,
    schedule(url: string, delayMs = 300) {
      clearTimeout(timer)
      timer = setTimeout(() => void check(url), delayMs)
    },
    cancel() {
      clearTimeout(timer)
    },
  }
}
