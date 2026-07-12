<script lang="ts">
  // Tap-to-edit single-line text. Mobile-friendly: tapping the
  // displayed value swaps in an autofocused input. Keyboard behaviour
  // comes from the shared inlineEdit action (Enter/blur commit, Escape
  // cancels, Ctrl+Enter commits + closes the containing page). Used
  // for the card title; reusable for other inline-editable fields.

  import { getContext } from 'svelte'
  import { inlineEdit } from '@shared/inlineEdit'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { tapGuardActive } from '../lib/tapGuard'
  import { t } from '../lib/i18n.svelte'

  let {
    value,
    placeholder = '',
    ariaLabel = '',
    className = '',
    onSave,
  }: {
    value: string
    placeholder?: string
    ariaLabel?: string
    className?: string
    onSave: (next: string) => void | Promise<void>
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let inputEl: HTMLInputElement | undefined = $state()

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function startEdit() {
    if (tapGuardActive()) return // tail of a ✓ tap retargeted here after an editor collapsed
    draft = value
    editing = true
    queueMicrotask(() => inputEl?.focus())
  }

  function commit() {
    const trimmed = draft.trim()
    editing = false
    // Only fire onSave on a real change — avoids round-tripping the
    // server for taps that didn't actually edit anything.
    if (trimmed !== value) onSave(trimmed)
  }

  function cancel() {
    editing = false
  }
</script>

{#if editing}
  <input
    bind:this={inputEl}
    bind:value={draft}
    use:inlineEdit={{ onCommit: commit, onCancel: cancel, scope: editScope }}
    aria-label={ariaLabel}
    placeholder={placeholder}
    enterkeyhint="done"
    class="editable-input {className}"
  />
{:else}
  <button
    type="button"
    onclick={startEdit}
    aria-label={ariaLabel || t('editable.edit_label', { value: value || placeholder })}
    class="editable-display {className}"
    class:placeholder={!value}
  >
    {value || placeholder}
  </button>
{/if}

<style>
  .editable-input,
  .editable-display {
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    color: var(--text);
    font: inherit;
    padding: 0.25rem 0.5rem;
    border-radius: 6px;
    text-align: left;
    cursor: text;
  }

  .editable-input {
    background: var(--bg-elev-1);
    border-color: var(--accent);
    outline: none;
  }

  .editable-display:hover,
  .editable-display:focus-visible {
    border-color: var(--border);
    outline: none;
  }

  .editable-display.placeholder {
    color: var(--text-faint);
    font-style: italic;
  }
</style>
