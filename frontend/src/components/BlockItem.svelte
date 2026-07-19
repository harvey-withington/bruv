<script lang="ts">
  /**
   * BlockItem — one card block (any type). Handles:
   *   - drag handle + block wrapper (drag/drop is orchestrated by parent via callbacks)
   *   - label row (rename inline, action buttons, type-specific progress labels)
   *   - body: dispatches to the 14 block types (text, checklist, list, media,
   *     url, divider, select, number, date, rating, checkbox, radio,
   *     checkbox_group, image, progress, alarm, survey)
   *
   * Parent (CardDetail) retains all shared state and most mutation logic.
   * BlockItem is a presentational shell over the type-specific sub-components
   * (SelectBlock, DateBlock, etc.) that accept their own onUpdate callbacks.
   * Extracted from CardDetail to get the 2600-line god-component under
   * the 300-line limit — this file is the biggest single win in that split.
   */
  import { X, Trash2, GripVertical, Pencil, ChevronDown, ChevronRight, Maximize2, Minimize2, ListTree } from 'lucide-svelte'
  import { renderMarkdown } from '@shared/markdown'
  import { t } from '../lib/i18n.svelte'
  import { focusOnMount, inlineEdit } from '../lib/actions'
  import { getContext } from 'svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import { showToast } from '../lib/toast.svelte'
  import { UpdateCardBlocks, CreateCard, PinCard } from '@shared/api'
  import type { Block, BlockMeta, Card, ChecklistItem, ListItem, MediaItem, SurveyQuestion, SlideDeckValue } from '@shared/types'
  import EditableChecklist from './EditableChecklist.svelte'
  import EditableList from './EditableList.svelte'
  import MediaBlock from './MediaBlock.svelte'
  import SelectBlock from './SelectBlock.svelte'
  import NumberBlock from './NumberBlock.svelte'
  import DateBlock from './DateBlock.svelte'
  import RatingBlock from './RatingBlock.svelte'
  import CheckboxBlock from './CheckboxBlock.svelte'
  import RadioBlock from './RadioBlock.svelte'
  import CheckboxGroupBlock from './CheckboxGroupBlock.svelte'
  import ImageBlock from './ImageBlock.svelte'
  import ProgressBlock from './ProgressBlock.svelte'
  import AlarmBlock from './AlarmBlock.svelte'
  import SurveyBlock from './SurveyBlock.svelte'
  import SlideDeckBlock from './SlideDeckBlock.svelte'

  let {
    block,
    blockIdx,
    card,
    cardId,
    currentCategoryId,
    // Shared editing state — bindable so the parent keeps a single
    // source of truth (only one block can be editing at a time).
    editingBlockId = $bindable(),
    editingBlockLabelId = $bindable(),
    blockLabelDraft = $bindable(),
    blockDrafts = $bindable(),
    collapsedBlocks,
    expandedTextBlocks,
    // Read-only state from parent
    draggingBlockId,
    mentionVisible,
    textBlockOverflows,
    // Element refs — bindable so parent can focus/measure them
    blockTextareaEls = $bindable(),
    textBlockEls = $bindable(),
    // Callbacks — all block-mutating operations go through the parent
    // so CardDetail's save-tracking and optimistic-update paths stay
    // authoritative.
    tracked,
    onUpdated,
    onDragStart,
    onDragEnd,
    onDragOver,
    onKeydown,
    onToggleCollapse,
    onRenameLabel,
    onOpenOptionsEditor,
    onClearValue,
    onDelete,
    onTextKeydown,
    onTextInput,
    onSaveText,
    onSaveUrl,
    onToggleTextExpand,
    isBlockEmpty,
  }: {
    block: Block
    blockIdx: number
    card: Card | null
    cardId: string
    currentCategoryId: string | null | undefined
    editingBlockId: string | null
    editingBlockLabelId: string | null
    blockLabelDraft: string
    blockDrafts: Record<string, string>
    collapsedBlocks: Set<string>
    expandedTextBlocks: Set<string>
    draggingBlockId: string | null
    mentionVisible: boolean
    textBlockOverflows: Set<string>
    blockTextareaEls: Record<string, HTMLTextAreaElement | null>
    textBlockEls: Record<string, HTMLElement | null>
    tracked: <T>(p: Promise<T>) => Promise<T>
    onUpdated?: () => void
    onDragStart: (e: DragEvent, block: Block) => void
    onDragEnd: () => void
    onDragOver: (e: DragEvent, block: Block, idx: number) => void
    onKeydown: (e: KeyboardEvent, blockIdx: number) => void
    onToggleCollapse: (blockId: string) => void
    onRenameLabel: (blockId: string) => void
    onOpenOptionsEditor: (block: Block) => void
    onClearValue: (block: Block) => void
    onDelete: (blockId: string) => void
    onTextKeydown: (e: KeyboardEvent, blockId: string) => void
    onTextInput: (e: Event, blockId: string) => void
    onSaveText: (blockId: string) => void
    onSaveUrl: (blockId: string) => void
    onToggleTextExpand: (blockId: string) => void
    isBlockEmpty: (block: Block) => boolean
  } = $props()

  // Dialog edit scope — the inlineEdit actions below register their
  // in-flight edits here (Escape layering, Ctrl+Enter commit+close,
  // silent-reload guard). Text blocks register in CardBlocks instead.
  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  // One guarded save for every widget-style block (select, number,
  // date, rating, checkbox, radio, checkbox_group, image, progress,
  // alarm, survey). These used to fire tracked(UpdateCardBlocks(...))
  // without await/catch — on RPC failure the UI silently kept a value
  // the disk never got. Rolls the block back and toasts instead.
  async function commitBlock(target: Block, val: Block['value'], newMeta?: BlockMeta) {
    if (!card) return
    const prevValue = target.value
    const prevMeta = target.meta
    target.value = val
    if (newMeta) target.meta = { ...target.meta, ...newMeta }
    try {
      await tracked(UpdateCardBlocks(cardId, card.blocks))
      onUpdated?.()
    } catch (e) {
      // Roll back only if no newer local write landed while this save
      // was in flight — otherwise a slow failing save would clobber the
      // user's more recent value with the older snapshot.
      if (target.value === val) {
        target.value = prevValue
        target.meta = prevMeta
      }
      showToast(t('error.save_failed'), 'error')
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="block-wrapper"
  class:block-collapsed={collapsedBlocks.has(block.id)}
  class:block-dragging={draggingBlockId === block.id}
  ondragover={(e) => onDragOver(e, block, blockIdx)}
  onkeydown={(e) => onKeydown(e, blockIdx)}
  data-block-id={block.id}
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class="block-drag-handle"
    role="presentation"
    tabindex={-1}
    draggable={true}
    ondragstart={(e) => onDragStart(e, block)}
    ondragend={onDragEnd}
    title={t('tooltip.drag_block')}
  ><GripVertical size={14} /></div>

  <section class="section block-content">
    <!-- Editable block label with collapse toggle -->
    {#if editingBlockLabelId === block.id}
      <input
        class="block-label-input"
        use:focusOnMount={true}
        bind:value={blockLabelDraft}
        use:inlineEdit={{ onCommit: () => onRenameLabel(block.id), onCancel: () => { editingBlockLabelId = null }, scope: editScope }}
      />
    {:else}
      <div class="section-title block-label-row action-reveal-parent">
        {#if block.type !== 'divider'}
          <button class="block-collapse-btn" onclick={() => onToggleCollapse(block.id)} title={collapsedBlocks.has(block.id) ? t('tooltip.expand_block') : t('tooltip.collapse_block')}>
            {#if collapsedBlocks.has(block.id)}<ChevronRight size={14} />{:else}<ChevronDown size={14} />{/if}
          </button>
        {/if}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <span class="block-label-text" tabindex={0} role="button"
          onclick={() => { editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' } }}
        >{block.label || block.key || block.type}</span>
        {#if block.type === 'checklist'}
          {@const items = (Array.isArray(block.value) ? block.value : []) as ChecklistItem[]}
          {#if items.length > 0}
            <span class="checklist-progress">{items.filter(c => c.done).length}/{items.length}</span>
          {/if}
        {:else if block.type === 'list'}
          {@const items = Array.isArray(block.value) ? block.value : []}
          {#if items.length > 0}
            <span class="checklist-progress">{items.length}</span>
          {/if}
        {:else if block.type === 'media'}
          {@const items = Array.isArray(block.value) ? block.value : []}
          {#if items.length > 0}
            <span class="checklist-progress">{items.length}</span>
          {/if}
        {/if}
        <span class="block-actions">
          {#if block.type === 'select' || block.type === 'radio' || block.type === 'checkbox_group'}
            <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); onOpenOptionsEditor(block) }} title={t('block.edit_options')}><ListTree size={11} /></button>
          {/if}
          {#if block.type !== 'divider' && !isBlockEmpty(block)}
            <button class="block-action-btn action-reveal" onclick={(e) => { e.stopPropagation(); onClearValue(block) }} title={t('tooltip.clear_block')}><X size={11} /></button>
          {/if}
          <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} title={t('tooltip.rename_block')}><Pencil size={11} /></button>
          <button class="block-action-btn action-reveal action-reveal--danger" onclick={(e) => { e.stopPropagation(); onDelete(block.id) }} title={t('tooltip.delete_block')}><Trash2 size={11} /></button>
        </span>
      </div>
    {/if}

    <!-- Block body (hidden when collapsed, except divider) -->
    {#if !collapsedBlocks.has(block.id) || block.type === 'divider'}
      <div class="block-body">
        {#if block.type === 'text'}
          {#if editingBlockId === block.id}
            <textarea
              class="desc-textarea"
              use:focusOnMount
              bind:this={blockTextareaEls[block.id]}
              bind:value={blockDrafts[block.id]}
              onkeydown={(e) => onTextKeydown(e, block.id)}
              oninput={(e) => onTextInput(e, block.id)}
              onblur={() => { if (!mentionVisible) onSaveText(block.id) }}
              rows="4"
            ></textarea>
          {:else}
            <div class="text-scroll-wrap">
              <div
                class="desc-display"
                class:text-scroll={!expandedTextBlocks.has(block.id)}
                bind:this={textBlockEls[block.id]}
                role="button"
                tabindex={0}
                onclick={(e) => { if ((e.target as HTMLElement).closest('a') || (e.target as HTMLElement).closest('.text-expand-btn')) return; editingBlockId = block.id }}
                onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockId = block.id } }}
                title={t('tooltip.edit_description')}
              >
                {#if block.value}
                  <div class="markdown-content">{@html renderMarkdown(String(block.value))}</div>
                {:else}
                  <p class="placeholder">{t('block.text_placeholder')}</p>
                {/if}
              </div>
              {#if textBlockOverflows.has(block.id) && !expandedTextBlocks.has(block.id)}
                <div class="text-scroll-gradient"></div>
              {/if}
            </div>
            {#if textBlockOverflows.has(block.id) && !expandedTextBlocks.has(block.id)}
              <button class="text-expand-btn" onclick={() => onToggleTextExpand(block.id)}>
                <Maximize2 size={11} /> {t('block.scroll_expand')}
              </button>
            {/if}
            {#if expandedTextBlocks.has(block.id)}
              <button class="text-expand-btn" onclick={() => onToggleTextExpand(block.id)}>
                <Minimize2 size={11} /> {t('block.collapse')}
              </button>
            {/if}
          {/if}

        {:else if block.type === 'checklist'}
          <EditableChecklist
            items={Array.isArray(block.value) ? block.value as ChecklistItem[] : []}
            onUpdate={async (updated) => {
              if (!card) return
              // Mutate block.value in place so Svelte 5's $state proxy on
              // card.blocks sees the change and the UI re-renders. Building
              // a fresh blocks array via map() would persist but leave the
              // parent's card state stale — toggles would save but not
              // visibly update until the card was reopened.
              block.value = updated
              try {
                await tracked(UpdateCardBlocks(cardId, card.blocks))
                onUpdated?.()
              } catch (e) { showToast(t('error.save_failed'), 'error') }
            }}
            onPromote={async (text) => {
              try {
                const newCard = await CreateCard(card?.type || 'task', text)
                if (newCard && currentCategoryId) {
                  await PinCard(newCard.id, currentCategoryId)
                }
                showToast(t('card.promoted_to_card', { title: text }), 'success')
                onUpdated?.()
              } catch (e) { showToast(t('error.save_failed'), 'error') }
            }}
          />

        {:else if block.type === 'list'}
          <EditableList
            items={Array.isArray(block.value) ? block.value as ListItem[] : []}
            onUpdate={async (updated) => {
              if (!card) return
              block.value = updated
              try {
                await tracked(UpdateCardBlocks(cardId, card.blocks))
                onUpdated?.()
              } catch (e) { showToast(t('error.save_failed'), 'error') }
            }}
          />

        {:else if block.type === 'media'}
          <MediaBlock
            items={Array.isArray(block.value) ? block.value as MediaItem[] : []}
            onUpdate={async (updated) => {
              if (!card) return
              block.value = updated
              try {
                await tracked(UpdateCardBlocks(cardId, card.blocks))
                onUpdated?.()
              } catch (e) { showToast(t('error.save_failed'), 'error') }
            }}
          />

        {:else if block.type === 'url'}
          {#if editingBlockId === block.id}
            <input
              class="block-url-input"
              type="url"
              use:focusOnMount
              bind:value={blockDrafts[block.id]}
              placeholder={t('block.url_placeholder')}
              use:inlineEdit={{
                onCommit: () => onSaveUrl(block.id),
                onCancel: () => { blockDrafts[block.id] = String(block.value ?? ''); editingBlockId = null },
                scope: editScope,
              }}
            />
          {:else if block.value}
            <div class="block-url-row">
              <a href={String(block.value)} target="_blank" rel="noopener" class="block-link">{String(block.value)}</a>
              <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); editingBlockId = block.id; blockDrafts[block.id] = String(block.value ?? '') }} title={t('tooltip.edit_url')}><Pencil size={11} /></button>
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <span class="block-url-empty" role="button" tabindex={0}
              onclick={() => { editingBlockId = block.id; blockDrafts[block.id] = '' }}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockId = block.id; blockDrafts[block.id] = '' } }}
            >{t('block.url_placeholder')}</span>
          {/if}

        {:else if block.type === 'divider'}
          <hr class="block-divider" />

        {:else if block.type === 'select'}
          <SelectBlock
            value={block.value as string | string[]}
            meta={block.meta || { options: [] }}
            onUpdate={(val, newMeta) => commitBlock(block, val, newMeta)}
          />
        {:else if block.type === 'number'}
          <NumberBlock
            value={block.value as number | null}
            meta={block.meta || {}}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'date'}
          <DateBlock
            value={block.value as string | null}
            meta={block.meta || {}}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'rating'}
          <RatingBlock
            value={(block.value as number) || 0}
            meta={block.meta || {}}
            onUpdate={(val) => commitBlock(block, val)}
          />

        {:else if block.type === 'checkbox'}
          <CheckboxBlock
            value={!!block.value}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'radio'}
          <RadioBlock
            value={(block.value as string) || ''}
            meta={block.meta || { options: [] }}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'checkbox_group'}
          <CheckboxGroupBlock
            value={(block.value as string[]) || []}
            meta={block.meta || { options: [] }}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'image'}
          <ImageBlock
            value={block.value as string | { url: string; caption?: string } | null}
            cardId={cardId}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'progress'}
          <ProgressBlock
            value={(block.value as number) || 0}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'alarm'}
          <AlarmBlock
            value={block.value as string | null}
            meta={block.meta || {}}
            onUpdate={(val, newMeta) => commitBlock(block, val, newMeta)}
          />
        {:else if block.type === 'survey'}
          <SurveyBlock
            value={(block.value as SurveyQuestion[]) || []}
            onUpdate={(val) => commitBlock(block, val)}
          />
        {:else if block.type === 'slide_deck'}
          <SlideDeckBlock
            value={block.value && typeof block.value === 'object' && !Array.isArray(block.value) && 'slides' in block.value ? (block.value as SlideDeckValue) : { slides: [], currentIndex: 0 }}
            cardId={cardId}
            onUpdate={(val) => commitBlock(block, val)}
          />

        {:else}
          <!-- Legacy/unknown block type: show value as read-only text -->
          <span class="block-value">{block.value ?? ''}</span>
        {/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .block-wrapper {
    display: flex;
    align-items: flex-start;
    gap: 0;
    position: relative;
    transition: opacity var(--duration-normal);
    border-radius: 6px;
  }
  .block-wrapper:focus-within {
    outline: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    outline-offset: 2px;
  }
  .block-wrapper.block-dragging {
    opacity: 0.35;
  }

  /* CSS-only visual collapse during drag — hides block bodies without
     removing DOM nodes. The outer .blocks-list lives in CardDetail, so
     we prefix with :global() to cross the component boundary. */
  /* The dragged block collapses too — a compact source row (paired with
     the header-only drag image set in CardDetail's handleBlockDragStart)
     makes drop positions much easier to read. */
  :global(.blocks-list.drag-visual-collapse) .block-wrapper .block-body {
    max-height: 0;
    overflow: hidden;
    opacity: 0;
    margin: 0;
    padding: 0;
    transition: max-height var(--duration-normal) ease, opacity var(--duration-fast) ease;
  }

  .block-drag-handle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    align-self: stretch;
    color: transparent;
    cursor: grab;
    flex-shrink: 0;
    border-radius: 4px;
    transition: color var(--duration-fast), background var(--duration-fast);
    margin-right: 2px;
  }
  .block-wrapper:hover .block-drag-handle {
    color: var(--text-muted);
  }
  .block-drag-handle:hover {
    color: var(--text-secondary) !important;
    background: var(--bg-elevated);
  }
  .block-drag-handle:active {
    cursor: grabbing;
  }

  .block-content {
    flex: 1;
    min-width: 0;
  }

  .block-label-row {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin: 0 0 0.5rem;
  }
  .block-label-text {
    cursor: text;
    padding: 0.1rem 0.2rem;
    border-radius: 3px;
  }
  .block-label-text:hover {
    color: var(--accent-light);
    background: var(--bg-elevated);
  }

  .block-actions {
    margin-left: auto;
    display: flex;
    gap: 0.15rem;
    opacity: 0;
    transition: opacity var(--duration-fast);
  }
  .block-wrapper:hover .block-actions {
    opacity: 1;
  }

  .block-action-btn {
    padding: 0.15rem;
    line-height: 1;
    display: flex;
    align-items: center;
  }

  .block-label-input {
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    padding: 0.15rem 0.4rem;
    outline: none;
    margin-bottom: 0.5rem;
  }

  .block-collapse-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.25rem;
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border-radius: 4px;
    margin: -0.25rem 0;
    transition: background var(--duration-fast), color var(--duration-fast);
  }
  .block-collapse-btn:hover {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }

  .block-body {
    padding-left: 8px; /* indent content past the drag-handle column */
  }

  .text-scroll-wrap {
    position: relative;
  }

  /* desc-display and desc-textarea base styles live in CardDetail
     (shared with the description section). The .text-scroll modifier
     is BlockItem-specific — applied to text-block bodies only. */
  :global(.desc-display.text-scroll) {
    max-height: 200px;
    overflow-y: auto;
  }

  .text-scroll-gradient {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 32px;
    background: linear-gradient(transparent, var(--bg-surface));
    pointer-events: none;
    z-index: 1;
  }

  .text-expand-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.7rem;
    padding: 0.2rem 0;
  }
  .text-expand-btn:hover { color: var(--accent); }

  .block-divider {
    border: none;
    border-top: 1px solid var(--border-muted);
    margin: 0.25rem 0;
  }

  .block-url-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .block-url-empty {
    font-size: 0.85rem;
    color: var(--text-faint);
    cursor: text;
    padding: 0.25rem 0;
    display: inline-block;
  }
  .block-url-empty:hover { color: var(--text-muted); }

  .block-url-input {
    width: 100%;
    background: var(--bg-elevated);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.35rem 0.5rem;
    font-size: 0.85rem;
    outline: none;
  }
  .block-url-input:focus { border-color: var(--accent); }

  .block-value {
    font-size: 0.85rem;
    color: var(--text-body);
    padding: 0.25rem 0;
    display: inline-block;
  }

  .block-link {
    font-size: 0.85rem;
    color: var(--accent-light);
    text-decoration: none;
    word-break: break-all;
  }
  .block-link:hover { text-decoration: underline; }

  .checklist-progress {
    font-size: 0.7rem;
    color: var(--text-muted);
    font-weight: 500;
    letter-spacing: normal;
    text-transform: none;
  }

  .desc-textarea {
    width: 100%;
    background: var(--bg-elevated);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.5rem;
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
  }
  .desc-textarea:focus { border-color: var(--accent); }

  /* Section scaffolding within a block. The outer .section rule lives
     in CardDetail; this is the body-only portion that scopes to the
     BlockItem content area. */
  .section-title {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
</style>
