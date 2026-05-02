<script lang="ts">
  // Tap-to-edit single-line text. Mobile-friendly: tapping the
  // displayed value swaps in an autofocused input. Enter or blur
  // commits, Escape cancels. Used for the card title; reusable for
  // other inline-editable fields as they appear.

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

  function startEdit() {
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

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancel()
    }
  }
</script>

{#if editing}
  <input
    bind:this={inputEl}
    bind:value={draft}
    onblur={commit}
    onkeydown={onKey}
    aria-label={ariaLabel}
    placeholder={placeholder}
    class="editable-input {className}"
  />
{:else}
  <button
    type="button"
    onclick={startEdit}
    aria-label={ariaLabel || `Edit ${value || placeholder}`}
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
