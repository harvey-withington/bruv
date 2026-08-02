// "Does BRUV have a capture plugin for this URL?" — the state machine
// behind the share page's clip-vs-plain decision.
//
// Extracted because getting this wrong is silent and expensive: on
// 2026-08-02 a YouTube link was saved as a plain link card because Save
// was tapped before the debounced check returned, and the page had no
// way to tell "no plugin" apart from "haven't checked yet" or "the check
// failed". A verdict is therefore always tied to the URL it was made
// about, and failure is its own state — never mistaken for a no-match.

import { repoRPC } from './auth'

export type Preflight = {
  /** Matched platform id, '' when there's no verdict or no match. */
  readonly platform: string
  readonly checking: boolean
  /** The check itself errored — we do NOT know whether it's clippable. */
  readonly failed: boolean
  /** True only when the current verdict was made about exactly this URL. */
  hasVerdictFor(url: string): boolean
  /** Run the check now and await it (callers that must not race). */
  check(url: string): Promise<void>
  /** Schedule a debounced check (typing/pasting). */
  schedule(url: string, delayMs?: number): void
  cancel(): void
}

export function createPreflight(): Preflight {
  let platform = $state('')
  let checking = $state(false)
  let failed = $state(false)
  let verdictFor = $state('')
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  async function check(raw: string): Promise<void> {
    const url = raw.trim()
    const mine = ++seq
    if (!url) {
      platform = ''
      verdictFor = ''
      failed = false
      checking = false
      return
    }
    checking = true
    try {
      const match = await repoRPC<string>('MatchCaptureURL', [url])
      if (mine !== seq) return // a newer URL already won
      platform = match ?? ''
      verdictFor = url
      failed = false
    } catch {
      if (mine !== seq) return
      // Unknown, NOT "unsupported" — the caller decides what to do with
      // that, and the UI must not claim the platform isn't supported.
      platform = ''
      verdictFor = ''
      failed = true
    } finally {
      if (mine === seq) checking = false
    }
  }

  return {
    get platform() {
      return platform
    },
    get checking() {
      return checking
    },
    get failed() {
      return failed
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
