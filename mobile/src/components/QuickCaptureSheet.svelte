<script lang="ts">
  import { onMount } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import { X, Clipboard } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { inlineEdit } from '@shared/inlineEdit'
  import { EditScope } from '@shared/editScope'
  import { t } from '../lib/i18n.svelte'
  import type { Card } from '@shared/types'

  // Quick capture: tap FAB → slide-up sheet → type → Save creates an
  // Inbox card (no pin) → sheet closes. Designed for the "have an
  // idea, want to dump it before I forget" flow. Elaboration happens
  // later from the Inbox.

  let {
    onClose,
    onSaved,
  }: {
    onClose: () => void
    /** Optional callback after successful save. Receives the new card
     *  ID; the parent decides whether to navigate to it or stay put. */
    onSaved?: (cardID: string) => void
  } = $props()

  let title = $state('')
  let saving = $state(false)
  let errorMsg = $state<string | null>(null)
  let inputEl: HTMLTextAreaElement | undefined = $state()

  // Keyboard entry contract. The composer registers while focused, so
  // Escape first cancels the entry (clears the draft, drops focus) and
  // only a further Escape closes the sheet — it no longer discards a
  // non-empty draft and closes in one keystroke.
  const editScope = new EditScope()
  editScope.requestClose = () => onClose()

  onMount(() => {
    // Auto-focus on open. Mobile keyboards typically open on focus,
    // shaving a tap off the most common path.
    queueMicrotask(() => inputEl?.focus())
  })

  async function paste() {
    if (!navigator.clipboard?.readText) {
      errorMsg = t('capture.paste_unsupported')
      return
    }
    try {
      const text = await navigator.clipboard.readText()
      if (!text) return
      // Append to whatever's already typed so the paste doesn't wipe
      // partial input. With a space separator if the existing text
      // doesn't end on whitespace.
      title = title ? `${title.replace(/\s+$/, '')} ${text}` : text
      // Re-focus the textarea so the user can keep typing.
      queueMicrotask(() => inputEl?.focus())
    } catch (err) {
      // iOS Safari throws when permission is denied; Android Chrome
      // throws when the page isn't focused. Either way, the manual
      // long-press paste still works.
      errorMsg = t('capture.paste_failed')
    }
  }

  async function save() {
    const trimmed = title.trim()
    if (!trimmed || saving) return
    saving = true
    errorMsg = null
    try {
      // Empty cardType + no pin = orphan card lands in Inbox.
      const card = await repoRPC<Card>('CreateCard', ['', trimmed])
      onSaved?.(card.id)
      onClose()
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('capture.err_save')
    } finally {
      saving = false
    }
  }

  // Composer keyboard behaviour comes from the shared inlineEdit action,
  // mobile multiline variant (Enter inserts a newline; the Save button
  // saves). Cancel (Escape / Back) only dismisses the keyboard — it
  // must NOT destroy the draft: an accidental back-swipe mid-capture
  // would be data loss. closeOnCtrlEnter is off because save() already
  // closes the sheet on success — letting the action also call
  // requestClose would double-close AND close before a save error
  // could surface. Ctrl+Enter therefore still means "save + close",
  // via save() itself.
  function cancelCompose() {
    inputEl?.blur()
  }

  function onBackdrop(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

<svelte:window onkeydown={(e) => editScope.handleWindowKeydown(e)} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  class="backdrop"
  role="presentation"
  onclick={onBackdrop}
  transition:fade={{ duration: 150 }}
>
  <div class="sheet" transition:fly={{ y: 400, duration: 220 }} role="dialog" aria-modal="true" aria-labelledby="capture-title">
    <header class="sheet-header">
      <h2 id="capture-title">{t('capture.title')}</h2>
      <button type="button" class="close" onclick={onClose} aria-label={t('common.cancel')}>
        <X size={18} />
      </button>
    </header>

    <textarea
      bind:this={inputEl}
      bind:value={title}
      use:inlineEdit={{
        serial: true,
        multiline: true,
        enterInsertsNewline: true,
        closeOnCtrlEnter: false,
        onCommit: () => { void save() },
        onCancel: cancelCompose,
        scope: editScope,
      }}
      placeholder={t('capture.placeholder')}
      rows="3"
      disabled={saving}
      aria-label={t('capture.placeholder')}
    ></textarea>

    {#if errorMsg}
      <div class="error" role="alert">{errorMsg}</div>
    {/if}

    <div class="actions">
      <button
        type="button"
        class="secondary"
        onclick={paste}
        disabled={saving}
        aria-label={t('capture.paste')}
      >
        <Clipboard size={14} />
        {t('capture.paste')}
      </button>
      <button
        type="button"
        class="primary"
        onclick={save}
        disabled={saving || !title.trim()}
      >
        {saving ? t('capture.saving') : t('capture.save')}
      </button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 100;
    display: flex;
    align-items: flex-end;
    justify-content: center;
  }

  .sheet {
    width: 100%;
    max-width: 600px;
    background: var(--bg-elev-1);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    border-left: 1px solid var(--border);
    border-right: 1px solid var(--border);
    padding: 1.25rem 1.1rem 1.5rem;
    /* Respect the bottom safe area on iOS so the actions don't sit
       under the home indicator. */
    padding-bottom: calc(1.5rem + env(safe-area-inset-bottom));
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
  }

  .sheet-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .sheet-header h2 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
  }

  .close {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: inline-flex;
  }

  .close:hover,
  .close:focus-visible {
    color: var(--text);
    background: var(--bg);
    outline: none;
  }

  textarea {
    width: 100%;
    resize: none;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    padding: 0.75rem 0.85rem;
    line-height: 1.4;
    outline: none;
  }

  textarea:focus {
    border-color: var(--accent);
  }

  .error {
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.8rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: space-between;
    align-items: center;
  }

  .primary,
  .secondary {
    padding: 0.65rem 1.1rem;
    border-radius: 8px;
    font: inherit;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    border: 1px solid transparent;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .primary {
    background: var(--accent);
    color: var(--bg);
    flex: 1;
    justify-content: center;
  }

  .primary:hover:not(:disabled),
  .primary:focus-visible:not(:disabled) {
    filter: brightness(1.08);
    outline: none;
  }

  .primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .secondary {
    background: transparent;
    color: var(--text-muted);
    border-color: var(--border);
  }

  .secondary:hover:not(:disabled),
  .secondary:focus-visible:not(:disabled) {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
</style>
