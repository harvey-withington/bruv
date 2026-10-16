<script lang="ts">
  /**
   * DescriptionSection — the card's primary description field.
   * Renders the inline-editable markdown text block that sits between
   * the fields grid and the block list inside the Details tab.
   *
   * State (editingDescription, descriptionDraft, descTextareaEl) is
   * bindable so the parent can keep using them for:
   *   - Ctrl+Enter save-and-close (parent's backdrop keydown)
   *   - card:updated guard (don't clobber mid-edit)
   *   - mention-picker integration (mutates descriptionDraft +
   *     setSelectionRange on descTextareaEl from CardDetail's @-handler)
   *
   * Event callbacks let the parent run its own save/input/blur
   * handlers — we don't own the save side effect here because the
   * save path is entangled with the parent's tracked()/savingCount.
   * Extracted from CardDetail to shrink the god-component.
   */
  import { renderMarkdown } from '@shared/markdown'
  import { taskToggleFromClick } from '@shared/markdownTasks'
  import { t } from '../lib/i18n.svelte'
  import { focusOnMount } from '../lib/actions'
  import { mentionable } from '../lib/mentions.svelte'
  import type { Card } from '@shared/types'

  let {
    card,
    editingDescription = $bindable(),
    descriptionDraft = $bindable(),
    descTextareaEl = $bindable(),
    onKeydown,
    onBlur,
    onToggleTask,
  }: {
    card: Card
    editingDescription: boolean
    descriptionDraft: string
    descTextareaEl: HTMLTextAreaElement | null
    onKeydown: (e: KeyboardEvent) => void
    onBlur: () => void
    /** A "- [ ]" checkbox was ticked in the rendered view; save this text. */
    onToggleTask: (next: string) => void
  } = $props()

  function handleDisplayClick(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (target.closest('a')) return
    const next = taskToggleFromClick(e, card.description)
    if (next !== null) { onToggleTask(next); return }
    if (target.closest('input')) return // refused toggle — don't open the editor either
    editingDescription = true
  }
</script>

<section class="section">
  <h3 class="section-title">{t('card.description')}</h3>
  {#if editingDescription}
    <textarea
      class="desc-textarea"
      use:focusOnMount
      use:mentionable
      bind:this={descTextareaEl}
      bind:value={descriptionDraft}
      onkeydown={onKeydown}
      onblur={onBlur}
      rows="4"
    ></textarea>
  {:else}
    <!-- press-still: the global press pulse shrinks this wide area enough to
         slide a task checkbox out from under the pointer (UI-CONVENTIONS §12.8). -->
    <div class="desc-display press-still"
      role="button"
      tabindex="0"
      onclick={handleDisplayClick}
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingDescription = true } }}
      title={t('tooltip.edit_description')}
    >
      {#if card.description}
        <div class="markdown-content">{@html renderMarkdown(card.description, { tasks: true })}</div>
      {:else}
        <p class="placeholder">{t('card.description_placeholder')}</p>
      {/if}
    </div>
  {/if}
</section>

<!--
  Styles for .section, .section-title, .desc-display, .desc-textarea,
  and .markdown-content / .placeholder live in CardDetail with :global
  scope so the same visual language applies to text blocks inside
  BlockItem too. Keeping those selectors here would scope them to this
  component only, re-breaking the text block's look.
-->
