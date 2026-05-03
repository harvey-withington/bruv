<script lang="ts">
  // BlockEditor — dispatcher over the 17 block types. Replaces the
  // read-only BlockView. Each per-type component takes `block` and an
  // `onChange(next: Block)` callback; the parent (CardPage) holds the
  // canonical blocks array and persists changes via UpdateCardBlocks.
  //
  // BlockEditor also owns the per-block management UI:
  //   - Tap label → inline rename, blur or Enter commits via onChange
  //   - Trash icon → fires onDelete; parent removes from card.blocks
  //
  // Schema-drift fallback: unknown block.type values render with a
  // small placeholder rather than crashing.

  import { tick, untrack } from 'svelte'
  import { Trash2, Pencil } from 'lucide-svelte'
  import type { Block } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import TextBlock from './TextBlock.svelte'
  import ChecklistBlock from './ChecklistBlock.svelte'
  import ListBlock from './ListBlock.svelte'
  import DividerBlock from './DividerBlock.svelte'
  import UrlBlock from './UrlBlock.svelte'
  import DateBlock from './DateBlock.svelte'
  import NumberBlock from './NumberBlock.svelte'
  import RatingBlock from './RatingBlock.svelte'
  import CheckboxBlock from './CheckboxBlock.svelte'
  import SelectBlock from './SelectBlock.svelte'
  import RadioBlock from './RadioBlock.svelte'
  import CheckboxGroupBlock from './CheckboxGroupBlock.svelte'
  import ImageBlock from './ImageBlock.svelte'
  import MediaBlock from './MediaBlock.svelte'
  import ProgressBlock from './ProgressBlock.svelte'
  import AlarmBlock from './AlarmBlock.svelte'
  import SurveyBlock from './SurveyBlock.svelte'

  let {
    block,
    cardId,
    onChange,
    onDelete,
  }: {
    block: Block
    /** Card ID — passed to ImageBlock for attachment uploads. */
    cardId: string
    onChange: (next: Block) => void
    /** Called when the user taps trash and confirms. Parent should
     *  remove this block from card.blocks and persist. */
    onDelete: () => void
  } = $props()

  // Block types that show the BlockEditor's own label header. Divider
  // owns its label rendering (with-or-without-label cases inside the
  // component); checkbox shows the label inline next to the toggle.
  // For everything else, BlockEditor renders an editable header.
  const ownsLabel = $derived(block.type === 'divider' || block.type === 'checkbox')

  // Inline label rename. untrack: seed once; startLabelEdit() refreshes
  // from the current block.label when the user actually opens the input.
  let labelEditing = $state(false)
  let labelDraft = $state(untrack(() => block.label ?? ''))
  let labelInputEl: HTMLInputElement | null = $state(null)

  async function startLabelEdit() {
    labelDraft = block.label ?? ''
    labelEditing = true
    await tick()
    labelInputEl?.focus()
    labelInputEl?.select()
  }

  function commitLabel() {
    labelEditing = false
    if (labelDraft === block.label) return
    onChange({ ...block, label: labelDraft })
  }

  function cancelLabel() {
    labelEditing = false
    labelDraft = block.label ?? ''
  }

  let confirmingDelete = $state(false)
</script>

<section class="block" class:has-label={!ownsLabel}>
  <header class="block-toolbar">
    {#if !ownsLabel}
      {#if labelEditing}
        <input
          bind:this={labelInputEl}
          class="label-input"
          type="text"
          value={labelDraft}
          oninput={(e) => (labelDraft = (e.currentTarget as HTMLInputElement).value)}
          onblur={commitLabel}
          onkeydown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              ;(e.currentTarget as HTMLInputElement).blur()
            } else if (e.key === 'Escape') {
              e.preventDefault()
              cancelLabel()
            }
          }}
          placeholder={t('block.label_placeholder')}
        />
      {:else}
        <button type="button" class="label-btn" onclick={startLabelEdit}>
          {block.label || t('block.label_placeholder')}
          <Pencil size={11} class="pencil" />
        </button>
      {/if}
    {:else}
      <span class="spacer"></span>
    {/if}
    <button
      type="button"
      class="trash-btn"
      onclick={() => (confirmingDelete = true)}
      aria-label={t('block.delete')}
      title={t('block.delete')}
    >
      <Trash2 size={13} />
    </button>
  </header>

  {#if block.type === 'text'}
    <TextBlock {block} {onChange} />
  {:else if block.type === 'checklist'}
    <ChecklistBlock {block} {onChange} />
  {:else if block.type === 'list'}
    <ListBlock {block} {onChange} />
  {:else if block.type === 'divider'}
    <DividerBlock {block} {onChange} />
  {:else if block.type === 'url'}
    <UrlBlock {block} {onChange} />
  {:else if block.type === 'date'}
    <DateBlock {block} {onChange} />
  {:else if block.type === 'number'}
    <NumberBlock {block} {onChange} />
  {:else if block.type === 'rating'}
    <RatingBlock {block} {onChange} />
  {:else if block.type === 'checkbox'}
    <CheckboxBlock {block} {onChange} />
  {:else if block.type === 'select'}
    <SelectBlock {block} {onChange} />
  {:else if block.type === 'radio'}
    <RadioBlock {block} {onChange} />
  {:else if block.type === 'checkbox_group'}
    <CheckboxGroupBlock {block} {onChange} />
  {:else if block.type === 'image'}
    <ImageBlock {block} {cardId} {onChange} />
  {:else if block.type === 'media'}
    <MediaBlock {block} />
  {:else if block.type === 'progress'}
    <ProgressBlock {block} {onChange} />
  {:else if block.type === 'alarm'}
    <AlarmBlock {block} {onChange} />
  {:else if block.type === 'survey'}
    <SurveyBlock {block} {onChange} />
  {:else}
    <p class="placeholder">
      {t('block.unsupported_on_mobile', { type: block.type })}
    </p>
  {/if}
</section>

{#if confirmingDelete}
  <ConfirmDialog
    title={t('block.delete')}
    body={t('block.delete_body', { name: block.label || t('block.unlabelled') })}
    confirmLabel={t('block.delete')}
    destructive
    onConfirm={() => { confirmingDelete = false; onDelete() }}
    onCancel={() => (confirmingDelete = false)}
  />
{/if}

<style>
  .block {
    margin-bottom: 1.25rem;
    position: relative;
  }

  .block-toolbar {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-bottom: 0.4rem;
    min-height: 1.6rem;
  }
  .spacer {
    flex: 1;
  }

  .label-btn {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 0.2rem 0.5rem;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    text-align: left;
    cursor: text;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .label-btn:hover,
  .label-btn:focus-visible {
    color: var(--text);
    border-color: var(--border);
    background: var(--bg-elev-1);
    outline: none;
  }
  :global(.pencil) {
    color: var(--text-faint);
    flex-shrink: 0;
  }

  .label-input {
    flex: 1;
    min-width: 0;
    background: var(--bg-elev-1);
    border: 1px solid var(--accent);
    border-radius: 6px;
    padding: 0.2rem 0.5rem;
    color: var(--text);
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    outline: none;
  }

  .trash-btn {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.3rem 0.4rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    min-width: 30px;
    min-height: 30px;
  }
  .trash-btn:hover,
  .trash-btn:focus-visible {
    color: #ef4444;
    border-color: rgba(239, 68, 68, 0.4);
    background: rgba(239, 68, 68, 0.08);
    outline: none;
  }

  .placeholder {
    margin: 0;
    padding: 0.5rem 0.75rem;
    background: var(--bg-elev-1);
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text-faint);
    font-size: 0.85rem;
    font-style: italic;
  }
</style>
