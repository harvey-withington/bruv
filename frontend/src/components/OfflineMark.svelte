<script lang="ts">
  // The "available offline" mark for a card: a small cloud-off icon whose
  // colour says how its copy stands — quiet when synced, accent with edits
  // waiting to sync, warning when it needs a conflict choice or a retry.
  // Renders nothing for a card that isn't kept offline. Used on project
  // cards (CardItem) and in the card dialog header.
  import { CloudOff } from 'lucide-svelte'
  import { offline } from '../lib/offline.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offlineMarkLabel, offlineMarkTone } from '@shared/offline/labels'

  let { cardId, size = 11 }: { cardId: string; size?: number } = $props()

  const summary = $derived(offline.summary(cardId))
</script>

{#if summary}
  {@const label = offlineMarkLabel(summary, t, offline.working)}
  <span class="offline-mark {offlineMarkTone(summary)}" title={label} aria-label={label} role="img">
    <CloudOff {size} />
  </span>
{/if}

<style>
  .offline-mark {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--text-muted);
  }
  .offline-mark.pending { color: var(--accent); }
  .offline-mark.attention { color: var(--warning); }
</style>
