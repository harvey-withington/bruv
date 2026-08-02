<script lang="ts">
  import { untrack } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { askReasonText, defaultChoices, type CaptureChoices } from '../lib/captureOptions'
  import type { CapturePrefs as ClipTargetPrefs } from '../lib/capturePrefs'
  import type { CapturePreview } from '@shared/types'
  import BottomSheet from './BottomSheet.svelte'
  import CaptureMediaOptions from './CaptureMediaOptions.svelte'
  import ClipTargetControls from './ClipTargetControls.svelte'

  // Capture Options — the dialog the whole 2026-08-02 ruling is about.
  // Everything is pre-filled from the vault's defaults and one tap on
  // Capture accepts it; nothing here has to be touched. What it must
  // never do is hide a consequence: a blocked platform says it lands as a
  // pending clip, an unsupported link says it lands as a link card.
  //
  // There is deliberately NO "don't ask again" checkbox — when to ask is
  // a setting, and settings live in Settings.

  let {
    preview,
    prefs,
    onPrefsChange,
    onConfirm,
    onClose,
  }: {
    preview: CapturePreview
    /** Sticky deck/pin targets — the Destination section edits these. */
    prefs: ClipTargetPrefs
    onPrefsChange: (next: ClipTargetPrefs) => void
    onConfirm: (choices: CaptureChoices) => void
    onClose: () => void
  } = $props()

  // Deliberately a one-time seed: the sheet is mounted per preview, and
  // the user's edits must not be clobbered by a late re-check.
  const initial = untrack(() => defaultChoices(preview))
  let title = $state(initial.title)
  let video = $state(initial.video)
  let imageMode = $state(initial.imageMode)
  let confirming = $state(false)

  function confirm() {
    if (confirming) return
    confirming = true
    onConfirm({ title, video, imageMode })
  }
</script>

<BottomSheet
  title={t('capture.options_title')}
  subtitle={askReasonText(preview)}
  historyKey="captureOptions"
  {onClose}
>
  <div class="body">
    {#if preview.blocked}
      <!-- Honesty rule: a blocked capture is NOT a normal capture. Say
           what actually happens before they tap, not after. -->
      <p class="warn" role="status">
        {t('capture.blocked_note', { platform: preview.platform || t('capture.this_platform') })}
      </p>
    {:else if !preview.supported}
      <p class="warn" role="status">{t('capture.unsupported_note')}</p>
    {/if}

    <label class="field">
      <span class="field-label">{t('capture.field_title')}</span>
      <input bind:value={title} type="text" placeholder={t('capture.field_title_placeholder')} />
    </label>

    <CaptureMediaOptions {preview} bind:video bind:imageMode />

    <section class="section">
      <h3>{t('capture.destination')}</h3>
      <ClipTargetControls {prefs} onChange={onPrefsChange} showPin={preview.supported} />
    </section>

    <div class="actions">
      <button type="button" class="ghost" onclick={onClose}>{t('common.cancel')}</button>
      <button type="button" class="primary" onclick={confirm} disabled={confirming}>
        {confirming ? t('capture.capturing') : t('capture.capture')}
      </button>
    </div>
  </div>
</BottomSheet>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    overflow-y: auto;
    padding-bottom: 0.25rem;
  }

  .warn {
    margin: 0;
    padding: 0.55rem 0.7rem;
    border-radius: 8px;
    font-size: 0.82rem;
    line-height: 1.45;
    color: var(--text);
    background: color-mix(in srgb, #f59e0b 12%, var(--bg));
    border: 1px solid color-mix(in srgb, #f59e0b 35%, transparent);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field-label,
  .section h3 {
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
  }

  .field input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    padding: 0.65rem 0.85rem;
    outline: none;
  }
  .field input:focus {
    border-color: var(--accent);
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.25rem;
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
    touch-action: manipulation;
  }
  .ghost {
    background: transparent;
    color: var(--text-muted);
    border-color: var(--border);
  }
  .ghost:hover,
  .ghost:focus-visible {
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
  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
