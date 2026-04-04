<script lang="ts">
  import CardItem from './CardItem.svelte'
  import { GripVertical, Trash2, Settings } from 'lucide-svelte'
  import { dnd, prefs, columnSettings, cardTypes } from '../lib/store.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '../lib/markdown'
  import { getCardTypeColor, CARD_TYPE_ORDER } from '../lib/cardTypes'
  import { UpdateCategoryAcceptedTypes } from '../lib/api'
  import { inlineEdit, floatingDropdown } from '../lib/actions'

  type CardData = {
    id: string
    type: string
    title: string
    tags: string[]
    labels?: string[]
    due_date: string | null
    checklist_total: number
    checklist_done: number
  }

  type CategoryData = {
    id: string
    name: string
    slug: string
    position: number
    accepted_types?: string[]
    cards: CardData[]
  }

  let { category, brandSlug, streamSlug, projectSlug, onCardClick, onAddCard, onCardDrop, onDeleteCategory, onStartRename, renaming, renamingName, onRenamingNameChange, onCommitRename, onCancelRename, isReadonly, onCategoryUpdated, onAcceptedTypesChanged }: {
    category: CategoryData
    brandSlug?: string
    streamSlug?: string
    projectSlug?: string
    onCardClick?: (cardId: string, categoryId: string) => void
    onAddCard?: (categoryId: string) => void
    onCardDrop?: (cardId: string, fromCategoryId: string, toCategoryId: string, toIndex: number, copy?: boolean) => void
    onDeleteCategory?: (categoryId: string, categorySlug: string, categoryName: string, cardCount: number) => void
    onStartRename?: (categorySlug: string, categoryName: string) => void
    renaming?: boolean
    renamingName?: string
    onRenamingNameChange?: (value: string) => void
    onCommitRename?: () => void
    onCancelRename?: () => void
    isReadonly?: boolean
    onCategoryUpdated?: () => void
    onAcceptedTypesChanged?: (categoryId: string, acceptedTypes: string[] | undefined) => void
  } = $props()

  let renameInputEl = $state<HTMLInputElement | null>(null)
  let showSettings = $derived(columnSettings.openCategoryId === category.id)
  let dropRejected = $state(false)

  // Sorted list of all card types (built-ins first in order, then user types)
  let allCardTypes = $derived(
    [...cardTypes.list].sort((a, b) => {
      const ai = CARD_TYPE_ORDER.indexOf(a.id as typeof CARD_TYPE_ORDER[number])
      const bi = CARD_TYPE_ORDER.indexOf(b.id as typeof CARD_TYPE_ORDER[number])
      if (ai === -1 && bi === -1) return a.label.localeCompare(b.label)
      if (ai === -1) return 1
      if (bi === -1) return -1
      return ai - bi
    })
  )
  let settingsBtnEl = $state<HTMLButtonElement | null>(null)

  $effect(() => {
    if (renaming && renameInputEl) {
      renameInputEl.focus()
      renameInputEl.select()
    }
  })

  async function openSettings() {
    if (showSettings) {
      closeSettings()
      return
    }
    columnSettings.openCategoryId = category.id
  }

  function isTypeAccepted(typeId: string): boolean {
    if (!category.accepted_types || category.accepted_types.length === 0) return false
    return category.accepted_types.includes(typeId)
  }

  function allTypesAccepted(): boolean {
    return !!category.accepted_types && category.accepted_types.length >= allCardTypes.length
  }

  let settingsDirty = $state(false)

  async function toggleAllTypes() {
    if (!brandSlug || !streamSlug || !projectSlug) return
    const newTypes: string[] = allTypesAccepted() ? [] : allCardTypes.map(t => t.id)
    const val = newTypes.length ? newTypes : undefined
    onAcceptedTypesChanged?.(category.id, val)
    settingsDirty = true
    try {
      await UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, category.slug, newTypes)
    } catch (e) {
      console.error('UpdateCategoryAcceptedTypes:', e)
    }
  }

  async function toggleType(type: string) {
    if (!brandSlug || !streamSlug || !projectSlug) return
    let current = category.accepted_types ? [...category.accepted_types] : []
    if (current.includes(type)) {
      current = current.filter(t => t !== type)
    } else {
      current.push(type)
    }
    const val = current.length ? current : undefined
    onAcceptedTypesChanged?.(category.id, val)
    settingsDirty = true
    try {
      await UpdateCategoryAcceptedTypes(brandSlug, streamSlug, projectSlug, category.slug, current)
    } catch (e) {
      console.error('UpdateCategoryAcceptedTypes:', e)
    }
  }

  // Check if dragged card type is allowed in this column
  function cardTypeAllowed(): boolean {
    if (!category.accepted_types || category.accepted_types.length === 0) return true
    if (dnd.dragging?.type !== 'card') return true
    const cardType = dnd.dragging.cardType
    if (!cardType) return true // untyped cards are always allowed
    return category.accepted_types.includes(cardType)
  }

  // --- Card drop zone ---
  function handleCardDragOver(e: DragEvent) {
    if (dnd.dragging?.type !== 'card') return
    e.preventDefault()

    if (!cardTypeAllowed()) {
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'none'
      dropRejected = true
      return
    }

    dropRejected = false
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'
    dnd.copyMode = e.ctrlKey
    dnd.overCategoryId = category.id

    // Calculate which card index we're hovering over
    const list = (e.currentTarget as HTMLElement)
    const cards = Array.from(list.querySelectorAll('.card-wrapper'))
    let idx = cards.length // default: append at end
    for (let i = 0; i < cards.length; i++) {
      const rect = cards[i].getBoundingClientRect()
      if (e.clientY < rect.top + rect.height / 2) {
        idx = i
        break
      }
    }
    dnd.overCardIndex = idx
  }

  function handleCardDragLeave(e: DragEvent) {
    // Only clear if actually leaving this column
    const related = e.relatedTarget as HTMLElement | null
    if (related && (e.currentTarget as HTMLElement).contains(related)) return
    if (dnd.overCategoryId === category.id) {
      dnd.overCategoryId = null
      dnd.overCardIndex = null
    }
    dropRejected = false
  }

  function handleCardDropOnList(e: DragEvent) {
    e.preventDefault()
    dropRejected = false
    if (dnd.dragging?.type !== 'card') return

    if (!cardTypeAllowed()) {
      dnd.dragging = null
      dnd.overCategoryId = null
      dnd.overCardIndex = null
      return
    }

    const { cardId, fromCategoryId } = dnd.dragging
    const toIndex = dnd.overCardIndex ?? category.cards.length
    onCardDrop?.(cardId, fromCategoryId, category.id, toIndex, e.ctrlKey)
    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
  }

  // --- Column drag ---
  function handleColDragStart(e: DragEvent) {
    if (!e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'copyMove'
    e.dataTransfer.setData('text/plain', category.id)
    dnd.dragging = { type: 'column', categoryId: category.id }
  }

  function handleColDragEnd() {
    dnd.dragging = null
    dnd.overColumnIndex = null
  }

  function closeSettings() {
    columnSettings.openCategoryId = null
    if (settingsDirty) {
      settingsDirty = false
      onCategoryUpdated?.()
    }
  }

  // Close settings popover on outside click
  function handleSettingsClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement
    if (!target.closest('.settings-popover') && !target.closest('.col-settings-btn')) {
      closeSettings()
    }
  }

  $effect(() => {
    if (showSettings) {
      document.addEventListener('click', handleSettingsClickOutside)
      return () => document.removeEventListener('click', handleSettingsClickOutside)
    }
  })

</script>

<div class="column" class:drop-rejected={dropRejected}>
  <div
    class="column-header"
    role="toolbar"
    tabindex="-1"
    draggable="true"
    ondragstart={handleColDragStart}
    ondragend={handleColDragEnd}
  >
    {#if !isReadonly}<span class="drag-handle" title={t('tooltip.drag_column')}><GripVertical size={14} /></span>{/if}
    {#if renaming}
      <input
        class="column-rename-input"
        bind:this={renameInputEl}
        value={renamingName}
        oninput={(e: Event) => onRenamingNameChange?.((e.target as HTMLInputElement).value)}
        use:inlineEdit={{ onCommit: () => onCommitRename?.(), onCancel: () => onCancelRename?.() }}
      />
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <h3
        class="column-title"
        class:editable={!isReadonly}
        role={!isReadonly ? 'button' : undefined}
        tabindex={!isReadonly ? 0 : undefined}
        onclick={() => { if (!isReadonly) onStartRename?.(category.slug, category.name) }}
      >{@html renderInline(category.name)}</h3>
      <span class="card-count">{category.cards.length}</span>
      {#if !isReadonly}
        <button
          class="col-action-btn col-settings-btn"
          bind:this={settingsBtnEl}
          title={t('tooltip.accepted_types')}
          onclick={(e: MouseEvent) => { e.stopPropagation(); openSettings() }}
        ><Settings size={13} /></button>
        <button
          class="col-action-btn col-delete-btn"
          title={t('tooltip.delete_category')}
          onclick={(e: MouseEvent) => { e.stopPropagation(); onDeleteCategory?.(category.id, category.slug, category.name, category.cards.length) }}
        ><Trash2 size={13} /></button>
      {/if}
    {/if}
  </div>

  {#if category.accepted_types && category.accepted_types.length > 0 && prefs.typeBadgeDisplay !== 'hidden'}
    {#if prefs.typeBadgeDisplay === 'color'}
      <div class="type-color-bar">
        {#each category.accepted_types as type}
          <span class="type-color-segment" style:background={getCardTypeColor(type, cardTypes.list)}></span>
        {/each}
      </div>
    {:else}
      <div class="type-badges">
        {#each category.accepted_types as type}
          <span class="type-chip" style:background={getCardTypeColor(type, cardTypes.list)}>{cardTypes.list.find(t => t.id === type)?.label || type}</span>
        {/each}
      </div>
    {/if}
  {/if}

  {#if showSettings && settingsBtnEl}
    <div class="settings-popover" use:floatingDropdown={{ trigger: settingsBtnEl }}>
      <div class="popover-title">{t('column.accepted_types')}</div>
      <label class="type-option">
        <input type="checkbox" checked={allTypesAccepted()} onchange={toggleAllTypes} />
        <span class="type-option-label">{t('column.all_types')}</span>
      </label>
      <div class="popover-divider"></div>
      {#each allCardTypes as type}
        <label class="type-option">
          <input type="checkbox" checked={isTypeAccepted(type.id)} onchange={() => toggleType(type.id)} />
          <span class="type-chip-inline" style:background={type.color}>{type.label}</span>
        </label>
      {/each}
    </div>
  {/if}

  <div
    class="card-list"
    role="list"
    ondragover={handleCardDragOver}
    ondragleave={handleCardDragLeave}
    ondrop={handleCardDropOnList}
  >
    {#each category.cards as card, i (card.id)}
      {#if dnd.dragging?.type === 'card' && dnd.overCategoryId === category.id && dnd.overCardIndex === i}
        <div class="drop-indicator" class:copy={dnd.copyMode}></div>
      {/if}
      <div class="card-wrapper">
        <CardItem {card} categoryId={category.id} onclick={() => onCardClick?.(card.id, category.id)} />
      </div>
    {/each}
    {#if dnd.dragging?.type === 'card' && dnd.overCategoryId === category.id && (dnd.overCardIndex ?? 0) >= category.cards.length}
      <div class="drop-indicator" class:copy={dnd.copyMode}></div>
    {/if}
  </div>

  {#if !isReadonly}
    <div class="column-footer">
      <button class="add-card-btn" onclick={() => onAddCard?.(category.id)} title={t('tooltip.add_card')}>
        {t('column.add_card_long')}
      </button>
    </div>
  {/if}
</div>

<style>
  .column {
    width: 272px;
    min-width: 272px;
    max-height: calc(100vh - 4rem);
    background: var(--bg-surface);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    transition: outline 0.15s;
    position: relative;
  }

  .column.drop-rejected {
    outline: 2px solid var(--danger, #ef4444);
    opacity: 0.6;
  }

  .column-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.6rem 0.75rem;
    cursor: grab;
  }

  .drag-handle {
    color: var(--text-faint);
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .card-wrapper {
    width: 100%;
  }

  .drop-indicator {
    height: 3px;
    background: var(--accent);
    border-radius: 2px;
    margin: 0 0.25rem;
  }
  .drop-indicator.copy {
    background: var(--success, #22c55e);
  }

  .column-rename-input {
    flex: 1;
    font-size: 0.85rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-strong);
    padding: 0.2rem 0.4rem;
    outline: none;
    min-width: 0;
  }

  .column-title {
    margin: 0;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-strong);
  }

  .column-title.editable {
    cursor: pointer;
    border-radius: 3px;
    padding: 0.05rem 0.2rem;
  }

  .column-title.editable:hover {
    color: var(--accent);
  }

  .card-count {
    font-size: 0.7rem;
    color: var(--text-muted);
    background: var(--bg-elevated);
    padding: 0.1rem 0.4rem;
    border-radius: 10px;
  }

  .col-action-btn {
    background: none;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.2rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    opacity: 0;
    transition: opacity 0.15s, color 0.15s;
  }
  .col-settings-btn {
    margin-left: auto;
  }
  .column-header:hover .col-action-btn { opacity: 1; }
  .col-action-btn:hover { color: var(--accent); }
  .col-delete-btn:hover { color: var(--danger); }

  /* Type badges below column title */
  .type-badges {
    display: flex;
    gap: 0.2rem;
    padding: 0 0.75rem 0.3rem;
    flex-wrap: wrap;
  }

  .type-chip {
    font-size: 0.55rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.05rem 0.3rem;
    border-radius: 3px;
    color: #fff;
    line-height: 1.4;
  }

  .type-color-bar {
    display: flex;
    height: 3px;
    margin: 0 0.5rem 0.3rem;
    border-radius: 2px;
    overflow: hidden;
  }

  .type-color-segment {
    flex: 1;
  }

  /* Settings popover */
  .settings-popover {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.5rem;
    box-shadow: 0 4px 16px var(--shadow);
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    width: 220px;
  }

  .popover-title {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.25rem;
  }

  .popover-divider {
    height: 1px;
    background: var(--border);
    margin: 0.15rem 0;
  }

  .type-option {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.25rem;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    color: var(--text-body);
  }

  .type-option:hover {
    background: var(--bg-surface);
  }

  .type-option input[type="checkbox"] {
    margin: 0;
    cursor: pointer;
  }

  .type-option-label {
    font-size: 0.8rem;
    color: var(--text-body);
  }

  .type-chip-inline {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.05rem 0.35rem;
    border-radius: 3px;
    color: #fff;
    line-height: 1.4;
  }

  .card-list {
    flex: 1;
    overflow-y: auto;
    padding: 0 0.5rem;
    min-height: 40px;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .column-footer {
    padding: 0.5rem;
  }

  .add-card-btn {
    width: 100%;
    padding: 0.4rem 0.5rem;
    background: none;
    border: none;
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s, color 0.1s;
  }

  .add-card-btn:hover {
    background: var(--bg-elevated);
    color: var(--text-body);
  }

</style>
