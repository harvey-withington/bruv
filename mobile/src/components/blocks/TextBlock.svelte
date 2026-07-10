<script lang="ts">
  import { untrack, getContext } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import { renderMarkdown } from '@shared/markdown'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'
  import { draftEdit } from '../../lib/actions/draftEdit'
  import EditorDoneButton from '../EditorDoneButton.svelte'

  // `mode` is owned by the parent BlockEditor so the edit/preview toggle
  // can live in the shared block toolbar (next to the trash button)
  // rather than taking its own row inside this component.
  let {
    block,
    mode = 'edit',
    onChange,
  }: {
    block: Block
    mode?: 'edit' | 'preview'
    onChange: (next: Block) => void
  } = $props()
  // untrack: draft is intentionally seeded from block.value once and
  // then owned by the textarea — we don't want it to clobber the user's
  // in-flight typing whenever the parent re-renders.
  let draft = $state(untrack(() => asString(block.value)))
  let textareaEl: HTMLTextAreaElement | null = $state(null)
  let lastSaved = $state(untrack(() => asString(block.value)))
  // Focused = an edit session is live → show the ✓ Done affordance.
  let editorActive = $state(false)

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function autoGrow() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
    // Cap at 60vh so a wall of text doesn't push the composer off screen.
    const cap = window.innerHeight * 0.6
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, cap)}px`
  }

  // Keyboard entry contract, mobile multiline variant (draftEdit
  // action): Enter inserts a newline; the draft is committed on blur /
  // ✓ Done / Ctrl+Enter and reverted on Escape — no keystroke-debounced
  // autosave, so cancel actually cancels.
  function commitDraft() {
    if (draft === lastSaved) return
    lastSaved = draft
    onChange(withValue(block, draft))
  }

  function revertDraft() {
    draft = lastSaved
  }

  // Fresh block prop (e.g. the component gets re-keyed to a different
  // block): drop any stale draft and re-seed from the new block.
  let seededID = untrack(() => block.id)
  $effect(() => {
    if (block.id === seededID) return
    seededID = block.id
    untrack(() => {
      draft = asString(block.value)
      lastSaved = draft
    })
  })

  // External-edit sync: when block.value changes from outside (SSE
  // event refetch on the page) AND the user isn't holding an in-flight
  // local edit, re-seed the draft. This is the path that lets a
  // desktop edit to the same card appear on the phone live.
  $effect(() => {
    const next = asString(block.value)
    if (next === lastSaved) return
    if (draft !== lastSaved) return // user has unsaved typing — don't clobber
    draft = next
    lastSaved = next
  })
</script>

<div class="text-block">
  {#if mode === 'edit'}
    <textarea
      bind:this={textareaEl}
      class="text-input"
      placeholder={t('block.text.placeholder')}
      bind:value={draft}
      oninput={autoGrow}
      use:draftEdit={{ multiline: true, enterInsertsNewline: true, onCommit: commitDraft, onCancel: revertDraft, scope: editScope }}
      rows="3"
      onfocus={() => { editorActive = true; autoGrow() }}
      onblur={() => (editorActive = false)}
    ></textarea>
    {#if editorActive}
      <div class="editor-actions">
        <!-- Commit via the action's blur path — one code path for
             tap-away and ✓ Done. -->
        <EditorDoneButton onDone={() => textareaEl?.blur()} />
      </div>
    {/if}
  {:else}
    <div class="preview">
      {#if draft.trim() === ''}
        <p class="empty">{t('block.text.placeholder')}</p>
      {:else}
        {@html renderMarkdown(draft)}
      {/if}
    </div>
  {/if}
</div>

<style>
  .text-block {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .text-input {
    width: 100%;
    min-height: 3.5rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    line-height: 1.5;
    padding: 0.65rem 0.75rem;
    resize: none;
    overflow: auto;
  }
  .text-input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .editor-actions {
    display: flex;
    justify-content: flex-end;
  }

  .preview {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.65rem 0.75rem;
    font-size: 0.95rem;
    line-height: 1.55;
    color: var(--text);
  }
  .preview :global(p) {
    margin: 0 0 0.6rem;
  }
  .preview :global(p:last-child) {
    margin-bottom: 0;
  }
  .preview :global(a) {
    color: var(--accent);
  }
  .preview :global(code) {
    background: var(--bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .preview :global(pre) {
    background: var(--bg);
    padding: 0.5rem 0.65rem;
    border-radius: 6px;
    overflow-x: auto;
  }
  .preview :global(pre code) {
    background: transparent;
    padding: 0;
  }
  .preview :global(blockquote) {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }

  .empty {
    color: var(--text-faint);
    font-style: italic;
    margin: 0;
  }
</style>
