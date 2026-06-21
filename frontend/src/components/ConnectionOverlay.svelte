<script lang="ts">
  // Full-window overlay shown when the active REMOTE backend becomes
  // unreachable mid-session (the in-process Local backend never triggers
  // this — see connectivity.svelte.ts). Blurs the app, auto-reconnects in
  // the background, and offers a manual retry plus a switch-to-Local
  // escape hatch (connection management bypasses the dead backend).
  import { connectivity, retryNow } from '../lib/connectivity.svelte'
  import { switchConnection, LOCAL_CONNECTION_ID, activeConnectionLabel } from '../lib/connections.svelte'
  import { t } from '../lib/i18n.svelte'

  let retrying = $state(false)

  async function handleRetry() {
    if (retrying) return
    retrying = true
    try {
      await retryNow()
    } finally {
      retrying = false
    }
  }
</script>

{#if connectivity.offline}
  <div class="overlay" role="alertdialog" aria-modal="true" aria-label={t('connection.lost_title')}>
    <div class="panel">
      <h2>{t('connection.lost_title')}</h2>
      <p>{t('connection.lost_body', { name: activeConnectionLabel() })}</p>
      <div class="actions">
        <button type="button" class="primary" onclick={handleRetry} disabled={retrying || connectivity.checking}>
          {retrying || connectivity.checking ? t('connection.reconnecting') : t('common.retry')}
        </button>
        <button type="button" class="secondary" onclick={() => switchConnection(LOCAL_CONNECTION_ID)}>
          {t('connection.use_local')}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 2000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
    background: color-mix(in srgb, var(--bg) 55%, transparent);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }
  .panel {
    width: 100%;
    max-width: 420px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1.75rem;
    text-align: center;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  }
  h2 {
    margin: 0 0 0.5rem;
    font-size: 1.15rem;
    color: var(--text-primary);
  }
  p {
    margin: 0 0 1.25rem;
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--text-secondary);
  }
  .actions {
    display: flex;
    gap: 0.6rem;
    justify-content: center;
  }
  button {
    font: inherit;
    font-weight: 600;
    font-size: 0.85rem;
    padding: 0.55rem 1.15rem;
    border-radius: 8px;
    cursor: pointer;
    border: 1px solid var(--border);
  }
  .primary {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .primary:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .secondary {
    background: transparent;
    color: var(--text-primary);
  }
  .secondary:hover {
    background: var(--bg);
  }
</style>
