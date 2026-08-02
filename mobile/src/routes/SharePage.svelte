<script lang="ts">
  import { Share2 } from 'lucide-svelte'
  import { navigate } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { loadCapturePrefs, saveCapturePrefs, type CapturePrefs } from '../lib/capturePrefs'
  import { seedShareParams } from '../lib/shareCapture'
  import { createPreflight } from '../lib/capturePreflight.svelte'
  import { createShareFlow } from '../lib/shareFlow.svelte'
  import ShareFields from '../components/ShareFields.svelte'
  import ClipTargetControls from '../components/ClipTargetControls.svelte'
  import ClipOutcomePanel from '../components/ClipOutcomePanel.svelte'
  import ShareActions from '../components/ShareActions.svelte'
  import CaptureOptionsSheet from '../components/CaptureOptionsSheet.svelte'

  // Landing page for shares from the Android system share sheet, and the
  // home "Clip a link" tile.
  //
  // Two modes, decided by the server: a URL a capture plugin claims goes
  // into CLIP MODE (BRUV resolves the post itself and builds the card),
  // anything else stays in PLAIN MODE (title/text/url → a plain card).
  // Both modes can append a slide to a sticky deck target.
  //
  // A capture the vault calls consequential (an oversized video, a big
  // gallery, a blocked or unsupported link — thresholds are the user's,
  // in Settings → Capture) opens the Capture Options sheet first instead
  // of deciding for them. Options is always reachable by hand.

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

  let prefs = $state<CapturePrefs>(loadCapturePrefs())

  const preflight = createPreflight()
  // Save/Options and everything they can end in live in lib/shareFlow —
  // the branches there are the load-bearing part, and the page shouldn't
  // bury them.
  const flow = createShareFlow(
    preflight,
    () => ({ title, text, url, prefs }),
    (next) => (title = next),
  )

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

  function save() {
    if (canSave) void flow.save()
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
  {#if flow.outcome}
    <ClipOutcomePanel
      platform={platformLabel || flow.outcome.platform}
      cardId={flow.outcome.cardId}
      slideAppended={flow.outcome.slideAppended}
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
      disabled={flow.saving}
      onPasteError={(msg) => flow.setError(msg)}
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
      disabled={flow.saving}
      showPin={!!platform}
    />

    <ShareActions
      errorMsg={flow.errorMsg}
      saving={flow.saving}
      {canSave}
      isClip={!!platform}
      showOptions={!!url.trim()}
      onCancel={cancel}
      onOptions={() => void flow.openOptions()}
      onSave={save}
    />
  {/if}
</main>

{#if flow.optionsPreview}
  <CaptureOptionsSheet
    preview={flow.optionsPreview}
    {prefs}
    onPrefsChange={updatePrefs}
    onConfirm={(choices) => void flow.confirmOptions(choices)}
    onClose={() => flow.closeOptions()}
  />
{/if}

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
