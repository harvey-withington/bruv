<script lang="ts">
  import { t } from '../lib/i18n.svelte'

  // The share page's commit bar: the inline error rail plus Cancel /
  // Save. Extracted so SharePage stays a view — the button label is the
  // only place the two modes differ down here (Clip vs Save), and the
  // error rail belongs with the action that produces it.

  let {
    errorMsg = null,
    saving = false,
    canSave = false,
    isClip = false,
    onCancel,
    onSave,
  }: {
    errorMsg?: string | null
    saving?: boolean
    canSave?: boolean
    /** Clip mode relabels the primary action — same button, honest verb. */
    isClip?: boolean
    onCancel: () => void
    onSave: () => void
  } = $props()
</script>

{#if errorMsg}
  <div class="error" role="alert">{errorMsg}</div>
{/if}

<div class="actions">
  <button type="button" class="ghost" onclick={onCancel} disabled={saving}>
    {t('common.cancel')}
  </button>
  <button type="button" class="primary" onclick={onSave} disabled={saving || !canSave}>
    {#if isClip}
      {saving ? t('share.clipping') : t('share.clip')}
    {:else}
      {saving ? t('share.saving') : t('share.save')}
    {/if}
  </button>
</div>

<style>
  .error {
    padding: 0.5rem 0.75rem;
    background: var(--danger-bg);
    color: var(--danger-text);
    border: 1px solid var(--danger-border);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .ghost,
  .primary {
    padding: 0.7rem 1.2rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
    touch-action: manipulation;
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
    border-color: var(--border);
  }
  .ghost:hover:not(:disabled),
  .ghost:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .primary {
    background: var(--accent);
    color: var(--bg);
    flex: 1;
    font-weight: 600;
  }
  .primary:hover:not(:disabled),
  .primary:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }

  .ghost:disabled,
  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
