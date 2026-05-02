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
  import { t } from '../lib/i18n.svelte'
  import { focusOnMount } from '../lib/actions'
  import type { Card } from '@shared/types'

  let {
    card,
    editingDescription = $bindable(),
    descriptionDraft = $bindable(),
    descTextareaEl = $bindable(),
    onKeydown,
    onInput,
    onBlur,
  }: {
    card: Card
    editingDescription: boolean
    descriptionDraft: string
    descTextareaEl: HTMLTextAreaElement | null
    onKeydown: (e: KeyboardEvent) => void
    onInput: (e: Event) => void
    onBlur: () => void
  } = $props()
</script>

<section class="section">
  <h3 class="section-title">{t('card.description')}</h3>
  {#if editingDescription}
    <textarea
      class="desc-textarea"
      use:focusOnMount
      bind:this={descTextareaEl}
      bind:value={descriptionDraft}
      onkeydown={onKeydown}
      oninput={onInput}
      onblur={onBlur}
      rows="4"
    ></textarea>
  {:else}
    <div class="desc-display"
      role="button"
      tabindex="0"
      onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingDescription = true }}
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingDescription = true } }}
      title={t('tooltip.edit_description')}
    >
      {#if card.description}
        <div class="markdown-content">{@html renderMarkdown(card.description)}</div>
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
