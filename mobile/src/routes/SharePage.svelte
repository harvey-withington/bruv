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
  import { savePlainShare } from '../lib/shareCapture'
  import ShareFields from '../components/ShareFields.svelte'
  import ClipTargetControls from '../components/ClipTargetControls.svelte'
  import ClipOutcomePanel from '../components/ClipOutcomePanel.svelte'
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
  // svelte-ignore state_referenced_locally
  let title = $state(readParam('title'))
  // svelte-ignore state_referenced_locally
  let text = $state(readParam('text'))
  // svelte-ignore state_referenced_locally
  let url = $state(readParam('url'))

  let platform = $state('')
  let checking = $state(false)
  let saving = $state(false)
  let errorMsg = $state<string | null>(null)
  let outcome = $state<CaptureResult | null>(null)
  let prefs = $state<CapturePrefs>(loadCapturePrefs())

  const platformLabel = $derived(platform ? platform[0].toUpperCase() + platform.slice(1) : '')
  const canSave = $derived(platform ? !!url.trim() : !!title.trim())

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
  // Ask the server whether any capture plugin claims this URL. Debounced
  // so typing/pasting doesn't fire a call per keystroke; sequenced so a
  // slow answer for an older URL can't overwrite a newer one.

  let preflightSeq = 0

  async function preflight(raw: string) {
    const u = raw.trim()
    const mine = ++preflightSeq
    if (!title.trim()) title = deriveTitle()
    if (!u) {
      platform = ''
      checking = false
      return
    }
    checking = true
    try {
      const match = await repoRPC<string>('MatchCaptureURL', [u])
      if (mine !== preflightSeq) return
      platform = match ?? ''
    } catch {
      // Preflight is an enhancement, not a gate: an unreachable server
      // just means plain mode, and Save will surface the real error.
      if (mine === preflightSeq) platform = ''
    } finally {
      if (mine === preflightSeq) checking = false
    }
  }

  $effect(() => {
    const current = url
    const timer = setTimeout(() => void preflight(current), 300)
    return () => clearTimeout(timer)
  })

  // --- Saving ---------------------------------------------------------

  async function saveClip() {
    const u = url.trim()
    const opts = captureOptsFrom(prefs)
    const result = await repoRPC<CaptureResult>('CaptureFromURL', [u, opts])
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
      if (platform) await saveClip()
      else await savePlain()
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t(platform ? 'share.err_capture' : 'share.err_save')
    } finally {
      saving = false
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
    {#if !platform}
      <p class="intro">{t('share.intro')}</p>
    {/if}

    <ShareFields
      {platformLabel}
      bind:title
      bind:text
      bind:url
      disabled={saving}
      onPasteError={(msg) => (errorMsg = msg)}
    />

    {#if checking}
      <p class="checking">{t('share.checking')}</p>
    {/if}

    <ClipTargetControls
      {prefs}
      onChange={updatePrefs}
      disabled={saving}
      showPin={!!platform}
    />

    {#if errorMsg}
      <div class="error" role="alert">{errorMsg}</div>
    {/if}

    <div class="actions">
      <button type="button" class="ghost" onclick={cancel} disabled={saving}>
        {t('common.cancel')}
      </button>
      <button type="button" class="primary" onclick={save} disabled={saving || !canSave}>
        {#if platform}
          {saving ? t('share.clipping') : t('share.clip')}
        {:else}
          {saving ? t('share.saving') : t('share.save')}
        {/if}
      </button>
    </div>
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

  .error {
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .ghost,
  .primary {
    padding: 0.7rem 1.2rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
    border-color: var(--border);
  }
  .ghost:hover:not(:disabled),
  .ghost:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .primary {
    background: var(--accent);
    color: var(--bg);
    flex: 1;
    font-weight: 600;
  }
  .primary:hover:not(:disabled),
  .primary:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }

  .ghost:disabled,
  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
