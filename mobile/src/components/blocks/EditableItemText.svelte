<script lang="ts">
  // A single list/checklist item's text: renders inline markdown when idle,
  // swaps to a text input on tap. Mobile's analog of desktop's EditableText
  // (inlineMarkdown mode) so checklist/list items render markdown the same
  // way on both surfaces. Owns its own draft + edit state; the parent only
  // sees committed text via onSave (or onEmpty when blurred blank).
  import { renderInline } from '@shared/markdown'

  let {
    text = '',
    done = false,
    placeholder = '',
    autoEdit = false,
    onSave,
    onEmpty,
    onEnter,
  }: {
    text?: string
    /** Strike through the rendered text (checklist done state). */
    done?: boolean
    placeholder?: string
    /** Start in edit mode + autofocus — used for freshly-added blank rows. */
    autoEdit?: boolean
    /** Commit non-empty, changed text. */
    onSave?: (text: string) => void
    /** Blurred while blank — the parent drops the row. */
    onEmpty?: () => void
    /** Enter pressed — the parent adds the next row. */
    onEnter?: () => void
  } = $props()

  // Initial-value capture is intended: autoEdit/text seed the starting
  // state, then this component owns them until the next commit.
  /* svelte-ignore state_referenced_locally */
  let editing = $state(autoEdit)
  /* svelte-ignore state_referenced_locally */
  let draft = $state(text)
  let inputEl = $state<HTMLInputElement | null>(null)

  // Keep the draft in sync with upstream text while idle; never clobber it
  // mid-edit (matches EditableText).
  $effect(() => { if (!editing) draft = text })

  $effect(() => { if (editing && inputEl) inputEl.focus() })

  function startEdit() {
    draft = text
    editing = true
  }

  function save() {
    if (!editing) return
    editing = false
    const v = draft.trim()
    if (v === '') { onEmpty?.(); return }
    if (v !== text) onSave?.(v)
  }

  function cancel() {
    editing = false
    draft = text
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      save()
      onEnter?.()
    } else if (e.key === 'Escape') {
      // Revert, and never let Escape bubble up to close the card.
      e.preventDefault()
      e.stopPropagation()
      cancel()
    }
  }
</script>

{#if editing}
  <input
    class="field"
    type="text"
    bind:this={inputEl}
    bind:value={draft}
    onblur={save}
    onkeydown={handleKeydown}
  />
{:else}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span
    class="field display"
    class:done
    role="button"
    tabindex="0"
    onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; startEdit() }}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); startEdit() } }}
  >
    {#if text}
      {@html renderInline(text)}
    {:else}
      <span class="placeholder">{placeholder}</span>
    {/if}
  </span>
{/if}

<style>
  /* Shared shape so the display span and the input line up pixel-for-pixel. */
  .field {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.4rem 0.5rem;
  }
  .field:hover {
    border-color: var(--border);
  }
  input.field:focus {
    outline: none;
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .display {
    cursor: text;
    display: block;
    word-break: break-word;
  }
  .display.done {
    text-decoration: line-through;
    color: var(--text-muted);
  }
  .placeholder {
    color: var(--text-muted);
  }
</style>
