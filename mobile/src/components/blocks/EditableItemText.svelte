<script lang="ts">
  // A single list/checklist item's text: renders inline markdown when idle,
  // swaps to a text input on tap. Mobile's analog of desktop's EditableText
  // (inlineMarkdown mode) so checklist/list items render markdown the same
  // way on both surfaces. Owns its own draft + edit state; the parent only
  // sees committed text via onSave (or onEmpty when the row ends up blank).
  import { getContext } from 'svelte'
  import { renderInline } from '@shared/markdown'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'

  let {
    text = '',
    done = false,
    placeholder = '',
    autoEdit = false,
    onSave,
    onEmpty,
  }: {
    text?: string
    /** Strike through the rendered text (checklist done state). */
    done?: boolean
    placeholder?: string
    /** Start in edit mode + autofocus — used for freshly-added blank rows. */
    autoEdit?: boolean
    /** Commit non-empty, changed text. */
    onSave?: (text: string) => void
    /** Row ended up blank (blank commit, or cancel of a never-committed
     *  fresh row) — the parent drops the row. */
    onEmpty?: () => void
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

  // Keyboard entry contract: count as an active edit while editing so
  // the containing page's Escape doesn't close underneath us and
  // Ctrl+Enter commits this row too. Handlers stay hand-rolled because
  // plain Enter must commit WITHOUT closing the containing page, and
  // Escape must be consumed here (cancel this row only, not the card).
  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null
  $effect(() => {
    if (!editing || !editScope) return
    return editScope.register({ commit: save, cancel })
  })

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
    if (!editing) return
    editing = false
    draft = text
    // A row with no committed text is a just-added placeholder (the +
    // button's auto-edit spawn). Per the add-cancel ruling
    // (UI-CONVENTIONS §12.5) cancelling an add-flow leaves nothing
    // behind — hand it to the parent to drop, mirroring save()'s
    // blank-commit path.
    if (text.trim() === '') onEmpty?.()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      if (e.ctrlKey || e.metaKey) {
        // Contract: Ctrl+Enter commits and closes the containing page.
        e.preventDefault()
        e.stopPropagation()
        save()
        editScope?.requestClose?.()
        return
      }
      // Plain Enter (the mobile keyboard's ✓/Done tick) JUST commits —
      // no next-row advance. Adding rows is the + button's job; rapid-
      // entry chaining on tick surprised users (ruling 2026-07-10).
      e.preventDefault()
      save()
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
    enterkeyhint="done"
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
