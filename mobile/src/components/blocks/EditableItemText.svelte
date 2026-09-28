<script lang="ts">
  // A single list/checklist item's text: renders inline markdown when idle,
  // swaps to a multi-line, auto-growing textarea on tap. Mobile's analog of
  // desktop's EditableText (inlineMarkdown mode) so checklist/list items
  // render markdown the same way on both surfaces. Owns its own draft +
  // edit state; the parent only sees committed text via onSave (or onEmpty
  // when the row ends up blank).
  //
  // Multi-line since 2026-09-20 (Harvey): editing a long item in a
  // single-line input on a phone meant the text scrolled out of view and
  // the caret was hard to place. The editor now wraps and grows, and takes
  // the mobile multiline contract (UI-CONVENTIONS §8): virtual Enter
  // inserts a newline, tap-away or ✓ Done commits, Ctrl+Enter commits and
  // closes the page, Escape/Back cancels. Adding rows stays the + button's
  // job.
  import { getContext, tick } from 'svelte'
  import { renderInline } from '@shared/markdown'
  import { inlineEdit } from '@shared/inlineEdit'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { autoGrow } from '../../lib/actions/autoGrow'
  import { tapGuardActive } from '../../lib/tapGuard'
  import { mentionable, followMention } from '../../lib/mentions.svelte'
  import EditorDoneButton from '../EditorDoneButton.svelte'

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
  let inputEl = $state<HTMLTextAreaElement | null>(null)

  // Keep the draft in sync with upstream text while idle; never clobber it
  // mid-edit (matches EditableText).
  $effect(() => { if (!editing) draft = text })

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  async function startEdit() {
    // The ✓ Done tap on the row above can retarget its tail here as the
    // rows reflow — ignore it (see lib/tapGuard.ts).
    if (tapGuardActive()) return
    draft = text
    editing = true
    await tick()
    inputEl?.focus()
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
</script>

{#if editing}
  <div class="editor">
    <!-- inlineEdit owns the contract: blur commits, Escape cancels (and is
         consumed so the card underneath stays open), Ctrl+Enter commits
         and closes the page, plain Enter inserts a newline. -->
    <textarea
      class="field"
      rows="1"
      bind:this={inputEl}
      bind:value={draft}
      use:autoGrow={{ minHeight: 0, maxHeight: 200 }}
      use:mentionable
      use:inlineEdit={{ multiline: true, enterInsertsNewline: true, onCommit: save, onCancel: cancel, scope: editScope }}
      enterkeyhint="enter"
      {placeholder}
    ></textarea>
    <EditorDoneButton onDone={() => inputEl?.blur()} />
  </div>
{:else}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span
    class="field display"
    class:done
    role="button"
    tabindex="0"
    onclick={(e) => { if (followMention(e.target)) { e.preventDefault(); e.stopPropagation(); return } if ((e.target as HTMLElement).closest('a')) return; void startEdit() }}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); void startEdit() } }}
  >
    {#if text}
      {@html renderInline(text)}
    {:else}
      <span class="placeholder">{placeholder}</span>
    {/if}
  </span>
{/if}

<style>
  .editor {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.3rem;
  }
  /* Shared shape so the display span and the textarea line up pixel-for-pixel. */
  .field {
    width: 100%;
    flex: 1;
    min-width: 0;
    box-sizing: border-box;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    line-height: 1.4;
    padding: 0.4rem 0.5rem;
  }
  .field:hover {
    border-color: var(--border);
  }
  /* NOT a flex item with a 0 basis: `.field { flex: 1 }` made flex-basis
     win over the height autoGrow sets, pinning the textarea to one line
     no matter how many newlines it held. Grows to ~8 lines, then scrolls. */
  textarea.field {
    flex: none;
    resize: none;
    overflow-y: auto;
    display: block;
    min-height: 0;
  }
  textarea.field:focus {
    outline: none;
    border-color: var(--accent);
    background: var(--bg-elev-1);
  }
  .display {
    cursor: text;
    display: block;
    word-break: break-word;
    white-space: pre-wrap;
  }
  .display.done {
    text-decoration: line-through;
    color: var(--text-muted);
  }
  .placeholder {
    color: var(--text-muted);
  }
</style>
