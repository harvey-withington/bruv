// What happens when you tap Save on the share page — the whole decision
// tree, out of the component so SharePage can stay a view.
//
// The tree is small but every branch is load-bearing, and each one exists
// because of a real defect:
//
//   no verdict yet     → wait for the preview, never guess plain
//                        (a YouTube link filed as a bare card, 2026-08-02)
//   preview says ask   → hand the choices to the user, capture nothing
//                        (the whole capture-options ruling, same day)
//   preview failed     → ask the server anyway; it is the authority
//   supported          → capture with the defaults, one tap
//   not supported      → a plain link card, said out loud beforehand
//
// Everything the server did OTHER than what was asked gets toasted; a
// half-done clip never navigates away like a success.

import { navigate, cardURL } from './router.svelte'
import { t } from './i18n.svelte'
import { showToast } from './toast.svelte'
import { captureOptsFrom, type CapturePrefs } from './capturePrefs'
import { captureOptsWith, type CaptureChoices } from './captureOptions'
import { savePlainShare, saveClipShare } from './shareCapture'
import type { Preflight } from './capturePreflight.svelte'
import type { CaptureOpts, CapturePreview, CaptureResult } from '@shared/types'

/** The page's live fields, read at the moment they're needed. */
export type ShareDraft = { title: string; text: string; url: string; prefs: CapturePrefs }

export type ShareFlow = {
  readonly saving: boolean
  readonly errorMsg: string | null
  /** Set once a clip comes back half-done: the page shows the pending
   *  panel instead of the form. */
  readonly outcome: CaptureResult | null
  /** The preview the Capture Options sheet is open about, or null. */
  readonly optionsPreview: CapturePreview | null
  setError(msg: string | null): void
  save(): Promise<void>
  openOptions(): Promise<void>
  confirmOptions(choices: CaptureChoices): Promise<void>
  closeOptions(): void
}

export function createShareFlow(
  preflight: Preflight,
  read: () => ShareDraft,
  setTitle: (title: string) => void,
): ShareFlow {
  let saving = $state(false)
  let errorMsg = $state<string | null>(null)
  let outcome = $state<CaptureResult | null>(null)
  // Snapshotted rather than read live from the preflight so a late
  // debounced check for a newer URL can't swap the facts out from under
  // the choices being made — and so the capture goes to the URL that was
  // previewed, not whatever ends up in the field.
  let optionsPreview = $state<CapturePreview | null>(null)

  async function saveClip(u: string, opts: CaptureOpts) {
    const report = await saveClipShare(u, opts)
    for (const w of report.warnings) {
      if (w.ms) showToast(w.text, 'warning', w.ms)
      else showToast(w.text, 'warning')
    }
    if (report.pending) {
      // Never navigate away like a success — the clip is half-done and
      // the user needs to know where to finish it.
      outcome = report.result
      return
    }
    showToast(t('share.clipped'), 'success')
    navigate(cardURL(report.result.cardId))
  }

  async function savePlain() {
    const { title, text, url, prefs } = read()
    const result = await savePlainShare({ title, text, url, prefs })
    if (result.deckFailed) showToast(t('share.deck_append_failed'), 'warning')
    navigate(cardURL(result.cardID))
  }

  /**
   * The preview couldn't run, so we don't know whether this URL is
   * clippable. Ask the server to capture it — it's the authority — and
   * fall back to a plain card only when it says it has no plugin.
   * Guessing "plain" here is the expensive mistake: a bare card for a
   * supported post is tedious to fix, an error message isn't.
   */
  async function saveClipOrPlain(u: string, opts: CaptureOpts) {
    try {
      await saveClip(u, opts)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      // Matches the server's own wording in supervisor/capture.go.
      if (msg.includes('no capture support')) await savePlain()
      else throw err
    }
  }

  /** Hold a preview about exactly this URL before acting on it. */
  async function ensurePreview(u: string) {
    if (!u || preflight.hasVerdictFor(u)) return
    preflight.cancel()
    await preflight.check(u)
  }

  function fail(err: unknown, clip: boolean) {
    errorMsg = err instanceof Error ? err.message : t(clip ? 'share.err_capture' : 'share.err_save')
  }

  return {
    get saving() {
      return saving
    },
    get errorMsg() {
      return errorMsg
    },
    get outcome() {
      return outcome
    },
    get optionsPreview() {
      return optionsPreview
    },
    setError(msg: string | null) {
      errorMsg = msg
    },
    closeOptions() {
      optionsPreview = null
    },

    async save() {
      if (saving) return
      saving = true
      errorMsg = null
      const { url, prefs } = read()
      const u = url.trim()
      try {
        await ensurePreview(u)
        const p = preflight.preview
        // The vault's own thresholds said this one is worth a decision.
        // Hand it over instead of deciding on the user's behalf.
        if (p?.shouldAsk) {
          optionsPreview = p
          return
        }
        if (p?.supported) await saveClip(p.url || u, captureOptsFrom(prefs))
        else if (u && preflight.failed) await saveClipOrPlain(u, captureOptsFrom(prefs))
        else await savePlain()
      } catch (err) {
        fail(err, !!preflight.platform)
      } finally {
        saving = false
      }
    },

    /** Open the sheet by hand — the choices are always available, not
     *  only when a trigger fires. */
    async openOptions() {
      if (saving) return
      const u = read().url.trim()
      if (!u) return
      saving = true
      errorMsg = null
      try {
        await ensurePreview(u)
        const p = preflight.preview
        // No preview means the check failed — give the reason, not a shrug.
        if (!p) throw new Error(preflight.error || t('share.err_preview'))
        optionsPreview = p
      } catch (err) {
        errorMsg = err instanceof Error ? err.message : t('share.err_preview')
      } finally {
        saving = false
      }
    },

    /** The user accepted the sheet: capture exactly what they chose, for
     *  the URL the preview was about. */
    async confirmOptions(choices: CaptureChoices) {
      const p = optionsPreview
      optionsPreview = null
      if (!p || saving) return
      saving = true
      errorMsg = null
      try {
        if (p.supported) await saveClip(p.url, captureOptsWith(read().prefs, choices, p))
        else {
          // Not clippable: it's a link card, and the sheet said so. The
          // edited title is the one thing worth carrying over.
          if (choices.title.trim()) setTitle(choices.title.trim())
          await savePlain()
        }
      } catch (err) {
        fail(err, p.supported)
      } finally {
        saving = false
      }
    },
  }
}
