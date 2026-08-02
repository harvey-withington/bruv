<script lang="ts">
  import { Share2 } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import {
    loadCapturePrefs,
    saveCapturePrefs,
    captureOptsFrom,
    type CapturePrefs,
  } from '../lib/capturePrefs'
  import { savePlainShare, seedShareParams } from '../lib/shareCapture'
  import { createPreflight } from '../lib/capturePreflight.svelte'
  import ShareFields from '../components/ShareFields.svelte'
  import ClipTargetControls from '../components/ClipTargetControls.svelte'
  import ClipOutcomePanel from '../components/ClipOutcomePanel.svelte'
  import ShareActions from '../components/ShareActions.svelte'
  import type { CaptureResult } from '@shared/types'

  // Landing page for shares from the Android system share sheet, and the
  // home "Clip a link" tile.
  //
  // Two modes, decided by the server: a URL a capture plugin claims goes
  // into CLIP MODE (BRUV resolves the post itself and builds the card),
  // anything else stays in PLAIN MODE (title/text/url → a plain card).
  // Both modes can append a slide to a sticky deck target.

  function readParam(name: string): string {
    if (typeof window === 'undefined') return ''
    return new URLSearchParams(window.location.search).get(name) ?? ''
  }

  // Captured once so the inputs are seeded; the user edits freely after.
  // seedShareParams lifts a text-borne URL into the url slot first —
  // otherwise apps that share the link inside EXTRA_TEXT never trigger
  // clip mode (see shareCapture.ts).
  const seeded = seedShareParams(readParam('title'), readParam('text'), readParam('url'))
  let title = $state(seeded.title)
  let text = $state(seeded.text)
  let url = $state(seeded.url)

  const preflight = createPreflight()
  let saving = $state(false)
  let errorMsg = $state<string | null>(null)
  let outcome = $state<CaptureResult | null>(null)
  let prefs = $state<CapturePrefs>(loadCapturePrefs())

  const platform = $derived(preflight.platform)
  const platformLabel = $derived(platform ? platform[0].toUpperCase() + platform.slice(1) : '')
  const canSave = $derived(platform ? !!url.trim() : !!title.trim())

  // A real URL that no capture plugin claims: the share still saves (as
  // a link card), but say so up front rather than letting the user
  // discover it from a bare card afterwards. Requires a verdict about
  // THIS url — a pending or failed check must never claim "unsupported".
  const unsupportedHost = $derived.by(() => {
    if (platform || preflight.checking || preflight.failed) return ''
    if (!url.trim() || !preflight.hasVerdictFor(url)) return ''
    try {
      return new URL(url.trim()).hostname.replace(/^www\./, '')
    } catch {
      return ''
    }
  })

  function updatePrefs(next: CapturePrefs) {
    prefs = next
    saveCapturePrefs(next)
  }

  /** Fallback title for a plain card: the URL's host, or the first line
   *  of the shared text. Real-world Android shares carry a URL but no
   *  title. */
  function deriveTitle(): string {
    const u = url.trim()
    if (u) {
      try {
        return new URL(u).hostname
      } catch {
        return u
      }
    }
    return text.trim().split('\n')[0].slice(0, 80)
  }

  // --- Mode preflight -------------------------------------------------
  //
  // Ask the server whether any capture plugin claims this URL (debounced
  // while typing/pasting). The verdict is tied to the URL it was made
  // about, so Save can tell "no plugin" apart from "not checked yet" —
  // see lib/capturePreflight.svelte.ts.

  $effect(() => {
    const current = url
    if (!title.trim()) title = deriveTitle()
    preflight.schedule(current)
    return () => preflight.cancel()
  })

  // --- Saving ---------------------------------------------------------

  async function saveClip() {
    const u = url.trim()
    const opts = captureOptsFrom(prefs)
    const result = await repoRPC<CaptureResult>('CaptureFromURL', [u, opts])
    // A bounced pin (accepted-types gate, stale category, unpinned deck
    // mirror) lands the card in the Inbox — say so, with the server's
    // reason, instead of celebrating a destination that was ignored.
    if (result.pinFailed) {
      showToast(t('share.pin_failed', { error: result.pinError ?? '' }), 'warning', 7000)
    }
    if (result.pending) {
      // Never navigate away like a success — the clip is half-done and
      // the user needs to know where to finish it.
      outcome = result
      return
    }
    showToast(t('share.clipped'), 'success')
    if (opts.includeInDeck && !result.slideAppended) {
      showToast(t('share.deck_append_failed'), 'warning')
    }
    navigate(cardURL(result.cardId))
  }

  async function savePlain() {
    const result = await savePlainShare({ title, text, url, prefs })
    if (result.deckFailed) showToast(t('share.deck_append_failed'), 'warning')
    navigate(cardURL(result.cardID))
  }

  async function save() {
    if (saving || !canSave) return
    saving = true
    errorMsg = null
    try {
      const u = url.trim()
      // Never let a fast Save beat the check: without a verdict about
      // THIS url we'd silently take the plain path and save a clippable
      // link as a bare card (a YouTube share did exactly that on
      // 2026-08-02). Await the answer first.
      if (u && !preflight.hasVerdictFor(u)) {
        preflight.cancel()
        await preflight.check(u)
      }
      if (platform) await saveClip()
      else if (u && preflight.failed) await saveClipOrPlain()
      else await savePlain()
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t(platform ? 'share.err_capture' : 'share.err_save')
    } finally {
      saving = false
    }
  }

  /** The check couldn't run, so we don't know whether this URL is
   *  clippable. Ask the server to capture it — it's the authority — and
   *  fall back to a plain card only when it says it has no plugin.
   *  Guessing "plain" here is the expensive mistake: a bare card for a
   *  supported post is tedious to fix, an error message isn't. */
  async function saveClipOrPlain() {
    try {
      await saveClip()
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      // Matches the server's own wording in supervisor/capture.go.
      if (msg.includes('no capture support')) await savePlain()
      else throw err
    }
  }

  function cancel() {
    navigate('/')
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={cancel}>
    <span aria-hidden="true">‹</span> {t('common.cancel')}
  </button>
  <span class="topbar-title"><Share2 size={14} /> {t('share.title')}</span>
  <span class="spacer"></span>
</header>

<main>
  {#if outcome}
    <ClipOutcomePanel
      platform={platformLabel || outcome.platform}
      cardId={outcome.cardId}
      slideAppended={outcome.slideAppended}
    />
  {:else}
    {#if !platform && !unsupportedHost}
      <p class="intro">{t('share.intro')}</p>
    {/if}

    <ShareFields
      {platformLabel}
      {unsupportedHost}
      bind:title
      bind:text
      bind:url
      disabled={saving}
      onPasteError={(msg) => (errorMsg = msg)}
    />

    {#if preflight.checking}
      <p class="checking">{t('share.checking')}</p>
    {:else if preflight.failed && url.trim()}
      <!-- Unknown ≠ unsupported: say we couldn't check, and Save will
           still try to capture rather than quietly making a link card. -->
      <p class="checking">{t('share.check_failed')}</p>
    {/if}

    <ClipTargetControls
      {prefs}
      onChange={updatePrefs}
      disabled={saving}
      showPin={!!platform}
    />

    <ShareActions
      {errorMsg}
      {saving}
      {canSave}
      isClip={!!platform}
      onCancel={cancel}
      onSave={save}
    />
  {/if}
</main>

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar-title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    justify-self: center;
  }

  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    justify-self: start;
  }

  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    max-width: 600px;
    margin: 0 auto;
    padding: 1.25rem 1rem 4rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .intro {
    margin: 0 0 0.5rem;
    color: var(--text-muted);
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .checking {
    margin: -0.35rem 0 0;
    color: var(--text-faint);
    font-size: 0.8rem;
  }

/* The error rail + Cancel/Save styles moved to ShareActions.svelte. */
</style>
