<script lang="ts">
  import { toasts, dismissToast } from '../lib/toast.svelte'
  import { X } from 'lucide-svelte'
</script>

{#if toasts.list.length > 0}
  <div class="toast-container">
    {#each toasts.list as toast (toast.id)}
      <div class="toast toast--{toast.type}" class:dismissing={toast.dismissing}>
        <span class="toast-message">{toast.message}</span>
        <button class="toast-dismiss" onclick={() => dismissToast(toast.id)}><X size={14} /></button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    bottom: 1.5rem;
    right: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    z-index: 99999;
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 0.75rem;
    border-radius: 8px;
    font-size: 0.875rem;
    min-width: 240px;
    max-width: 400px;
    box-shadow: 0 4px 16px var(--shadow);
    pointer-events: auto;
    animation: toast-in 0.2s var(--ease-out) both;
  }

  .toast.dismissing {
    animation: toast-out 0.2s var(--ease-in-out) forwards;
  }

  @keyframes toast-in {
    from { opacity: 0; transform: translateY(8px) scale(0.96); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }

  @keyframes toast-out {
    from { opacity: 1; transform: translateY(0) scale(1); }
    to   { opacity: 0; transform: translateY(4px) scale(0.96); }
  }

  .toast--info    { background: var(--bg-elevated); border: 1px solid var(--border); color: var(--text-primary); }
  .toast--success { background: var(--success-bg); border: 1px solid var(--success-border); color: var(--success-text); }
  .toast--error   { background: var(--danger-bg); border: 1px solid var(--danger-border); color: var(--danger-text); }
  .toast--warning { background: var(--warning-bg); border: 1px solid var(--warning-border); color: var(--warning-text); }

  .toast-message { flex: 1; line-height: 1.4; }

  .toast-dismiss {
    background: none;
    border: none;
    color: inherit;
    opacity: 0.6;
    cursor: pointer;
    padding: 0;
    display: flex;
    flex-shrink: 0;
    transition: opacity var(--duration-fast);
  }
  .toast-dismiss:hover { opacity: 1; }
</style>
