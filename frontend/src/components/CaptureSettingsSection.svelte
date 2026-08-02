<script lang="ts">
  // Settings → Capture: vault-level capture defaults (what BRUV stores)
  // and the user's own definition of "worth asking about" (when it asks).
  //
  // Ruling that prompted this (Harvey, 2026-08-02): capture decisions —
  // video quality, when to prompt — are the user's, with per-VAULT
  // defaults (phone, browser clipper and desktop must all agree). See
  // plan/2026-08-02 capture options at capture time.md and
  // internal/repo/capture_prefs.go (source of truth for the shape and
  // the built-in default values mirrored below).
  //
  // Working copy is owned by the parent dialog (bind:prefs) and saved
  // through its existing Save button / error-toast flow — there is no
  // drag-and-drop or per-field validation here that would justify
  // self-persisting the way SlideTemplatePrefsSection does.
  import { t } from '../lib/i18n.svelte'
  import type { CapturePrefs, VideoMode, ImageMode, AskMode } from '@shared/types'

  let { prefs = $bindable() }: { prefs: CapturePrefs } = $props()
</script>

<p class="intro">{t('capture.vault_hint')}</p>

<div class="field-section-label">{t('capture.section_defaults')}</div>

<div class="field-row">
  <span class="field-label">{t('capture.video_mode')}</span>
  <div class="field-value">
    <select
      value={prefs.videoMode ?? 'fit'}
      onchange={(e) => (prefs.videoMode = e.currentTarget.value as VideoMode)}
    >
      <option value="fit">{t('capture.video_mode_fit')}</option>
      <option value="best">{t('capture.video_mode_best')}</option>
      <option value="smallest">{t('capture.video_mode_smallest')}</option>
      <option value="link">{t('capture.video_mode_link')}</option>
      <option value="skip">{t('capture.video_mode_skip')}</option>
    </select>
  </div>
</div>

{#if (prefs.videoMode ?? 'fit') === 'fit'}
  <div class="field-row">
    <span class="field-label">{t('capture.video_budget')}</span>
    <div class="field-value">
      <input
        type="number"
        min="1"
        max="10000"
        class="field-input field-input-short"
        value={prefs.videoBudgetMB ?? 50}
        oninput={(e) => (prefs.videoBudgetMB = Number(e.currentTarget.value))}
      />
      <span class="field-hint">{t('capture.video_budget_hint')}</span>
    </div>
  </div>
{/if}

<div class="field-row">
  <span class="field-label">{t('capture.image_mode')}</span>
  <div class="field-value">
    <select
      value={prefs.imageMode ?? 'all'}
      onchange={(e) => (prefs.imageMode = e.currentTarget.value as ImageMode)}
    >
      <option value="all">{t('capture.image_mode_all')}</option>
      <option value="first">{t('capture.image_mode_first')}</option>
      <option value="link">{t('capture.image_mode_link')}</option>
      <option value="skip">{t('capture.image_mode_skip')}</option>
    </select>
  </div>
</div>

<div class="field-section-label">{t('capture.section_ask')}</div>

<div class="field-row">
  <span class="field-label">{t('capture.ask_mode')}</span>
  <div class="field-value">
    <select
      value={prefs.askMode ?? 'triggers'}
      onchange={(e) => (prefs.askMode = e.currentTarget.value as AskMode)}
    >
      <option value="always">{t('capture.ask_mode_always')}</option>
      <option value="triggers">{t('capture.ask_mode_triggers')}</option>
      <option value="never">{t('capture.ask_mode_never')}</option>
    </select>
  </div>
</div>

{#if (prefs.askMode ?? 'triggers') === 'triggers'}
  <p class="intro triggers-hint">{t('capture.triggers_hint')}</p>

  <div class="field-row">
    <span class="field-label">{t('capture.trigger_video_size')}</span>
    <div class="field-value">
      <input
        type="number"
        min="0"
        max="10000"
        class="field-input field-input-short"
        value={prefs.triggers.videoOverMB ?? 0}
        oninput={(e) => (prefs.triggers.videoOverMB = Number(e.currentTarget.value))}
      />
      <span class="field-hint">{t('capture.trigger_video_size_hint')}</span>
    </div>
  </div>

  <div class="field-row">
    <span class="field-label">{t('capture.trigger_gallery_size')}</span>
    <div class="field-value">
      <input
        type="number"
        min="0"
        max="1000"
        class="field-input field-input-short"
        value={prefs.triggers.galleryOverCount ?? 0}
        oninput={(e) => (prefs.triggers.galleryOverCount = Number(e.currentTarget.value))}
      />
      <span class="field-hint">{t('capture.trigger_gallery_size_hint')}</span>
    </div>
  </div>

  <label class="field toggle-field">
    <span class="field-label">{t('capture.trigger_unsupported')}</span>
    <input type="checkbox" bind:checked={prefs.triggers.unsupportedUrl} />
  </label>

  <label class="field toggle-field">
    <span class="field-label">{t('capture.trigger_blocked')}</span>
    <input type="checkbox" bind:checked={prefs.triggers.blocked} />
  </label>
{/if}

<style>
  .intro {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin: 0 0 0.75rem;
    line-height: 1.45;
  }

  .triggers-hint {
    margin-top: -0.25rem;
  }

  .field-section-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    margin-top: 0.5rem;
    padding-bottom: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .toggle-field {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }

  .field-row {
    display: flex;
    align-items: baseline;
    gap: 0.75rem;
    margin-top: 0.6rem;
  }
  .field-row > .field-label {
    min-width: 120px;
    flex-shrink: 0;
  }

  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .field-value {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .field-hint {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .field-input,
  select {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
    box-sizing: border-box;
  }
  .field-input:focus,
  select:focus {
    border-color: var(--accent);
  }

  select {
    cursor: pointer;
    width: 100%;
  }

  .field-input-short {
    max-width: 100px;
  }

  input[type='checkbox'] {
    accent-color: var(--accent);
    width: 16px;
    height: 16px;
    cursor: pointer;
  }
</style>
