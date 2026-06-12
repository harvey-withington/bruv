<script lang="ts">
  import { toasts, dismissToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import { X } from 'lucide-svelte'
</script>

<!-- Snackbar-style toasts: bottom-centered (phone reach zone), kept
     above the home-indicator via safe-area inset. Same shared store
     as desktop; only the presentation differs. -->
{#if toasts.list.length > 0}
  <div class="toast-container" role="status" aria-live="polite">
    {#each toasts.list as toast (toast.id)}
      <div class="toast toast--{toast.type}" class:dismissing={toast.dismissing}>
        <span class="toast-message">{toast.message}</span>
        <button class="toast-dismiss" onclick={() => dismissToast(toast.id)} aria-label={t('toast.dismiss')}><X size={16} /></button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    bottom: calc(1rem + env(safe-area-inset-bottom, 0px));
    left: 50%;
    transform: translateX(-50%);
    width: min(calc(100vw - 2rem), 480px);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    z-index: 9999;
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.7rem 0.85rem;
    border-radius: 10px;
    font-size: 0.875rem;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.3);
    pointer-events: auto;
    animation: toast-in 0.2s ease-out both;
  }

  .toast.dismissing {
    animation: toast-out 0.2s ease-in forwards;
  }

  @keyframes toast-in {
    from { opacity: 0; transform: translateY(8px) scale(0.96); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }

  @keyframes toast-out {
    from { opacity: 1; transform: translateY(0) scale(1); }
    to   { opacity: 0; transform: translateY(4px) scale(0.96); }
  }

  .toast--info    { background: var(--bg-elev-1); border: 1px solid var(--border); color: var(--text); }
  .toast--success { background: var(--success-bg); border: 1px solid var(--success-border); color: var(--success-text); }
  .toast--error   { background: var(--danger-bg); border: 1px solid var(--danger-border); color: var(--danger-text); }
  .toast--warning { background: var(--warn-bg); border: 1px solid var(--warn-border); color: var(--warn-text); }

  .toast-message { flex: 1; line-height: 1.4; }

  .toast-dismiss {
    background: none;
    border: none;
    color: inherit;
    opacity: 0.6;
    cursor: pointer;
    padding: 0.25rem;
    margin: -0.25rem;
    display: flex;
    flex-shrink: 0;
  }
  .toast-dismiss:active { opacity: 1; }
</style>
