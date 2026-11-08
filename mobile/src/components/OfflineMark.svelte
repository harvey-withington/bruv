<script lang="ts">
  // The "available offline" mark for a card: a small cloud-off icon whose
  // colour says how its copy stands — quiet when synced, accent with edits
  // waiting to sync, red when it needs a conflict choice or a retry.
  // Renders nothing for a card that isn't kept offline. With onActivate it
  // is a button (the card page header uses it to jump to the toggle).
  import { CloudOff } from 'lucide-svelte'
  import { offline } from '../lib/offline.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offlineMarkLabel, offlineMarkTone } from '@shared/offline/labels'

  let { cardId, size = 13, onActivate }: { cardId: string; size?: number; onActivate?: () => void } = $props()

  const summary = $derived(offline.summary(cardId))
</script>

{#if summary}
  {@const label = offlineMarkLabel(summary, t, offline.working)}
  {#if onActivate}
    <button type="button" class="offline-mark tap {offlineMarkTone(summary)}" title={label} aria-label={label} onclick={onActivate}>
      <CloudOff {size} />
    </button>
  {:else}
    <span class="offline-mark {offlineMarkTone(summary)}" title={label} aria-label={label} role="img">
      <CloudOff {size} />
    </span>
  {/if}
{/if}

<style>
  .offline-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: var(--text-faint);
  }
  .offline-mark.tap {
    background: transparent;
    border: none;
    padding: 0.35rem;
    min-width: 32px;
    min-height: 32px;
    border-radius: 6px;
    cursor: pointer;
    font: inherit;
  }
  .offline-mark.tap:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .offline-mark.pending { color: var(--accent); }
  /* Mobile's accent is already amber, so "needs you" goes red. */
  .offline-mark.attention { color: var(--danger-border); }
</style>
