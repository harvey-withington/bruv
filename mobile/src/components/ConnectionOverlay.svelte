<script lang="ts">
  // Full-screen overlay shown when the server is unreachable (network
  // down / Tailscale tunnel down). Blurs the app behind it and offers a
  // manual Retry; recovery is also automatic (see connectivity.svelte.ts).
  import { connectivity, retryNow } from '../lib/connectivity.svelte'
  import { t } from '../lib/i18n.svelte'
  import ErrorState from './ErrorState.svelte'
</script>

{#if connectivity.offline}
  <div class="connection-overlay" role="alertdialog" aria-modal="true" aria-label={t('error.offline')}>
    <div class="panel">
      <ErrorState message={t('error.offline')} onRetry={retryNow} />
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
</style>
