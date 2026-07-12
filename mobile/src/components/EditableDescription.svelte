<script lang="ts">
  // Tap-to-edit multi-line markdown. Used for Card.description on
  // mobile. Renders markdown when displayed; swaps to an autosizing
  // textarea when the user taps in. Keyboard behaviour comes from the
  // shared inlineEdit action (mobile multiline variant per the keyboard
  // entry contract: Enter inserts a newline, blur or the ✓ Done button
  // commits, Escape cancels, Ctrl+Enter commits + closes the page).
  //
  // Same value-rendering shape as the desktop's DescriptionSection,
  // sized for thumb input — bigger tap target, larger initial textarea
  // height, no preview/edit toggle (the rendered markdown IS the
  // preview, and tapping it is the edit affordance).

  import { tick, getContext } from 'svelte'
  import { renderMarkdown } from '@shared/markdown'
  import { inlineEdit } from '@shared/inlineEdit'
  import { autoGrow } from '../lib/actions/autoGrow'
  import { tapGuardActive } from '../lib/tapGuard'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { t } from '../lib/i18n.svelte'
  import EditorDoneButton from './EditorDoneButton.svelte'

  let {
    value,
    placeholder = '',
    onSave,
  }: {
    value: string
    placeholder?: string
    onSave: (next: string) => void | Promise<void>
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let textareaEl: HTMLTextAreaElement | undefined = $state()

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  async function startEdit() {
    if (tapGuardActive()) return // tail of the ✓ tap retargeted here — not a real tap
    draft = value
    editing = true
    await tick()
    textareaEl?.focus() // the autoGrow action sizes on focus/input/viewport
  }

  function commit() {
    // Guarded: the ✓ Done tap and the action's blur-commit can both
    // land in one gesture — only the first may fire onSave.
    if (!editing) return
    editing = false
    if (draft !== value) onSave(draft)
  }

  function cancel() {
    editing = false
  }

</script>

{#if editing}
  <textarea
    bind:this={textareaEl}
    bind:value={draft}
    use:autoGrow
    use:inlineEdit={{ multiline: true, enterInsertsNewline: true, onCommit: commit, onCancel: cancel, scope: editScope }}
    placeholder={placeholder}
    class="editor"
    rows="4"
  ></textarea>
  <div class="editor-actions">
    <EditorDoneButton onDone={commit} />
  </div>
{:else if value}
  <button type="button" class="display" onclick={startEdit} aria-label={t('card.edit_description')}>
    <div class="prose">{@html renderMarkdown(value)}</div>
  </button>
{:else}
  <button type="button" class="display empty" onclick={startEdit} aria-label={t('card.edit_description')}>
    {placeholder}
  </button>
{/if}

<style>
  .display {
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.5rem 0.65rem;
    color: var(--text);
    font: inherit;
    cursor: text;
    text-align: left;
  }
  .display:hover,
  .display:focus-visible {
    border-color: var(--border);
    background: var(--bg-elev-1);
    outline: none;
  }
  .display.empty {
    color: var(--text-faint);
    font-style: italic;
    border-color: var(--border);
    border-style: dashed;
    padding: 0.65rem;
  }

  .editor-actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 0.35rem;
  }

  .editor {
    width: 100%;
    min-height: 6rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--accent);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    line-height: 1.55;
    padding: 0.65rem 0.75rem;
    resize: none;
    overflow-y: auto;
    outline: none;
  }

  .prose {
    font-size: 0.95rem;
    line-height: 1.55;
    color: var(--text);
  }
  .prose :global(p) { margin: 0 0 0.75rem; }
  .prose :global(p:last-child) { margin-bottom: 0; }
  .prose :global(a) { color: var(--accent); word-break: break-word; }
  .prose :global(code) {
    background: var(--bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .prose :global(pre) {
    background: var(--bg);
    padding: 0.65rem;
    border-radius: 6px;
    overflow-x: auto;
  }
  .prose :global(pre code) { background: transparent; padding: 0; }
  .prose :global(blockquote) {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }
  .prose :global(ul),
  .prose :global(ol) {
    padding-left: 1.25rem;
    margin: 0.5rem 0;
  }
</style>
