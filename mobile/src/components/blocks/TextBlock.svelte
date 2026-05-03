<script lang="ts">
  import { untrack } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import { renderMarkdown } from '@shared/markdown'
  import { Eye, PencilLine } from 'lucide-svelte'
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  // Edit-by-default; tap "Preview" to swap. Mirrors the desktop text
  // block's behaviour but with a tap target sized for fingers.
  let mode = $state<'edit' | 'preview'>('edit')
  // untrack: draft is intentionally seeded from block.value once and
  // then owned by the textarea — we don't want it to clobber the user's
  // in-flight typing whenever the parent re-renders.
  let draft = $state(untrack(() => asString(block.value)))
  let textareaEl: HTMLTextAreaElement | null = $state(null)
  let lastSaved = $state(untrack(() => asString(block.value)))

  // Debounced save: 500ms after last keystroke, push through onChange.
  // Cleared on blur (immediate save) and on unmount.
  let timer: ReturnType<typeof setTimeout> | null = null

  function autoGrow() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
    // Cap at 60vh so a wall of text doesn't push the composer off screen.
    const cap = window.innerHeight * 0.6
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, cap)}px`
  }

  function commit(next: string) {
    if (next === lastSaved) return
    lastSaved = next
    onChange(withValue(block, next))
  }

  function handleInput(e: Event) {
    draft = (e.currentTarget as HTMLTextAreaElement).value
    autoGrow()
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      commit(draft)
    }, 500)
  }

  function handleBlur() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    commit(draft)
  }

  $effect(() => {
    return () => {
      if (timer) clearTimeout(timer)
    }
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
  <div class="toolbar">
    <button
      type="button"
      class="mode-btn"
      class:active={mode === 'edit'}
      onclick={() => (mode = 'edit')}
      aria-label={t('block.text.edit')}
    >
      <PencilLine size={14} />
      {t('block.text.edit')}
    </button>
    <button
      type="button"
      class="mode-btn"
      class:active={mode === 'preview'}
      onclick={() => {
        if (timer) {
          clearTimeout(timer)
          timer = null
        }
        commit(draft)
        mode = 'preview'
      }}
      aria-label={t('block.text.preview')}
    >
      <Eye size={14} />
      {t('block.text.preview')}
    </button>
  </div>

  {#if mode === 'edit'}
    <textarea
      bind:this={textareaEl}
      class="text-input"
      placeholder={t('block.text.placeholder')}
      value={draft}
      oninput={handleInput}
      onblur={handleBlur}
      rows="3"
      onfocus={autoGrow}
    ></textarea>
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

  .toolbar {
    display: inline-flex;
    gap: 0.25rem;
    align-self: flex-end;
  }

  .mode-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.75rem;
    padding: 0.3rem 0.5rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .mode-btn:hover,
  .mode-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }
  .mode-btn.active {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 50%, transparent);
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
