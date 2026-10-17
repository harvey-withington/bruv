<script lang="ts">
  // Thin bar at the top of every page while working offline: says
  // so, and leads back to the offline cards from wherever the user wandered
  // (a card page's Back walks the hierarchy, which isn't on the device).
  import { CloudOff } from 'lucide-svelte'
  import { offline } from '../lib/offline.svelte'
  import { route, navigate } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
</script>

{#if offline.working && route.current.name !== 'offline'}
  <button type="button" class="offline-bar" onclick={() => navigate('/offline')}>
    <CloudOff size={14} />
    <span>{t('offline.bar', { count: offline.cards.length })}</span>
  </button>
{/if}

<style>
  .offline-bar {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    width: 100%;
    min-height: 36px;
    padding: 0.35rem 0.75rem;
    padding-top: calc(0.35rem + env(safe-area-inset-top, 0px));
    border: none;
    border-bottom: 1px solid var(--warn-border);
    background: var(--warn-bg);
    color: var(--warn-text);
    font: inherit;
    font-size: 0.82rem;
    font-weight: 600;
    cursor: pointer;
  }
</style>
