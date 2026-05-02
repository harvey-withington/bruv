<script lang="ts">
  import { Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import QuickCaptureSheet from './QuickCaptureSheet.svelte'

  // Floating action button that opens the quick capture sheet. Fixed
  // bottom-right with safe-area padding so it stays clear of iOS
  // home indicators / Android nav gestures.

  let {
    onSaved,
  }: {
    /** Optional follow-up after a successful capture. The default is
     *  to do nothing — the sheet closing IS the confirmation. */
    onSaved?: (cardID: string) => void
  } = $props()

  let open = $state(false)
</script>

<button
  type="button"
  class="fab"
  onclick={() => (open = true)}
  aria-label={t('capture.fab_label')}
  title={t('capture.fab_label')}
>
  <Plus size={26} />
</button>

{#if open}
  <QuickCaptureSheet
    onClose={() => (open = false)}
    onSaved={(id) => onSaved?.(id)}
  />
{/if}

<style>
  .fab {
    position: fixed;
    right: calc(1.25rem + env(safe-area-inset-right));
    bottom: calc(1.25rem + env(safe-area-inset-bottom));
    width: 56px;
    height: 56px;
    border-radius: 50%;
    background: var(--accent);
    color: var(--bg);
    border: none;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 8px 20px rgba(0, 0, 0, 0.35), 0 2px 6px rgba(0, 0, 0, 0.2);
    transition: transform 120ms ease, filter 120ms ease;
    z-index: 50;
  }

  .fab:hover,
  .fab:focus-visible {
    filter: brightness(1.1);
    outline: none;
  }

  .fab:active {
    transform: scale(0.94);
  }
</style>
