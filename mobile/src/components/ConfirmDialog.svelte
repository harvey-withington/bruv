<script lang="ts">
  import { onMount } from 'svelte'
  import { fade, fly } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'

  // Generic confirmation dialog. Two variants — destructive (red
  // primary button) for delete-style actions, primary (accent) for
  // any other "are you sure" gate. Mirrors the desktop's
  // ConfirmDialog shape so call sites read similarly.

  let {
    title,
    body,
    confirmLabel,
    cancelLabel,
    destructive = false,
    onConfirm,
    onCancel,
  }: {
    title: string
    body: string
    confirmLabel?: string
    cancelLabel?: string
    destructive?: boolean
    onConfirm: () => void | Promise<void>
    onCancel: () => void
  } = $props()

  let confirming = $state(false)
  let confirmBtn: HTMLButtonElement | undefined = $state()

  onMount(() => {
    // Auto-focus the cancel button by default; the destructive primary
    // is intentionally NOT auto-focused so an accidental Enter doesn't
    // delete anything. Users explicitly tab/click to confirm.
    queueMicrotask(() => confirmBtn?.focus())
  })

  async function handleConfirm() {
    if (confirming) return
    confirming = true
    try {
      await onConfirm()
    } finally {
      // Caller is responsible for closing the dialog (typically by
      // setting their own open state to false). We just clear the
      // local in-flight flag.
      confirming = false
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onCancel()
    }
  }

  function onBackdrop(e: MouseEvent) {
    if (e.target === e.currentTarget) onCancel()
  }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  class="backdrop"
  role="presentation"
  onclick={onBackdrop}
  transition:fade={{ duration: 120 }}
>
  <div class="dialog" transition:fly={{ y: 20, duration: 180 }} role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-body">
    <h2 id="confirm-title">{title}</h2>
    <p id="confirm-body">{body}</p>

    <div class="actions">
      <button type="button" class="ghost" onclick={onCancel} disabled={confirming}>
        {cancelLabel ?? t('common.cancel')}
      </button>
      <button
        bind:this={confirmBtn}
        type="button"
        class={destructive ? 'danger' : 'primary'}
        onclick={handleConfirm}
        disabled={confirming}
      >
        {confirming ? t('common.working') : (confirmLabel ?? t('common.confirm'))}
      </button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 200;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }

  .dialog {
    width: 100%;
    max-width: 380px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1.5rem 1.25rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    box-shadow: 0 20px 40px rgba(0, 0, 0, 0.45);
  }

  h2 {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 600;
    color: var(--text);
  }

  p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.5;
    color: var(--text-muted);
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: 0.25rem;
  }

  .ghost,
  .primary,
  .danger {
    padding: 0.55rem 1rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid transparent;
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
  }
  .primary:hover:not(:disabled),
  .primary:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }

  .danger {
    background: #ef4444;
    color: #fff;
  }
  .danger:hover:not(:disabled),
  .danger:focus-visible:not(:disabled) {
    filter: brightness(1.1);
    outline: none;
  }

  .ghost:disabled,
  .primary:disabled,
  .danger:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>
