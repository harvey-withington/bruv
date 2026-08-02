<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { AskMode, CapturePrefs, ImageMode, VideoMode } from '@shared/types'
  import RadioRows, { type RadioRow } from './RadioRows.svelte'

  // Settings → Capture: the defaults the Capture Options sheet opens on,
  // and the thresholds that decide when it opens at all.
  //
  // Per VAULT, not per device (Harvey, 2026-08-02): the phone and the
  // desktop clipper capture into the same vault, so they must agree.
  // "Consequential" is the user's definition here — a zero threshold or
  // an off toggle simply never asks.

  const FALLBACK: CapturePrefs = {
    videoMode: 'fit',
    videoBudgetMB: 50,
    imageMode: 'all',
    askMode: 'triggers',
    triggers: { videoOverMB: 50, galleryOverCount: 8, unsupportedUrl: true, blocked: true },
  }

  let prefs = $state<CapturePrefs>(FALLBACK)
  let loading = $state(true)
  let saving = $state(false)
  let errorMsg = $state<string | null>(null)

  const videoRows: RadioRow[] = [
    { key: 'fit', label: t('capture.mode_fit'), sub: t('capture.mode_fit_sub') },
    { key: 'best', label: t('capture.mode_best'), sub: t('capture.mode_best_sub') },
    { key: 'smallest', label: t('capture.mode_smallest') },
    { key: 'link', label: t('capture.mode_link'), sub: t('capture.video_link_sub') },
    { key: 'skip', label: t('capture.mode_skip') },
  ]

  const imageRows: RadioRow[] = [
    { key: 'all', label: t('capture.mode_images_all') },
    { key: 'first', label: t('capture.mode_images_first') },
    { key: 'link', label: t('capture.mode_images_link') },
    { key: 'skip', label: t('capture.mode_images_skip') },
  ]

  const askRows: RadioRow[] = [
    { key: 'always', label: t('capture.ask_always'), sub: t('capture.ask_always_sub') },
    { key: 'triggers', label: t('capture.ask_triggers'), sub: t('capture.ask_triggers_sub') },
    { key: 'never', label: t('capture.ask_never'), sub: t('capture.ask_never_sub') },
  ]

  onMount(async () => {
    try {
      const loaded = await repoRPC<CapturePrefs | null>('GetCapturePrefs')
      if (loaded) prefs = { ...FALLBACK, ...loaded, triggers: { ...loaded.triggers } }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('capture.err_load')
    } finally {
      loading = false
    }
  })

  /** Thresholds are integers; a blank or negative field means "never ask". */
  function num(raw: string): number {
    const n = Math.floor(Number(raw))
    return Number.isFinite(n) && n > 0 ? n : 0
  }

  async function save() {
    if (saving) return
    saving = true
    errorMsg = null
    try {
      // The budget bounds "best that fits" — zero would mean nothing ever
      // fits, which is a setting nobody means to choose.
      const budget = prefs.videoBudgetMB && prefs.videoBudgetMB > 0 ? prefs.videoBudgetMB : 1
      const next: CapturePrefs = { ...prefs, videoBudgetMB: budget }
      await repoRPC('SetCapturePrefs', [next])
      prefs = next
      showToast(t('capture.saved'), 'success')
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('capture.err_save')
    } finally {
      saving = false
    }
  }
</script>

{#if loading}
  <p class="hint">{t('common.loading')}</p>
{:else}
  <p class="hint">{t('capture.per_vault')}</p>

  <div class="block">
    <h3>{t('capture.video')}</h3>
    <RadioRows
      name="prefs-video"
      rows={videoRows}
      value={prefs.videoMode ?? 'fit'}
      onChange={(k) => (prefs.videoMode = k as VideoMode)}
      disabled={saving}
    />
    <label class="num-row">
      <span class="num-label">{t('capture.budget')}</span>
      <input
        type="number"
        inputmode="numeric"
        min="1"
        value={prefs.videoBudgetMB ?? 50}
        oninput={(e) => (prefs.videoBudgetMB = num(e.currentTarget.value))}
        disabled={saving}
      />
    </label>
    <p class="hint">{t('capture.budget_sub')}</p>
  </div>

  <div class="block">
    <h3>{t('capture.images')}</h3>
    <RadioRows
      name="prefs-images"
      rows={imageRows}
      value={prefs.imageMode ?? 'all'}
      onChange={(k) => (prefs.imageMode = k as ImageMode)}
      disabled={saving}
    />
  </div>

  <div class="block">
    <h3>{t('capture.ask')}</h3>
    <RadioRows
      name="prefs-ask"
      rows={askRows}
      value={prefs.askMode ?? 'triggers'}
      onChange={(k) => (prefs.askMode = k as AskMode)}
      disabled={saving}
    />
  </div>

  <div class="block" class:dimmed={prefs.askMode !== 'triggers'}>
    <h3>{t('capture.triggers')}</h3>
    <p class="hint">{t('capture.triggers_sub')}</p>
    <label class="num-row">
      <span class="num-label">{t('capture.trigger_video')}</span>
      <input
        type="number"
        inputmode="numeric"
        min="0"
        value={prefs.triggers.videoOverMB ?? 0}
        oninput={(e) => (prefs.triggers.videoOverMB = num(e.currentTarget.value))}
        disabled={saving}
      />
    </label>
    <label class="num-row">
      <span class="num-label">{t('capture.trigger_gallery')}</span>
      <input
        type="number"
        inputmode="numeric"
        min="0"
        value={prefs.triggers.galleryOverCount ?? 0}
        oninput={(e) => (prefs.triggers.galleryOverCount = num(e.currentTarget.value))}
        disabled={saving}
      />
    </label>
    <label class="check-row">
      <input
        type="checkbox"
        checked={prefs.triggers.unsupportedUrl === true}
        onchange={(e) => (prefs.triggers.unsupportedUrl = e.currentTarget.checked)}
        disabled={saving}
      />
      <span>{t('capture.trigger_unsupported')}</span>
    </label>
    <label class="check-row">
      <input
        type="checkbox"
        checked={prefs.triggers.blocked === true}
        onchange={(e) => (prefs.triggers.blocked = e.currentTarget.checked)}
        disabled={saving}
      />
      <span>{t('capture.trigger_blocked')}</span>
    </label>
  </div>

  {#if errorMsg}
    <p class="error" role="alert">{errorMsg}</p>
  {/if}

  <button type="button" class="save" onclick={save} disabled={saving}>
    {saving ? t('common.working') : t('common.save')}
  </button>
{/if}

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.75rem 0.85rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    margin-bottom: 0.5rem;
  }
  /* The thresholds only apply in "when it matters" mode — say so
     visually rather than hiding them and losing the settings. */
  .block.dimmed {
    opacity: 0.55;
  }

  h3 {
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
  }

  .hint {
    margin: 0 0 0.5rem;
    font-size: 0.78rem;
    line-height: 1.45;
    color: var(--text-muted);
  }
  .block .hint {
    margin: 0;
  }

  .num-row,
  .check-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-height: 44px;
    cursor: pointer;
  }

  .num-label {
    flex: 1;
    min-width: 0;
    font-size: 0.9rem;
    color: var(--text);
  }

  .num-row input {
    width: 6.5rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    padding: 0.45rem 0.6rem;
    outline: none;
    text-align: right;
  }
  .num-row input:focus {
    border-color: var(--accent);
  }

  .check-row input {
    width: 20px;
    height: 20px;
    accent-color: var(--accent);
    flex-shrink: 0;
    margin: 0;
  }
  .check-row span {
    font-size: 0.9rem;
    color: var(--text);
  }

  .error {
    margin: 0.4rem 0.25rem;
    color: #fca5a5;
    font-size: 0.82rem;
  }

  .save {
    width: 100%;
    padding: 0.75rem 1rem;
    border-radius: 10px;
    border: 1px solid transparent;
    background: var(--accent);
    color: var(--bg);
    font: inherit;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    touch-action: manipulation;
  }
  .save:hover:not(:disabled),
  .save:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }
  .save:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
