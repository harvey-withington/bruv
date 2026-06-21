<script lang="ts">
  // Blocking error state for a failed data load (e.g. the server is
  // unreachable — Tailscale down). Shows the error message and, when an
  // onRetry is supplied, a button that re-runs the load in place so the
  // user doesn't have to back out of the repo and reopen it. The button
  // owns its own spinner state so callers just pass their loader.
  import { RefreshCw } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  let {
    message,
    onRetry,
    compact = false,
  }: {
    message: string
    onRetry?: () => void | Promise<void>
    // Compact: left-aligned inline row for nested contexts (e.g. a failed
    // stream/project load inside the browse tree) rather than the full
    // centered block used for a whole-page load failure.
    compact?: boolean
  } = $props()

  let retrying = $state(false)

  async function handleRetry() {
    if (!onRetry || retrying) return
    retrying = true
    try {
      await onRetry()
    } finally {
      retrying = false
    }
  }
</script>

<div class="error-state" class:compact role="alert">
  <p class="error-state-text">{message}</p>
  {#if onRetry}
    <button type="button" class="error-state-retry" onclick={handleRetry} disabled={retrying}>
      <RefreshCw size={15} class={retrying ? 'spinning' : ''} />
      <span>{retrying ? t('common.retrying') : t('common.retry')}</span>
    </button>
  {/if}
</div>

<style>
  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.85rem;
    text-align: center;
    padding: 2rem 1.25rem;
    color: var(--text-muted);
  }
  .error-state-text {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.45;
    max-width: 34ch;
  }

  /* Compact: inline row for nested/indented contexts. */
  .error-state.compact {
    flex-direction: row;
    align-items: center;
    justify-content: flex-start;
    flex-wrap: wrap;
    gap: 0.5rem 0.75rem;
    text-align: left;
    padding: 0.4rem 0;
  }
  .error-state.compact .error-state-text {
    font-size: 0.85rem;
  }
  .error-state.compact .error-state-retry {
    padding: 0.35rem 0.8rem;
    font-size: 0.78rem;
  }
  .error-state-retry {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    background: var(--accent);
    color: var(--bg);
    border: none;
    border-radius: 8px;
    padding: 0.55rem 1.15rem;
    font: inherit;
    font-weight: 600;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .error-state-retry:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .error-state-retry :global(.spinning) {
    animation: error-state-spin 0.8s linear infinite;
  }
  @keyframes error-state-spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
