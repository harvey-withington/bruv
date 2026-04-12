<script lang="ts">
  import { confirmState, resolveConfirm } from '../lib/confirm.svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap } from '../lib/actions'
  import { fade } from 'svelte/transition'

  function handleKeydown(e: KeyboardEvent) {
    if (!confirmState.visible) return
    if (e.key === 'Enter') { e.preventDefault(); resolveConfirm(true) }
    if (e.key === 'Escape') { e.preventDefault(); resolveConfirm(false) }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if confirmState.visible}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="confirm-backdrop" role="presentation" onclick={() => resolveConfirm(false)} out:fade={{ duration: 150 }}>
    <div class="confirm-dialog" role="alertdialog" aria-modal="true" tabindex="-1" onclick={(e) => e.stopPropagation()} use:focusTrap>
      <p class="confirm-message">{confirmState.message}</p>
      <div class="confirm-actions">
        <button class="btn-cancel" onclick={() => resolveConfirm(false)}>{t('common.cancel')}</button>
        <button class="btn-confirm" onclick={() => resolveConfirm(true)}>{t('common.confirm')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 99990;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .confirm-dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1.5rem;
    min-width: 300px;
    max-width: 420px;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .confirm-message {
    margin: 0 0 1.25rem;
    font-size: 0.9rem;
    color: var(--text-body);
    line-height: 1.5;
  }

  .confirm-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
  }

  .btn-cancel {
    padding: 0.4rem 0.9rem;
    border-radius: 5px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-body);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .btn-cancel:hover { border-color: var(--border-muted); background: var(--bg-surface); }

  .btn-confirm {
    padding: 0.4rem 0.9rem;
    border-radius: 5px;
    border: none;
    background: var(--danger-light, #f87171);
    color: #fff;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .btn-confirm:hover { opacity: 0.85; }
</style>
