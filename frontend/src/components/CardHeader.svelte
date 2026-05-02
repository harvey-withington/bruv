<script lang="ts">
  /**
   * CardHeader — the card modal's top strip: type-picker badge on the
   * left, editable title on the right, with the type-picker dropdown
   * floating off the badge button.
   *
   * State is hoisted via $bindable so CardDetail can keep a single
   * source of truth for editingTitle, showTypePicker, and the
   * element refs (typePickerEl used by the window click-outside
   * handler, typeBadgeBtnEl used as the floatingDropdown trigger).
   * All mutation logic (UpdateCardTitle, UpdateCardType,
   * RefreshTypeBlocks + save tracking) stays in the parent via
   * callbacks.
   *
   * Extracted from CardDetail to pull ~50 lines of template + ~100
   * lines of picker/badge/title CSS out of the god-component.
   */
  import { ArrowLeftRight, ChevronDown } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusOnMount, floatingDropdown } from '../lib/actions'
  import { renderInline } from '@shared/markdown'
  import { getCardTypeColor, getCardTypeTextColor } from '@shared/cardTypes'
  import type { Card, CardTypeInfo } from '@shared/types'

  let {
    card,
    cardTypesList,
    acceptedTypes,
    editingTitle = $bindable(),
    titleDraft = $bindable(),
    showTypePicker = $bindable(),
    typePickerEl = $bindable(),
    typeBadgeBtnEl = $bindable(),
    onSaveTitle,
    onTitleKeydown,
    onOpenTypePicker,
    onSelectType,
    onRefreshType,
  }: {
    card: Card
    cardTypesList: CardTypeInfo[]
    acceptedTypes: string[] | undefined
    editingTitle: boolean
    titleDraft: string
    showTypePicker: boolean
    typePickerEl: HTMLDivElement | null
    typeBadgeBtnEl: HTMLButtonElement | null
    onSaveTitle: () => void
    onTitleKeydown: (e: KeyboardEvent) => void
    onOpenTypePicker: () => void
    onSelectType: (typeId: string) => void
    onRefreshType: () => void
  } = $props()
</script>

<div class="modal-header">
  <div class="type-picker-wrap" bind:this={typePickerEl}>
    <button
      class="type-badge type-badge-btn"
      bind:this={typeBadgeBtnEl}
      style="background: {getCardTypeColor(card.type, cardTypesList)}; color: {getCardTypeTextColor(card.type)}"
      onclick={onOpenTypePicker}
      title={t('tooltip.change_card_type')}
    >{cardTypesList.find(t => t.id === card.type)?.label || card.type || t('card.type_none')}{#if card.type}<span class="refresh-type-btn" role="button" tabindex="-1"
        onclick={(e) => { e.stopPropagation(); onRefreshType() }}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); onRefreshType() } }}
        title={t('tooltip.refresh_type')}><ArrowLeftRight size={10} /></span>{/if}<ChevronDown size={10} class="type-chevron" /></button>
    {#if showTypePicker && typeBadgeBtnEl}
      {@const filteredTypes = acceptedTypes?.length
        ? cardTypesList.filter(ct => acceptedTypes.includes(ct.id))
        : cardTypesList}
      <div class="type-picker-dropdown" use:floatingDropdown={{ trigger: typeBadgeBtnEl }}>
        <button
          class="type-picker-option"
          class:active={!card.type}
          onclick={() => onSelectType('')}
        >
          <span class="type-option-badge" style="background: var(--bg-elevated); color: var(--text-muted)">{t('card.type_none')}</span>
        </button>
        {#each filteredTypes as ct}
          <button
            class="type-picker-option"
            class:active={card.type === ct.id}
            onclick={() => onSelectType(ct.id)}
          >
            <span class="type-option-badge" style="background: {ct.color}">{ct.label}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if editingTitle}
    <input
      class="title-input"
      use:focusOnMount={true}
      bind:value={titleDraft}
      onkeydown={onTitleKeydown}
      onblur={onSaveTitle}
    />
  {:else}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <h2 class="modal-title" tabindex="0" onclick={() => { editingTitle = true }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingTitle = true } }} title={t('tooltip.edit_title')}>
      {@html renderInline(card.title)}
    </h2>
  {/if}
</div>

<style>
  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .type-picker-wrap {
    position: relative;
    flex-shrink: 0;
  }

  .type-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.2rem 0.55rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .type-badge-btn {
    border: 1px solid transparent;
    cursor: pointer;
    transition: opacity var(--duration-normal), border-color var(--duration-normal);
  }
  .type-badge-btn:hover {
    opacity: 0.85;
    border-color: var(--border);
  }

  .refresh-type-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.1rem 0.2rem;
    margin: -0.1rem 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.75);
    cursor: pointer;
    border-radius: 2px;
    transition: color var(--duration-normal), background var(--duration-normal);
    flex-shrink: 0;
  }
  .refresh-type-btn:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.2);
  }

  /* type-chevron is inside the lucide svg element; needs :global to
     reach from component-scoped css. */
  :global(.type-chevron) {
    color: rgba(255, 255, 255, 0.5);
    margin-left: -0.1rem;
    flex-shrink: 0;
  }

  .type-picker-dropdown {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    min-width: 120px;
    overflow: hidden;
  }

  .type-picker-option {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 0.4rem 0.6rem;
    background: none;
    border: none;
    cursor: pointer;
    transition: background var(--duration-fast);
  }
  .type-picker-option:hover, .type-picker-option.active {
    background: var(--bg-elevated);
  }

  .type-option-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.5rem;
    border-radius: 3px;
    color: #fff;
    white-space: nowrap;
  }

  .modal-title {
    margin: 0;
    margin-right: 56px;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
    flex: 1;
    cursor: text;
    line-height: 1.3;
  }
  .modal-title:hover {
    color: var(--accent-light);
  }

  .title-input {
    flex: 1;
    font-size: 1.1rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-primary);
    padding: 0.3rem 0.5rem;
    outline: none;
    margin-right: 56px;
  }
</style>
