<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardType, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
    DeleteCard, PinCard, UnpinCard, GetCardPinBreadcrumbs, ListCardTypes, AddProjectLabel, GetProjectLabels, GetProjectLocation } from '../lib/api'
  import { projectTags, nav, getTagColor } from '../lib/store.svelte'
  import { X, Trash2, Square, CheckSquare, Plus, Type, ListChecks, Hash, Calendar, ToggleLeft, Link, Image, GripVertical, Pencil, MapPin, MapPinOff, MoveRight, MessageSquare } from 'lucide-svelte'
  import { renderMarkdown, renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import PinPicker from './PinPicker.svelte'
  import ChatSection from './ChatSection.svelte'
  import { draggable } from '../lib/draggable'

  type CategoryPath = {
    brandSlug: string; streamSlug: string; projectSlug: string; categorySlug: string
    brandName: string; streamName: string; projectName: string; categoryName: string
    projectId: string; categoryId: string
    breadcrumb: string
    pinnedProjectId?: string // set by GetCardPinBreadcrumbs — the actual stored pin.ProjectID for UnpinCard
  }

  let { cardId, currentCategoryId, currentCategoryName, onClose, onUpdated, onPin, autoEditTitle }: {
    cardId: string
    currentCategoryId?: string | null
    currentCategoryName?: string | null
    onClose: (opts?: { escaped?: boolean }) => void
    onUpdated?: () => void
    onPin?: () => void
    autoEditTitle?: boolean
  } = $props()

  let card = $state<any>(null)
  let loading = $state(true)
  let pinBreadcrumbs = $state<CategoryPath[]>([])
  let showPinPicker = $state(false)
  let pinPickerMode = $state<'pin' | 'move'>('pin')
  let pinPickerSourcePin = $state<CategoryPath | null>(null)
  let pinActionLoading = $state(false)
  let showOtherPins = $state(false)
  let showChat = $state(false)

  // Derived: pin state relative to the category the card was opened from
  let currentPin = $derived(
    currentCategoryId ? pinBreadcrumbs.find(p => p.categoryId === currentCategoryId) ?? null : null
  )
  let isPinnedHere = $derived(currentPin !== null)
  let otherPins = $derived(
    currentCategoryId ? pinBreadcrumbs.filter(p => p.categoryId !== currentCategoryId) : pinBreadcrumbs
  )
  let editingTitle = $state(false)
  let titleDraft = $state('')
  let titleInputEl = $state<HTMLInputElement | null>(null)
  let newTag = $state('')

  // Type picker
  let showTypePicker = $state(false)
  let cardTypes = $state<string[]>([])
  let typePickerEl = $state<HTMLDivElement | null>(null)

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
    const target = e.target as Node
    if (showBlockPicker) {
      if (!fabBtnEl?.contains(target) && !fabPickerEl?.contains(target)) showBlockPicker = false
    }
    if (showTypePicker) {
      if (!typePickerEl?.contains(target)) showTypePicker = false
    }
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
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
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

  async function openTypePicker() {
    if (cardTypes.length === 0) {
      try {
        cardTypes = await ListCardTypes() || []
      } catch (e) { console.error('Failed to load card types:', e) }
    }
    showTypePicker = !showTypePicker
  }

  async function selectType(newType: string) {
    showTypePicker = false
    if (newType === card.type) return
    try {
      card = await UpdateCardType(cardId, newType)
      onUpdated?.()
    } catch (e) { console.error('Failed to update card type:', e) }
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

  /** Ensure a tag exists in a project's tag definitions */
  async function ensureTagInProject(tagName: string, brandSlug: string, streamSlug: string, projectSlug: string) {
    try {
      const existing = await GetProjectLabels(brandSlug, streamSlug, projectSlug) || []
      if (!existing.some((t: any) => t.name.toLowerCase() === tagName.toLowerCase())) {
        await AddProjectLabel(brandSlug, streamSlug, projectSlug, tagName, '')
      }
    } catch { /* best-effort */ }
  }

  /** Sync a tag to the current project and all projects this card is pinned to */
  async function syncTagToProjects(tagName: string) {
    // Current project
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      await ensureTagInProject(tagName, nav.brandSlug, nav.streamSlug, nav.projectSlug)
    }
    // All pinned projects
    for (const pin of pinBreadcrumbs) {
      if (pin.brandSlug === nav.brandSlug && pin.streamSlug === nav.streamSlug && pin.projectSlug === nav.projectSlug) continue
      await ensureTagInProject(tagName, pin.brandSlug, pin.streamSlug, pin.projectSlug)
    }
    // Refresh current project tags
    if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
      try { projectTags.list = await GetProjectLabels(nav.brandSlug, nav.streamSlug, nav.projectSlug) || [] } catch {}
    }
  }

  async function addTag() {
    const tag = newTag.trim()
    if (!tag || card.tags?.some((t: string) => t.toLowerCase() === tag.toLowerCase())) { newTag = ''; return }
    const tags = [...(card.tags || []), tag]
    try {
      card = await UpdateCardTags(cardId, tags)
      newTag = ''
      await syncTagToProjects(tag)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  let suppressPickerUntil = 0

  async function removeTag(tag: string) {
    suppressPickerUntil = Date.now() + 200
    const tags = (card.tags || []).filter((t: string) => t !== tag)
    try {
      card = await UpdateCardTags(cardId, tags)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  // Combined tag input + picker
  let showTagPicker = $state(false)
  let tagInputEl = $state<HTMLInputElement | null>(null)
  let highlightIdx = $state(-1)

  let filteredProjectTags = $derived(
    projectTags.list.filter(t =>
      !isProjectTagAssigned(t.name) &&
      (!newTag.trim() || t.name.toLowerCase().includes(newTag.trim().toLowerCase()))
    )
  )

  // Reset highlight when filter text changes
  $effect(() => { newTag; highlightIdx = -1 })

  function isProjectTagAssigned(tagName: string): boolean {
    return (card.tags || []).some((t: string) => t.toLowerCase() === tagName.toLowerCase())
  }

  async function toggleProjectTag(tagName: string) {
    const current = card.tags || []
    const isAssigned = current.some((t: string) => t.toLowerCase() === tagName.toLowerCase())
    const tags = isAssigned
      ? current.filter((t: string) => t.toLowerCase() !== tagName.toLowerCase())
      : [...current, tagName]
    try {
      card = await UpdateCardTags(cardId, tags)
      if (!isAssigned) {
        await syncTagToProjects(tagName)
      }
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  function handleTagKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (filteredProjectTags.length > 0) {
        highlightIdx = Math.min(highlightIdx + 1, filteredProjectTags.length - 1)
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      highlightIdx = Math.max(highlightIdx - 1, -1)
    } else if (e.key === 'Tab' && showTagPicker && filteredProjectTags.length > 0) {
      e.preventDefault()
      if (e.shiftKey) {
        highlightIdx = Math.max(highlightIdx - 1, 0)
      } else {
        highlightIdx = Math.min(highlightIdx + 1, filteredProjectTags.length - 1)
      }
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (highlightIdx >= 0 && highlightIdx < filteredProjectTags.length) {
        toggleProjectTag(filteredProjectTags[highlightIdx].name)
        newTag = ''
        highlightIdx = -1
      } else {
        addTag()
      }
    } else if (e.key === 'Escape') {
      showTagPicker = false
      highlightIdx = -1
    }
  }

  function handleTagInputFocus() {
    if (Date.now() < suppressPickerUntil) return
    showTagPicker = true
    highlightIdx = -1
  }

  function handleTagInputBlur(e: FocusEvent) {
    // Keep picker open if focus moves to picker items
    const related = e.relatedTarget as HTMLElement | null
    if (related?.closest('.tag-picker-dropdown')) return
    // Small delay so click events on picker items fire first
    setTimeout(() => { showTagPicker = false; highlightIdx = -1 }, 150)
  }

  async function handleDueDateChange(e: Event) {
    const value = (e.target as HTMLInputElement).value
    try {
      card = await UpdateCardDueDate(cardId, value)
      onUpdated?.()
    } catch (e) { console.error(e) }
  }

  async function toggleCurrentPin() {
    if (!currentCategoryId) return
    pinActionLoading = true
    try {
      if (isPinnedHere && currentPin) {
        const msg = pinBreadcrumbs.length === 1
          ? `Unpin from "${currentCategoryName || currentPin.categoryName}"? The card will move to Inbox.`
          : `Unpin from "${currentCategoryName || currentPin.categoryName}"?`
        if (!confirm(msg)) { pinActionLoading = false; return }
        const unpinProject = currentPin.pinnedProjectId || currentPin.categoryId
        await UnpinCard(cardId, unpinProject, currentPin.categoryId)
      } else {
        await PinCard(cardId, currentCategoryId, currentCategoryId)
      }
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
    } catch (e) {
      console.error('Toggle pin failed:', e)
    }
    pinActionLoading = false
  }

  function openPinPicker() {
    pinPickerMode = 'pin'
    pinPickerSourcePin = null
    showPinPicker = true
  }

  function openMovePicker(fromPin: CategoryPath) {
    pinPickerMode = 'move'
    pinPickerSourcePin = fromPin
    showPinPicker = true
  }

  async function handlePinSelect(target: CategoryPath) {
    showPinPicker = false
    pinActionLoading = true
    try {
      if (pinPickerMode === 'move' && pinPickerSourcePin) {
        // Use pinnedProjectId (actual stored value) so UnpinCard can find the pin
        const unpinProject = pinPickerSourcePin.pinnedProjectId || pinPickerSourcePin.categoryId
        await UnpinCard(cardId, unpinProject, pinPickerSourcePin.categoryId)
        await PinCard(cardId, target.categoryId, target.categoryId) // convention: projectID == categoryID
      } else {
        await PinCard(cardId, target.categoryId, target.categoryId) // convention: projectID == categoryID
      }
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
    } catch (e) {
      console.error('Pin action failed:', e)
      alert('Could not pin card: ' + (e as any)?.message)
    }
    pinActionLoading = false
    pinPickerSourcePin = null
  }

  async function handleUnpin(pin: CategoryPath) {
    const msg = pinBreadcrumbs.length === 1
      ? `Unpin from "${pin.categoryName}"? The card will move to Inbox.`
      : `Unpin from "${pin.categoryName}"?`
    if (!confirm(msg)) return
    pinActionLoading = true
    try {
      const unpinProject = pin.pinnedProjectId || pin.categoryId
      await UnpinCard(cardId, unpinProject, pin.categoryId)
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
    } catch (e) {
      console.error('Unpin failed:', e)
    }
    pinActionLoading = false
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
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      if (editingTitle) saveTitle()
      if (editingDescription) saveDescription()
      if (editingBlockIdx !== null) saveTextBlock(editingBlockIdx)
      onClose()
      return
    }
    if (e.key === 'Escape') {
      if (editingTitle || editingDescription || editingBlockIdx !== null || editingBlockLabelIdx !== null) return
      onClose({ escaped: true })
    }
  }
</script>

<svelte:window onkeydown={handleBackdropKeydown} onclick={handleWindowClick} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="modal" class:chat-open={showChat} use:draggable={{ handle: '.modal-header' }}>
   <div class="modal-main">
    {#if loading}
      <div class="modal-loading">{t('app.loading')}</div>
    {:else if card}
      <div class="modal-header">
        <div class="type-picker-wrap" bind:this={typePickerEl}>
          <button
            class="type-badge type-badge-btn"
            style="background: {card.type ? (typeColors[card.type] || '#71717a') : 'var(--bg-elevated)'}; color: {card.type ? '#fff' : 'var(--text-muted)'}"
            onclick={openTypePicker}
            title="Change card type"
          >{card.type || 'None'}</button>
          {#if showTypePicker}
            <div class="type-picker-dropdown">
              <button
                class="type-picker-option"
                class:active={!card.type}
                onclick={() => selectType('')}
              >
                <span class="type-option-badge" style="background: var(--bg-elevated); color: var(--text-muted)">None</span>
              </button>
              {#each cardTypes as ct}
                <button
                  class="type-picker-option"
                  class:active={card.type === ct}
                  onclick={() => selectType(ct)}
                >
                  <span class="type-option-badge" style="background: {typeColors[ct] || '#71717a'}">{ct}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>

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

        <button class="chat-toggle-btn" class:active={showChat} onclick={() => showChat = !showChat} title="Toggle chat"><MessageSquare size={16} /></button>
        <button class="close-btn" onclick={() => onClose()} title={t('tooltip.close_card')}><X size={18} /></button>
      </div>

      <div class="modal-subheader">
        {#if currentCategoryId}
          <!-- Current-category toggle -->
          <button
            class="pin-toggle"
            class:pinned={isPinnedHere}
            onclick={toggleCurrentPin}
            disabled={pinActionLoading}
            title={isPinnedHere ? `Unpin from "${currentCategoryName}"` : `Pin to "${currentCategoryName}"`}
          >
            {#if isPinnedHere}
              <MapPin size={11} />
            {:else}
              <MapPinOff size={11} />
            {/if}
            <span class="pin-toggle-name">{currentCategoryName}</span>
          </button>

          <!-- Other pins expandable -->
          <button
            class="btn-other-pins"
            class:expanded={showOtherPins}
            onclick={() => showOtherPins = !showOtherPins}
            disabled={pinActionLoading}
          >
            Other pins{otherPins.length > 0 ? ` (${otherPins.length})` : ''} {showOtherPins ? '▲' : '▼'}
          </button>

        {:else}
          <!-- No context category (inbox or search) -->
          {#if pinBreadcrumbs.length === 0}
            <!-- Inbox: card has no pins — show direct pin action -->
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}><MapPin size={11} /> Pin to...</button>
          {:else}
            <!-- Opened from search with existing pins — show summary + editor -->
            <span class="location-inbox"><MapPin size={11} /> {pinBreadcrumbs.length} pin{pinBreadcrumbs.length !== 1 ? 's' : ''}</span>
            <button
              class="btn-other-pins"
              class:expanded={showOtherPins}
              onclick={() => showOtherPins = !showOtherPins}
              disabled={pinActionLoading}
            >
              {showOtherPins ? 'Hide ▲' : 'Edit pins ▼'}
            </button>
          {/if}
        {/if}

        <!-- Expanded pin editor (other pins panel) -->
        {#if showOtherPins}
          <div class="other-pins-panel">
            {#each otherPins as pin}
              <div class="location-pin">
                <span class="location-breadcrumb" title={pin.breadcrumb}><MapPin size={11} />{pin.breadcrumb}</span>
                <button class="btn-pin-action" onclick={() => openMovePicker(pin)} disabled={pinActionLoading} title="Move to another category" aria-label="Move pin"><MoveRight size={11} /></button>
                <button class="btn-pin-action btn-unpin" onclick={() => handleUnpin(pin)} disabled={pinActionLoading} title="Unpin" aria-label="Unpin"><MapPinOff size={11} /></button>
              </div>
            {/each}
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}>+ Pin to another category</button>
          </div>
        {/if}
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
                <span class="tag-chip" style:background={getTagColor(tag)}>
                  <span class="tag-label">{tag}</span>
                  <button class="tag-remove" onclick={() => removeTag(tag)} title={t('tooltip.remove_tag')}><X size={12} /></button>
                </span>
              {/each}
              <input
                type="text"
                bind:this={tagInputEl}
                bind:value={newTag}
                onkeydown={handleTagKeydown}
                onfocus={handleTagInputFocus}
                onblur={handleTagInputBlur}
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

    {#if !loading && card}
      <ChatSection {cardId} bind:visible={showChat} onCardChanged={loadCard} />
    {/if}
  </div>
</div>

<MentionPicker
  visible={mentionVisible}
  anchor={mentionAnchor}
  onSelect={handleMentionSelect}
  onClose={handleMentionClose}
/>

<PinPicker
  visible={showPinPicker}
  onSelect={handlePinSelect}
  onClose={() => { showPinPicker = false; pinPickerSourcePin = null }}
/>

{#if showTagPicker && tagInputEl && filteredProjectTags.length > 0}
  {@const rect = tagInputEl.getBoundingClientRect()}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="tag-picker-dropdown" role="listbox" style="position:fixed; top:{rect.bottom + 4}px; left:{rect.left}px; z-index:10000;">
    {#each filteredProjectTags as ptag, i (ptag.id)}
      <button
        class="tag-picker-item"
        class:highlighted={i === highlightIdx}
        tabindex="-1"
        onclick={() => { toggleProjectTag(ptag.name); newTag = ''; tagInputEl?.focus() }}
      >
        <span class="tag-picker-chip" style:background={ptag.color || 'var(--border)'}>{ptag.name}</span>
      </button>
    {/each}
    {#if newTag.trim() && !projectTags.list.some(t => t.name.toLowerCase() === newTag.trim().toLowerCase())}
      <div class="tag-picker-create">
        Press Enter to create "{newTag.trim()}"
      </div>
    {/if}
  </div>
{:else if showTagPicker && tagInputEl && newTag.trim() && !projectTags.list.some(t => t.name.toLowerCase() === newTag.trim().toLowerCase())}
  {@const rect = tagInputEl.getBoundingClientRect()}
  <div class="tag-picker-dropdown" style="position:fixed; top:{rect.bottom + 4}px; left:{rect.left}px; z-index:10000;">
    <div class="tag-picker-create">
      Press Enter to create "{newTag.trim()}"
    </div>
  </div>
{/if}

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
    flex-direction: row;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
    transition: width 0.2s ease;
  }
  .modal.chat-open {
    width: 960px;
  }

  .modal-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
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

  .modal-subheader {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem 0.5rem;
    padding: 0.35rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    font-size: 0.73rem;
    min-height: 2rem;
    position: relative;
  }

  /* Current-category pin toggle */
  .pin-toggle {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.55rem;
    border-radius: 5px;
    border: 1px solid var(--border-muted);
    background: var(--bg-surface);
    color: var(--text-muted);
    font-size: 0.73rem;
    cursor: pointer;
    transition: background 0.1s, border-color 0.1s, color 0.1s;
  }
  .pin-toggle.pinned {
    border-color: var(--accent);
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .pin-toggle:hover:not(:disabled) {
    border-color: var(--accent);
    color: var(--accent);
  }
  .pin-toggle.pinned:hover:not(:disabled) {
    border-color: #eb5a46;
    color: #eb5a46;
    background: color-mix(in srgb, #eb5a46 8%, transparent);
  }
  .pin-toggle:disabled { opacity: 0.5; cursor: default; }

  .pin-toggle-name {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* "Other pins (N)" button */
  .btn-other-pins {
    font-size: 0.7rem;
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    border: 1px solid transparent;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    margin-left: auto;
  }
  .btn-other-pins:hover { color: var(--text-body); background: var(--bg-surface); }
  .btn-other-pins.expanded { color: var(--text-body); }
  .btn-other-pins:disabled { opacity: 0.5; cursor: default; }

  /* Inbox / summary label */
  .location-inbox {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    color: var(--text-muted);
    font-style: italic;
  }

  /* Expanded other-pins panel — full width below the toggle row */
  .other-pins-panel {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.35rem 0 0.1rem;
    border-top: 1px solid var(--border-muted);
    margin-top: 0.1rem;
  }

  .location-pin {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    padding: 0.1rem 0.2rem 0.1rem 0.45rem;
    max-width: 100%;
  }

  .location-breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--text-body);
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .btn-pin {
    font-size: 0.7rem;
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    line-height: 1.4;
    align-self: flex-start;
  }
  .btn-pin:hover { border-color: var(--accent); color: var(--accent); }
  .btn-pin:disabled { opacity: 0.5; cursor: default; }

  .btn-pin-action {
    background: none;
    border: none;
    padding: 0.15rem 0.2rem;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    line-height: 1;
  }
  .btn-pin-action:hover { color: var(--text-primary); }
  .btn-unpin:hover { color: #eb5a46; }

  .type-picker-wrap {
    position: relative;
    flex-shrink: 0;
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

  .type-badge-btn {
    border: 1px solid transparent;
    cursor: pointer;
    transition: opacity 0.15s, border-color 0.15s;
  }
  .type-badge-btn:hover {
    opacity: 0.85;
    border-color: var(--border);
  }

  .type-picker-dropdown {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    z-index: 10;
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
    transition: background 0.1s;
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

  .chat-toggle-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    flex-shrink: 0;
    border-radius: 4px;
  }
  .chat-toggle-btn:hover { color: var(--text-primary); }
  .chat-toggle-btn.active { color: var(--accent); }

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
    width: 100px;
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

  /* Tag picker dropdown */
  :global(.tag-picker-dropdown) {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.3rem;
    box-shadow: 0 8px 32px var(--shadow-lg);
    min-width: 180px;
    max-height: 220px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  :global(.tag-picker-item) {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    cursor: pointer;
    padding: 0.25rem 0.35rem;
    border-radius: 4px;
    font-size: 0.8rem;
    background: none;
    border: none;
    color: var(--text-primary);
    width: 100%;
    text-align: left;
  }
  :global(.tag-picker-item:hover),
  :global(.tag-picker-item.highlighted) {
    background: var(--bg-surface);
  }

  :global(.tag-picker-chip) {
    font-size: 0.7rem;
    font-weight: 600;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    color: #fff;
    line-height: 1.4;
  }

  :global(.tag-picker-create) {
    font-size: 0.75rem;
    color: var(--text-muted);
    padding: 0.3rem 0.35rem;
    font-style: italic;
  }
</style>
