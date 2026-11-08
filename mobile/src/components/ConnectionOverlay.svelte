<script lang="ts">
  // Full-screen overlay shown when the server is unreachable (network
  // down / Tailscale tunnel down). Blurs the app behind it and offers a
  // manual Retry; recovery is also automatic (see connectivity.svelte.ts).
  // With cards kept on this device it also offers to work offline.
  import { connectivity, retryNow } from '../lib/connectivity.svelte'
  import { t } from '../lib/i18n.svelte'
  import { offline, workOffline } from '../lib/offline.svelte'
  import ErrorState from './ErrorState.svelte'
</script>

{#if connectivity.offline && !offline.working}
  <div class="connection-overlay" role="alertdialog" aria-modal="true" aria-label={t('error.offline')}>
    <div class="panel">
      <ErrorState message={t('error.offline')} onRetry={retryNow} />
      <!-- Always offered once a vault is set up for offline — with nothing
           kept yet it's disabled and says how to start (UI-CONVENTIONS §12.9). -->
      {#if offline.scope}
        <button type="button" class="work-offline" onclick={workOffline} disabled={offline.cards.length === 0}>
          {t('offline.work_offline', { count: offline.cards.length })}
        </button>
        {#if offline.cards.length === 0}
          <p class="hint">{t('offline.none_kept_hint')}</p>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .connection-overlay {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
    background: color-mix(in srgb, var(--bg) 55%, transparent);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }
  .panel {
    width: 100%;
    max-width: 360px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 14px;
    box-shadow: 0 18px 50px rgba(0, 0, 0, 0.5);
  }
  .work-offline {
    display: block;
    width: calc(100% - 2rem);
    margin: 0 1rem 1rem;
    min-height: 44px;
    border-radius: 10px;
    border: 1px solid var(--accent);
    background: transparent;
    color: var(--accent);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .work-offline:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .hint {
    margin: 0 1rem 1rem;
    font-size: 0.82rem;
    line-height: 1.4;
    color: var(--text-muted);
    text-align: center;
  }
</style>
