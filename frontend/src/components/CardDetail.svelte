<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
    DeleteCard, AssignTagColor, SetTagColor } from '../lib/api'
  import { tagColors } from '../lib/store.svelte'
  import { X, Trash2, Square, CheckSquare, Palette, Plus, Type, ListChecks, Hash, Calendar, ToggleLeft, Link, Image, GripVertical, Pencil } from 'lucide-svelte'
  import { renderMarkdown, renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import { draggable } from '../lib/draggable'

  const TAG_PALETTE = [
    '#61bd4f', '#f2d600', '#ff9f1a', '#eb5a46', '#c377e0',
    '#0079bf', '#00c2e0', '#51e898', '#ff78cb', '#344563',
    '#b3bac5', '#096dd9',
  ]

  let colorPickerTag = $state<string | null>(null)

  let { cardId, onClose, onUpdated, autoEditTitle }: {
    cardId: string
    onClose: (opts?: { escaped?: boolean }) => void
    onUpdated?: () => void
    autoEditTitle?: boolean
  } = $props()

  let card = $state<any>(null)
  let loading = $state(true)
  let editingTitle = $state(false)
  let titleDraft = $state('')
  let titleInputEl = $state<HTMLInputElement | null>(null)
  let newTag = $state('')

  // Description (standard field, always at top)
  let descriptionDraft = $state('')
  let editingDescription = $state(false)
  let descTextareaEl = $state<HTMLTextAreaElement | null>(null)

  // Block label editing
  let editingBlockLabelIdx = $state<number | null>(null)
  let blockLabelDraft = $state('')
  let blockLabelInputEl = $state<HTMLInputElement | null>(null)

  // Block editing state: tracks which block index is being edited + drafts
  let editingBlockIdx = $state<number | null>(null)
  let blockDrafts = $state<Record<number, any>>({})
  let blockTextareaEls = $state<Record<number, HTMLTextAreaElement | null>>({})
  let checklistInputEls = $state<Record<number, HTMLInputElement | null>>({})
  let newChecklistTexts = $state<Record<number, string>>({})

  // Block drag-and-drop state
  let draggingBlockIdx = $state<number | null>(null)
  let dropBlockIdx = $state<number | null>(null)
  let blockCopyMode = $state(false)

  function handleBlockDragStart(e: DragEvent, idx: number) {
    draggingBlockIdx = idx
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'copyMove'
      e.dataTransfer.setData('text/plain', String(idx))
    }
  }

  function handleBlockDragEnd() {
    draggingBlockIdx = null
    dropBlockIdx = null
    blockCopyMode = false
  }

  function handleBlockDragOver(e: DragEvent, idx: number) {
    if (draggingBlockIdx === null) return
    e.preventDefault()
    e.stopPropagation()
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'

    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    dropBlockIdx = e.clientY < midY ? idx : idx + 1
  }

  function handleBlockDragOverGap(e: DragEvent, gapIdx: number) {
    if (draggingBlockIdx === null) return
    e.preventDefault()
    e.stopPropagation()
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'
    dropBlockIdx = gapIdx
  }

  async function handleBlockDrop(e: DragEvent) {
    e.preventDefault()
    if (draggingBlockIdx === null || dropBlockIdx === null || !card?.blocks) {
      handleBlockDragEnd()
      return
    }

    const copy = e.ctrlKey
    const fromIdx = draggingBlockIdx
    let toIdx = dropBlockIdx
    const blocks = [...card.blocks]

    handleBlockDragEnd()

    if (copy) {
      // Duplicate block at drop position
      const original = blocks[fromIdx]
      const dup = { ...original, id: `blk-${crypto.randomUUID().slice(0, 8)}` }
      blocks.splice(toIdx, 0, dup)
    } else {
      // Move: remove from old position, insert at new
      if (fromIdx === toIdx || fromIdx === toIdx - 1) return // no-op
      const [item] = blocks.splice(fromIdx, 1)
      const adjustedTo = toIdx > fromIdx ? toIdx - 1 : toIdx
      blocks.splice(adjustedTo, 0, item)
    }

    try {
      card = await UpdateCardBlocks(cardId, blocks)
      // Re-init drafts
      blockDrafts = {}
      for (let i = 0; i < card.blocks.length; i++) {
        if (card.blocks[i].type === 'text') blockDrafts[i] = card.blocks[i].value || ''
      }
      onUpdated?.()
    } catch (err) { console.error(err) }
  }

  // Add-block picker state
  let showBlockPicker = $state(false)
  let fabBtnEl = $state<HTMLButtonElement | null>(null)
  let fabPickerPos = $state({ top: 0, left: 0 })

  let fabPickerEl = $state<HTMLDivElement | null>(null)

  function toggleBlockPicker() {
    if (!showBlockPicker && fabBtnEl) {
      const r = fabBtnEl.getBoundingClientRect()
      fabPickerPos = { top: r.bottom + 4, left: r.right - 220 }
    }
    showBlockPicker = !showBlockPicker
  }

  function handleWindowClick(e: MouseEvent) {
    if (!showBlockPicker) return
    const t = e.target as Node
    if (fabBtnEl?.contains(t) || fabPickerEl?.contains(t)) return
    showBlockPicker = false
  }

  const BLOCK_OPTIONS = [
    { type: 'text',      label: 'Text',      icon: 'Type' },
    { type: 'checklist', label: 'Checklist', icon: 'ListChecks' },
    { type: 'number',    label: 'Number',    icon: 'Hash' },
    { type: 'date',      label: 'Date',      icon: 'Calendar' },
    { type: 'checkbox',  label: 'Checkbox',  icon: 'ToggleLeft' },
    { type: 'select',    label: 'Select',    icon: 'ListChecks' },
    { type: 'url',       label: 'Link',      icon: 'Link' },
    { type: 'image',     label: 'Image',     icon: 'Image' },
  ] as const

  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, Hash, Calendar, ToggleLeft, Link, Image,
  }

  async function addBlock(type: string) {
    showBlockPicker = false
    const label = BLOCK_OPTIONS.find(o => o.type === type)?.label || type
    const id = `blk-${crypto.randomUUID().slice(0, 8)}`
    let value: any = ''
    let meta: Record<string, any> | undefined = undefined
    if (type === 'checklist') value = []
    else if (type === 'number') value = 0
    else if (type === 'checkbox') value = false
    else if (type === 'select') { value = ''; meta = { options: ['Option 1', 'Option 2', 'Option 3'] } }
    else if (type === 'date') value = ''

    const newBlock = { id, type, label, key: '', value, meta: meta || undefined }
    const blocks = [...(card.blocks || []), newBlock]
    try {
      card = await UpdateCardBlocks(cardId, blocks)
      // Init draft for text blocks
      if (type === 'text') {
        blockDrafts[blocks.length - 1] = ''
        editingBlockIdx = blocks.length - 1
      }
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  // @ mention picker state
  let mentionVisible = $state(false)
  let mentionAnchor = $state<{ top: number; left: number } | null>(null)
  let mentionTarget = $state<{ type: 'desc' } | { type: 'text'; blockIdx: number } | { type: 'checklist'; blockIdx: number } | null>(null)
  let mentionTriggerPos = $state<number>(0)

  const typeColors: Record<string, string> = {
    feature: '#6366f1',
    task: '#22c55e',
    brainstorm: '#f59e0b',
    episode: '#ec4899',
    reference: '#06b6d4',
  }

  $effect(() => {
    loadCard()
  })

  $effect(() => {
    if (editingTitle && titleInputEl) {
      titleInputEl.focus()
      titleInputEl.select()
    }
  })

  $effect(() => {
    if (editingDescription && descTextareaEl) {
      descTextareaEl.focus()
    }
  })

  $effect(() => {
    if (editingBlockIdx !== null && blockTextareaEls[editingBlockIdx]) {
      blockTextareaEls[editingBlockIdx]?.focus()
    }
  })

  $effect(() => {
    if (editingBlockLabelIdx !== null && blockLabelInputEl) {
      blockLabelInputEl.focus()
      blockLabelInputEl.select()
    }
  })

  async function loadCard() {
    loading = true
    try {
      card = await GetCard(cardId)
      titleDraft = card.title
      descriptionDraft = card.fields?.description || ''
      // Initialize block drafts from loaded blocks
      blockDrafts = {}
      newChecklistTexts = {}
      if (card.blocks) {
        for (let i = 0; i < card.blocks.length; i++) {
          const b = card.blocks[i]
          if (b.type === 'text') {
            blockDrafts[i] = b.value || ''
          }
        }
      }
      if (autoEditTitle) {
        editingTitle = true
      }
    } catch (e) {
      console.error('Failed to load card:', e)
    }
    loading = false
  }

  async function saveTitle() {
    if (!titleDraft.trim() || titleDraft === card.title) {
      editingTitle = false
      return
    }
    try {
      card = await UpdateCardTitle(cardId, titleDraft.trim())
      editingTitle = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTitleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      saveTitle()
      editingDescription = true
    } else if (e.key === 'Escape') {
      editingTitle = false
      titleDraft = card.title
    }
  }

  async function saveDescription() {
    const fields = { ...(card.fields || {}), description: descriptionDraft }
    try {
      card = await UpdateCardFields(cardId, fields)
      editingDescription = false
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleDescKeydown(e: KeyboardEvent) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      editingDescription = false
      descriptionDraft = card.fields?.description || ''
    }
  }

  function handleDescBlur() {
    if (!editingDescription || mentionVisible) return
    saveDescription()
  }

  function handleDescInput(e: Event) {
    const el = e.target as HTMLTextAreaElement
    checkForMention(el, { type: 'desc' })
  }

  async function deleteBlock(blockIdx: number) {
    if (!card?.blocks?.[blockIdx]) return
    const label = card.blocks[blockIdx].label || card.blocks[blockIdx].type
    if (!confirm(`Delete block "${label}"?`)) return
    const blocks = card.blocks.filter((_: any, i: number) => i !== blockIdx)
    try {
      card = await UpdateCardBlocks(cardId, blocks)
      blockDrafts = {}
      if (card.blocks) {
        for (let i = 0; i < card.blocks.length; i++) {
          if (card.blocks[i].type === 'text') blockDrafts[i] = card.blocks[i].value || ''
        }
      }
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function renameBlockLabel(blockIdx: number) {
    const label = blockLabelDraft.trim()
    editingBlockLabelIdx = null
    if (!label || !card?.blocks?.[blockIdx]) return
    if (label === card.blocks[blockIdx].label) return
    card.blocks[blockIdx] = { ...card.blocks[blockIdx], label }
    try {
      card = await UpdateCardBlocks(cardId, card.blocks)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }


  function handleTextBlockKeydown(e: KeyboardEvent, blockIdx: number) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      editingBlockIdx = null
      blockDrafts[blockIdx] = card.blocks[blockIdx]?.value || ''
    }
  }

  async function saveTextBlock(blockIdx: number) {
    if (editingBlockIdx !== blockIdx) return
    const draft = blockDrafts[blockIdx]
    if (draft === undefined || draft === (card.blocks[blockIdx]?.value || '')) {
      editingBlockIdx = null
      return
    }
    const updatedBlocks = card.blocks.map((b: any, i: number) =>
      i === blockIdx && b.type === 'text' ? { ...b, value: draft } : b
    )
    try {
      card = await UpdateCardBlocks(cardId, updatedBlocks)
      editingBlockIdx = null
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTextBlockInput(e: Event, blockIdx: number) {
    const el = e.target as HTMLTextAreaElement
    checkForMention(el, { type: 'text', blockIdx })
  }

  function handleChecklistInputEvent(e: Event, blockIdx: number) {
    const el = e.target as HTMLInputElement
    checkForMention(el, { type: 'checklist', blockIdx })
  }

  function checkForMention(el: HTMLTextAreaElement | HTMLInputElement, target: { type: 'desc' } | { type: 'text'; blockIdx: number } | { type: 'checklist'; blockIdx: number }) {
    const pos = el.selectionStart ?? 0
    const text = el.value
    if (pos > 0 && text[pos - 1] === '@') {
      if (pos === 1 || /\s/.test(text[pos - 2])) {
        mentionTriggerPos = pos - 1
        mentionTarget = target
        const rect = el.getBoundingClientRect()
        mentionAnchor = { top: rect.bottom + 4, left: rect.left }
        mentionVisible = true
        return
      }
    }
  }

  function handleMentionSelect(markdown: string) {
    if (!mentionTarget) return
    if (mentionTarget.type === 'desc') {
      const before = descriptionDraft.slice(0, mentionTriggerPos)
      const after = descriptionDraft.slice(descTextareaEl?.selectionStart ?? mentionTriggerPos + 1)
      descriptionDraft = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      const newPos = before.length + markdown.length
      setTimeout(() => { descTextareaEl?.focus(); descTextareaEl?.setSelectionRange(newPos, newPos) }, 0)
    } else if (mentionTarget.type === 'text') {
      const idx = mentionTarget.blockIdx
      const el = blockTextareaEls[idx]
      const draft = blockDrafts[idx] || ''
      const before = draft.slice(0, mentionTriggerPos)
      const after = draft.slice(el?.selectionStart ?? mentionTriggerPos + 1)
      blockDrafts[idx] = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      const newPos = before.length + markdown.length
      setTimeout(() => { el?.focus(); el?.setSelectionRange(newPos, newPos) }, 0)
    } else if (mentionTarget.type === 'checklist') {
      const idx = mentionTarget.blockIdx
      const el = checklistInputEls[idx]
      const text = newChecklistTexts[idx] || ''
      const before = text.slice(0, mentionTriggerPos)
      const after = text.slice(el?.selectionStart ?? mentionTriggerPos + 1)
      newChecklistTexts[idx] = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      const newPos = before.length + markdown.length
      setTimeout(() => { el?.focus(); el?.setSelectionRange(newPos, newPos) }, 0)
    }
  }

  function handleMentionClose() {
    const target = mentionTarget
    mentionVisible = false
    mentionTarget = null
    // Refocus the source field so the user can continue editing
    if (target?.type === 'desc') {
      setTimeout(() => descTextareaEl?.focus(), 0)
    } else if (target?.type === 'text') {
      setTimeout(() => blockTextareaEls[target.blockIdx]?.focus(), 0)
    } else if (target?.type === 'checklist') {
      setTimeout(() => checklistInputEls[target.blockIdx]?.focus(), 0)
    }
  }

  // Checklist block helpers
  async function addChecklistItem(blockIdx: number) {
    const text = (newChecklistTexts[blockIdx] || '').trim()
    if (!text) return
    const block = card.blocks[blockIdx]
    const items = Array.isArray(block.value) ? [...block.value] : []
    items.push({ id: `ck-${crypto.randomUUID().slice(0, 8)}`, text, done: false })
    card.blocks[blockIdx] = { ...block, value: items }
    newChecklistTexts[blockIdx] = ''
    try {
      card = await UpdateCardBlocks(cardId, card.blocks)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function toggleChecklistItem(blockIdx: number, itemId: string) {
    const block = card.blocks[blockIdx]
    const items = (Array.isArray(block.value) ? block.value : []).map((item: any) =>
      item.id === itemId ? { ...item, done: !item.done } : item
    )
    card.blocks[blockIdx] = { ...block, value: items }
    try {
      card = await UpdateCardBlocks(cardId, card.blocks)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function removeChecklistItem(blockIdx: number, itemId: string) {
    const block = card.blocks[blockIdx]
    const items = (Array.isArray(block.value) ? block.value : []).filter((item: any) => item.id !== itemId)
    card.blocks[blockIdx] = { ...block, value: items }
    try {
      card = await UpdateCardBlocks(cardId, card.blocks)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleChecklistKeydown(e: KeyboardEvent, blockIdx: number) {
    if (mentionVisible) return
    if (e.key === 'Enter') addChecklistItem(blockIdx)
  }

  // Select block helper
  async function handleSelectChange(blockIdx: number, value: string) {
    card.blocks[blockIdx] = { ...card.blocks[blockIdx], value }
    try {
      card = await UpdateCardBlocks(cardId, card.blocks)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  // Tag helpers
  async function addTag() {
    const tag = newTag.trim().toLowerCase()
    if (!tag || card.tags?.includes(tag)) { newTag = ''; return }
    const tags = [...(card.tags || []), tag]
    try {
      card = await UpdateCardTags(cardId, tags)
      const colors = await AssignTagColor(tag)
      tagColors.map = colors || {}
      newTag = ''
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function changeTagColor(tag: string, color: string) {
    try {
      const colors = await SetTagColor(tag, color)
      tagColors.map = colors || {}
      colorPickerTag = null
    } catch (e) { console.error(e) }
  }

  async function removeTag(tag: string) {
    const tags = (card.tags || []).filter((t: string) => t !== tag)
    try {
      card = await UpdateCardTags(cardId, tags)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTagKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') addTag()
  }

  async function handleDueDateChange(e: Event) {
    const value = (e.target as HTMLInputElement).value
    try {
      card = await UpdateCardDueDate(cardId, value)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function handleDelete() {
    if (!confirm(t('card.delete_confirm'))) return
    try {
      await DeleteCard(cardId)
      onUpdated?.()
      onClose()
    } catch (e) { console.error(e) }
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('modal-backdrop')) {
      onClose()
    }
  }

  function handleBackdropKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (editingTitle || editingDescription || editingBlockIdx !== null || editingBlockLabelIdx !== null) return
      onClose({ escaped: true })
    }
  }
</script>

<svelte:window onkeydown={handleBackdropKeydown} onclick={handleWindowClick} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="modal" use:draggable={{ handle: '.modal-header' }}>
    {#if loading}
      <div class="modal-loading">{t('app.loading')}</div>
    {:else if card}
      <div class="modal-header">
        <span class="type-badge" style="background: {typeColors[card.type] || '#71717a'}">{card.type}</span>

        {#if editingTitle}
          <input
            class="title-input"
            bind:this={titleInputEl}
            bind:value={titleDraft}
            onkeydown={handleTitleKeydown}
            onblur={saveTitle}
          />
        {:else}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <h2 class="modal-title" onclick={() => { editingTitle = true }} title={t('tooltip.edit_title')}>
            {@html renderInline(card.title)}
          </h2>
        {/if}

        <button class="close-btn" onclick={() => onClose()} title={t('tooltip.close_card')}><X size={18} /></button>
      </div>

      <div class="modal-body">
        <!-- FAB: Add block (top-right, overlapping) -->
        <button class="fab-add-block" bind:this={fabBtnEl} onclick={toggleBlockPicker} title="Add a block">
          <Plus size={18} />
        </button>
        {#if showBlockPicker}
          <div class="fab-picker" bind:this={fabPickerEl} style="position:fixed; top:{fabPickerPos.top}px; left:{fabPickerPos.left}px;">
            {#each BLOCK_OPTIONS as opt}
              {@const Icon = BLOCK_ICON_MAP[opt.icon]}
              <button class="block-picker-item" onclick={() => addBlock(opt.type)} title={opt.label}>
                <Icon size={14} />
                <span>{opt.label}</span>
              </button>
            {/each}
          </div>
        {/if}

        <!-- Standard fields: compact 2-column grid -->
        <div class="fields-grid">
          <div class="field-cell">
            <span class="field-label">{t('card.due_date')}</span>
            <input
              type="date"
              class="date-input"
              value={card.due_date ? card.due_date.slice(0, 10) : ''}
              onchange={handleDueDateChange}
            />
          </div>
          <div class="field-cell">
            <span class="field-label">{t('card.tags')}</span>
            <div class="tags-list">
              {#each (card.tags || []) as tag}
                <span class="tag-chip" style:background={tagColors.map[tag] || 'var(--border)'}>
                  <span class="tag-label">{tag}</span>
                  <button class="tag-color-btn" onclick={() => colorPickerTag = colorPickerTag === tag ? null : tag} title={t('tooltip.change_tag_color')}><Palette size={10} /></button>
                  <button class="tag-remove" onclick={() => removeTag(tag)} title={t('tooltip.remove_tag')}><X size={12} /></button>
                </span>
                {#if colorPickerTag === tag}
                  <div class="color-picker">
                    {#each TAG_PALETTE as color}
                      <button
                        class="color-swatch"
                        class:active={tagColors.map[tag] === color}
                        style:background={color}
                        onclick={() => changeTagColor(tag, color)}
                      ></button>
                    {/each}
                  </div>
                {/if}
              {/each}
              <input
                type="text"
                bind:value={newTag}
                onkeydown={handleTagKeydown}
                placeholder={t('card.tags_placeholder')}
                class="tag-input tag-input-inline"
              />
            </div>
          </div>
        </div>

        <!-- Description (standard field, always present) -->
        <section class="section">
          <h3 class="section-title">{t('card.description')}</h3>
          {#if editingDescription}
            <textarea
              class="desc-textarea"
              bind:this={descTextareaEl}
              bind:value={descriptionDraft}
              onkeydown={handleDescKeydown}
              oninput={handleDescInput}
              onblur={handleDescBlur}
              rows="4"
            ></textarea>
          {:else}
            <div class="desc-display" role="button" tabindex="0" onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingDescription = true }} title={t('tooltip.edit_description')}>
              {#if card.fields?.description}
                <div class="markdown-content">{@html renderMarkdown(card.fields.description)}</div>
              {:else}
                <p class="placeholder">{t('card.description_placeholder')}</p>
              {/if}
            </div>
          {/if}
        </section>

        <!-- Blocks (excluding description block) -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="blocks-list" role="list" ondrop={handleBlockDrop}>
          {#each (card.blocks || []) as block, blockIdx}
            {#if block.key !== 'description'}
              <!-- Drop indicator before this block -->
              {#if draggingBlockIdx !== null && dropBlockIdx === blockIdx}
                <div class="block-drop-indicator" class:copy-mode={blockCopyMode}></div>
              {/if}

              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="block-wrapper"
                role="listitem"
                class:block-dragging={draggingBlockIdx === blockIdx}
                ondragover={(e) => handleBlockDragOver(e, blockIdx)}
              >
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <span
                  class="block-drag-handle"
                  role="button"
                  tabindex="0"
                  draggable="true"
                  ondragstart={(e) => handleBlockDragStart(e, blockIdx)}
                  ondragend={handleBlockDragEnd}
                  title="Drag to reorder"
                ><GripVertical size={14} /></span>

                <section class="section block-content">
                  <!-- Editable block label -->
                  {#if editingBlockLabelIdx === blockIdx}
                    <input
                      class="block-label-input"
                      bind:this={blockLabelInputEl}
                      bind:value={blockLabelDraft}
                      onblur={() => renameBlockLabel(blockIdx)}
                      onkeydown={(e) => { if (e.key === 'Enter') renameBlockLabel(blockIdx); if (e.key === 'Escape') editingBlockLabelIdx = null }}
                    />
                  {:else}
                    {#if block.type === 'checklist'}
                      {@const items = Array.isArray(block.value) ? block.value : []}
                      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
                      <h3 class="section-title block-label-row" onclick={() => { editingBlockLabelIdx = blockIdx; blockLabelDraft = block.label || '' }}>
                        <span class="block-label-text">{block.label || block.type}</span>
                        {#if items.length > 0}
                          <span class="checklist-progress">{items.filter((c: any) => c.done).length}/{items.length}</span>
                        {/if}
                        <span class="block-actions">
                          <button class="block-action-btn" onclick={(e) => { e.stopPropagation(); editingBlockLabelIdx = blockIdx; blockLabelDraft = block.label || '' }} title="Rename block"><Pencil size={11} /></button>
                          <button class="block-action-btn block-action-danger" onclick={(e) => { e.stopPropagation(); deleteBlock(blockIdx) }} title="Delete block"><Trash2 size={11} /></button>
                        </span>
                      </h3>
                    {:else}
                      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
                      <h3 class="section-title block-label-row" onclick={() => { editingBlockLabelIdx = blockIdx; blockLabelDraft = block.label || '' }}>
                        <span class="block-label-text">{block.label || block.key || block.type}</span>
                        <span class="block-actions">
                          <button class="block-action-btn" onclick={(e) => { e.stopPropagation(); editingBlockLabelIdx = blockIdx; blockLabelDraft = block.label || '' }} title="Rename block"><Pencil size={11} /></button>
                          <button class="block-action-btn block-action-danger" onclick={(e) => { e.stopPropagation(); deleteBlock(blockIdx) }} title="Delete block"><Trash2 size={11} /></button>
                        </span>
                      </h3>
                    {/if}
                  {/if}

                  {#if block.type === 'text'}
                    <!-- Text block -->
                    {#if editingBlockIdx === blockIdx}
                      <textarea
                        class="desc-textarea"
                        bind:this={blockTextareaEls[blockIdx]}
                        bind:value={blockDrafts[blockIdx]}
                        onkeydown={(e) => handleTextBlockKeydown(e, blockIdx)}
                        oninput={(e) => handleTextBlockInput(e, blockIdx)}
                        onblur={() => { if (!mentionVisible) saveTextBlock(blockIdx) }}
                        rows="4"
                      ></textarea>
                    {:else}
                      <div class="desc-display" role="button" tabindex="0" onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingBlockIdx = blockIdx }} title={t('tooltip.edit_description')}>
                        {#if block.value}
                          <div class="markdown-content">{@html renderMarkdown(String(block.value))}</div>
                        {:else}
                          <p class="placeholder">{t('card.description_placeholder')}</p>
                        {/if}
                      </div>
                    {/if}

                  {:else if block.type === 'checklist'}
                    <!-- Checklist block -->
                    {@const items = Array.isArray(block.value) ? block.value : []}

                    {#if items.length > 0}
                      <div class="checklist-bar">
                        <div
                          class="checklist-bar-fill"
                          style="width: {(items.filter((c: any) => c.done).length / items.length) * 100}%"
                        ></div>
                      </div>
                    {/if}

                    <div class="checklist-items">
                      {#each items as item}
                        <div class="checklist-item" class:done={item.done}>
                          <button class="checkbox" onclick={() => toggleChecklistItem(blockIdx, item.id)} title={t('tooltip.toggle_checklist')}>
                            {#if item.done}<CheckSquare size={16} />{:else}<Square size={16} />{/if}
                          </button>
                          <span class="checklist-text">{@html renderInline(item.text)}</span>
                          <button class="checklist-remove" onclick={() => removeChecklistItem(blockIdx, item.id)} title={t('tooltip.remove_checklist_item')}><Trash2 size={12} /></button>
                        </div>
                      {/each}
                    </div>

                    <div class="checklist-add">
                      <input
                        type="text"
                        bind:this={checklistInputEls[blockIdx]}
                        bind:value={newChecklistTexts[blockIdx]}
                        onkeydown={(e) => handleChecklistKeydown(e, blockIdx)}
                        oninput={(e) => handleChecklistInputEvent(e, blockIdx)}
                        placeholder={t('card.checklist_placeholder')}
                        class="checklist-input"
                      />
                      <button class="btn-add-sm" onclick={() => addChecklistItem(blockIdx)}>{t('card.checklist_add')}</button>
                    </div>

                  {:else if block.type === 'select' || block.type === 'radio'}
                    <select class="select-input" value={block.value || ''} onchange={(e) => handleSelectChange(blockIdx, (e.target as HTMLSelectElement).value)}>
                      <option value="">—</option>
                      {#each (block.meta?.options || []) as opt}
                        <option value={opt}>{opt}</option>
                      {/each}
                    </select>

                  {:else if block.type === 'number'}
                    <span class="block-value">{block.value ?? '—'}</span>

                  {:else if block.type === 'date'}
                    <span class="block-value">{block.value || '—'}</span>

                  {:else if block.type === 'checkbox'}
                    <span class="block-value">{block.value ? '✓' : '✗'}</span>

                  {:else if block.type === 'url' || block.type === 'image' || block.type === 'video'}
                    {#if block.value}
                      <a href={String(block.value)} target="_blank" rel="noopener" class="block-link">{String(block.value)}</a>
                    {:else}
                      <span class="block-value">—</span>
                    {/if}

                  {:else if block.type !== 'divider'}
                    <span class="block-value">{block.value ?? ''}</span>
                  {/if}
                </section>
              </div>
            {/if}
          {/each}

          <!-- Drop indicator after last block -->
          {#if draggingBlockIdx !== null && dropBlockIdx !== null && dropBlockIdx >= (card.blocks || []).length}
            <div class="block-drop-indicator" class:copy-mode={blockCopyMode}></div>
          {/if}
        </div>

      </div>

      <div class="modal-footer">
        <button class="btn-delete" onclick={handleDelete} title={t('tooltip.delete_card')}><Trash2 size={14} /> {t('card.delete')}</button>
        <span class="meta">Created {card.created_at?.slice(0, 10) || '—'}</span>
      </div>
    {/if}
  </div>
</div>

<MentionPicker
  visible={mentionVisible}
  anchor={mentionAnchor}
  onSelect={handleMentionSelect}
  onClose={handleMentionClose}
/>

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 3rem;
    z-index: 100;
    overflow-y: auto;
  }

  .modal {
    background: var(--bg-surface);
    border-radius: 10px;
    width: 600px;
    max-width: 95vw;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .modal-loading {
    padding: 2rem;
    text-align: center;
    color: var(--text-muted);
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .type-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.15rem 0.5rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .modal-title {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
    flex: 1;
    cursor: pointer;
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
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0.25rem;
    flex-shrink: 0;
  }
  .close-btn:hover { color: var(--text-primary); }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    position: relative;
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin: 0 0 0.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .desc-display {
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 6px;
    transition: background 0.1s;
  }
  .desc-display:hover { background: var(--bg-elevated); }
  .desc-display p { margin: 0; color: var(--text-body); font-size: 0.9rem; line-height: 1.5; }
  .desc-display .placeholder { white-space: pre-wrap; }
  .desc-display .placeholder { color: var(--text-faint); font-style: italic; }

  .desc-textarea {
    width: 100%;
    padding: 0.5rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
    box-sizing: border-box;
  }
  .desc-textarea:focus { border-color: var(--accent); }

  .date-input {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    color-scheme: dark light;
  }
  :global([data-theme="dark"]) .date-input { color-scheme: dark; }
  :global([data-theme="light"]) .date-input { color-scheme: light; }
  .date-input:focus { border-color: var(--accent); }

  .fields-grid {
    display: grid;
    grid-template-columns: auto 1fr 40px;
    gap: 0.5rem 1rem;
    align-items: start;
  }

  .field-cell {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .field-label {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .tags-list {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
    align-items: center;
  }

  .tag-chip {
    font-size: 0.75rem;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    color: #fff;
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .tag-label {
    white-space: nowrap;
  }

  .tag-color-btn {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-color-btn:hover { color: #fff; }

  .tag-remove {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-remove:hover { color: var(--danger-light); }

  .color-picker {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
    padding: 0.4rem;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    width: 100%;
  }

  .color-swatch {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    border: 2px solid transparent;
    cursor: pointer;
    transition: border-color 0.1s, transform 0.1s;
  }
  .color-swatch:hover {
    transform: scale(1.15);
  }
  .color-swatch.active {
    border-color: #fff;
  }

  .tag-input {
    padding: 0.25rem 0.4rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.75rem;
    outline: none;
  }
  .tag-input:focus { border-color: var(--accent); }
  .tag-input-inline {
    width: 80px;
    flex-shrink: 1;
  }

  .checklist-progress {
    font-size: 0.7rem;
    color: var(--text-muted);
    font-weight: 400;
  }

  .checklist-bar {
    height: 4px;
    background: var(--border);
    border-radius: 2px;
    overflow: hidden;
    margin-bottom: 0.5rem;
  }
  .checklist-bar-fill {
    height: 100%;
    background: var(--success);
    border-radius: 2px;
    transition: width 0.2s;
  }

  .checklist-items {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .checklist-item {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0;
  }

  .checkbox {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 1rem;
    padding: 0;
  }
  .checklist-item.done .checkbox { color: var(--success); }
  .checklist-item.done .checklist-text { text-decoration: line-through; color: var(--text-faint); }

  .checklist-text {
    flex: 1;
    font-size: 0.85rem;
    color: var(--text-body);
  }

  .checklist-remove {
    background: none;
    border: none;
    color: transparent;
    cursor: pointer;
    font-size: 0.75rem;
    padding: 0.15rem 0.3rem;
  }
  .checklist-item:hover .checklist-remove { color: var(--text-muted); }
  .checklist-remove:hover { color: var(--danger-light) !important; }

  .checklist-add {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .checklist-input {
    flex: 1;
    padding: 0.35rem 0.5rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
  }
  .checklist-input:focus { border-color: var(--accent); }

  .btn-add-sm {
    padding: 0.3rem 0.6rem;
    border: none;
    border-radius: 4px;
    background: var(--border);
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-add-sm:hover { background: var(--border-hover); }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }

  .btn-delete {
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-delete:hover { color: var(--danger-light); background: rgba(248, 113, 113, 0.1); }

  .select-input {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    min-width: 180px;
  }
  .select-input:focus { border-color: var(--accent); }

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

  .fab-add-block {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    border: none;
    background: #22c55e;
    color: #fff;
    cursor: pointer;
    box-shadow: 0 2px 8px rgba(0,0,0,0.25);
    transition: background 0.15s, transform 0.1s;
    z-index: 5;
  }
  .fab-add-block:hover { background: #16a34a; transform: scale(1.1); }

  .fab-picker {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.35rem;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    z-index: 9999;
    min-width: 220px;
  }

  .block-picker-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .block-picker-item:hover { background: var(--bg-elevated); color: var(--text-primary); }

  .block-label-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }
  .block-label-row:hover .block-label-text {
    color: var(--accent-light);
  }

  .block-actions {
    margin-left: auto;
    display: flex;
    gap: 0.15rem;
    opacity: 0;
    transition: opacity 0.1s;
  }
  .block-wrapper:hover .block-actions {
    opacity: 1;
  }

  .block-action-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.15rem;
    line-height: 1;
    display: flex;
    align-items: center;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .block-action-btn:hover {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }
  .block-action-btn.block-action-danger:hover {
    color: var(--danger-light);
    background: rgba(248, 113, 113, 0.1);
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

  .blocks-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  .block-wrapper {
    display: flex;
    align-items: flex-start;
    gap: 0;
    position: relative;
    transition: opacity 0.15s;
  }

  .block-wrapper.block-dragging {
    opacity: 0.35;
  }

  .block-drag-handle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    align-self: stretch;
    color: transparent;
    cursor: grab;
    flex-shrink: 0;
    border-radius: 4px;
    transition: color 0.1s, background 0.1s;
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

  .block-drop-indicator {
    height: 3px;
    background: var(--accent);
    border-radius: 2px;
    margin: 2px 22px 2px 22px;
    transition: background 0.1s;
  }
  .block-drop-indicator.copy-mode {
    background: var(--success, #22c55e);
  }
</style>
