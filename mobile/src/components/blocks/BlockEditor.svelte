<script lang="ts">
  // BlockEditor — dispatcher over the 17 block types. Replaces the
  // read-only BlockView. Each per-type component takes `block` and an
  // `onChange(next: Block)` callback; the parent (CardPage) holds the
  // canonical blocks array and persists changes via UpdateCardBlocks.
  //
  // Schema-drift fallback: unknown block.type values render with a
  // small placeholder rather than crashing — same approach as the old
  // BlockView, just narrower in scope now that all known types have
  // editors.

  import type { Block } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
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
  }: {
    block: Block
    /** Card ID — passed to ImageBlock for attachment uploads. */
    cardId: string
    onChange: (next: Block) => void
  } = $props()
</script>

<section class="block">
  {#if block.label && block.type !== 'divider' && block.type !== 'checkbox'}
    <h3 class="label">{block.label}</h3>
  {/if}

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

<style>
  .block {
    margin-bottom: 1.25rem;
  }

  .label {
    margin: 0 0 0.4rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
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
