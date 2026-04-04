<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardType, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
    DeleteCard, PinCard, UnpinCard, GetCardPinBreadcrumbs, AddProjectLabel, GetProjectLabels, GetProjectLocation } from '../lib/api'
  import { projectTags, nav, getTagColor, cardTypes } from '../lib/store.svelte'
  import { X, Trash2, Plus, Type, ListChecks, Hash, Calendar, ToggleLeft, Link, Image, GripVertical, Pencil, MapPin, MapPinOff, MoveRight, Bot } from 'lucide-svelte'
  import { renderMarkdown, renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import PinPicker from './PinPicker.svelte'
  import ChatSection from './ChatSection.svelte'
  import EditableChecklist from './EditableChecklist.svelte'
  import SaveIndicator from './SaveIndicator.svelte'
  import { draggable } from '../lib/draggable'
  import { getCardTypeColor, getCardTypeTextColor } from '../lib/cardTypes'
  import { focusOnMount, focusTrap, inlineEdit, floatingDropdown } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, Block, BlockMeta, CardPin } from '../lib/types'


  let { cardId, currentCategoryId, currentCategoryName, onClose, onUpdated, onPin, autoEditTitle }: {
    cardId: string
    currentCategoryId?: string | null
    currentCategoryName?: string | null
    onClose: (opts?: { escaped?: boolean }) => void
    onUpdated?: () => void
    onPin?: () => void
    autoEditTitle?: boolean
  } = $props()

  let card = $state<Card | null>(null)
  let loading = $state(true)
  let pinBreadcrumbs = $state<CardPin[]>([])
  let showPinPicker = $state(false)
  let pinPickerMode = $state<'pin' | 'move'>('pin')
  let pinPickerSourcePin = $state<CardPin | null>(null)
  let pinActionLoading = $state(false)
  let showOtherPins = $state(false)
  const CHAT_VISIBLE_KEY = 'bruv:chatPanelVisible'
  let showChat = $state(localStorage.getItem(CHAT_VISIBLE_KEY) === 'true')
  $effect(() => { localStorage.setItem(CHAT_VISIBLE_KEY, String(showChat)) })

  // Resizable main panel width
  const MAIN_WIDTH_KEY = 'bruv:mainPanelWidth'
  const EDGE_PAD = 32
  const COLLAPSE_GUARD = 350
  let mainWidth = $state(Number(localStorage.getItem(MAIN_WIDTH_KEY)) || 600)
  let mainResizing = $state(false)
  let modalEl: HTMLDivElement | undefined = $state()

  function onMainResizeDown(e: MouseEvent) {
    e.preventDefault()
    if (!modalEl) return
    mainResizing = true
    const startX = e.clientX
    const startW = mainWidth
    const startLeft = parseFloat(modalEl.style.left) || modalEl.getBoundingClientRect().left

    function onMove(ev: MouseEvent) {
      const delta = ev.clientX - startX  // negative when dragging left
      const raw = startW - delta
      // Don't let the left edge past the screen edge (plus padding)
      const maxW = startLeft + startW - EDGE_PAD
      const newW = Math.max(COLLAPSE_GUARD, Math.min(raw, maxW))
      const actualDelta = newW - startW
      mainWidth = newW
      if (modalEl) modalEl.style.left = (startLeft - actualDelta) + 'px'
    }

    function suppressClick(ev: MouseEvent) {
      ev.stopPropagation()
      ev.preventDefault()
      window.removeEventListener('click', suppressClick, true)
    }

    function onUp() {
      mainResizing = false
      localStorage.setItem(MAIN_WIDTH_KEY, String(mainWidth))
      // Persist the new modal position after left-edge resize shifted it
      if (modalEl) {
        localStorage.setItem('bruv:cardDialogPos', JSON.stringify({
          left: parseFloat(modalEl.style.left),
          top: parseFloat(modalEl.style.top),
        }))
      }
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      window.addEventListener('click', suppressClick, true)
    }

    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  let savingCount = $state(0)
  let saving = $derived(savingCount > 0)

  async function tracked<T>(promise: Promise<T>): Promise<T> {
    savingCount++
    try { return await promise }
    finally { savingCount-- }
  }

  // Allow pinning from Inbox to upgrade the card's display context to the newly pinned category
  let inboxPinCategoryId = $state<string | null>(null)
  let inboxPinCategoryName = $state<string | null>(null)
  let effectiveCategoryId = $derived(inboxPinCategoryId ?? currentCategoryId ?? null)
  let effectiveCategoryName = $derived(inboxPinCategoryName ?? currentCategoryName ?? null)

  // Derived: pin state relative to the category the card was opened from
  let currentPin = $derived(
    effectiveCategoryId ? pinBreadcrumbs.find(p => p.categoryId === effectiveCategoryId) ?? null : null
  )
  let isPinnedHere = $derived(currentPin !== null)
  let otherPins = $derived(
    effectiveCategoryId ? pinBreadcrumbs.filter(p => p.categoryId !== effectiveCategoryId) : pinBreadcrumbs
  )
  let editingTitle = $state(false)
  let titleDraft = $state('')
  let newTag = $state('')

  // Type picker
  let showTypePicker = $state(false)
  let typePickerEl = $state<HTMLDivElement | null>(null)
  let typeBadgeBtnEl = $state<HTMLButtonElement | null>(null)

  // Description (standard field, always at top)
  let descriptionDraft = $state('')
  let editingDescription = $state(false)
  let descTextareaEl = $state<HTMLTextAreaElement | null>(null)

  // Block label editing
  let editingBlockLabelId = $state<string | null>(null)
  let blockLabelDraft = $state('')

  // Block editing state: keyed by block ID (stable across reorders)
  let editingBlockId = $state<string | null>(null)
  let blockDrafts = $state<Record<string, string>>({})
  let blockTextareaEls = $state<Record<string, HTMLTextAreaElement | null>>({})
  let checklistInputEls = $state<Record<string, HTMLInputElement | null>>({})
  let newChecklistTexts = $state<Record<string, string>>({})

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
    if (draggingBlockIdx === null || dropBlockIdx === null || !card) {
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
      card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
      blockDrafts = {}
      for (const b of card.blocks) {
        if (b.type === 'text') blockDrafts[b.id] = String(b.value ?? '')
      }
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
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
      if (!typePickerEl?.contains(target) && !(target as HTMLElement).closest?.('.type-picker-dropdown')) showTypePicker = false
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

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, Hash, Calendar, ToggleLeft, Link, Image,
  }

  async function addBlock(blockType: string) {
    showBlockPicker = false
    if (!card) return
    const label = BLOCK_OPTIONS.find(o => o.type === blockType)?.label || blockType
    const id = `blk-${crypto.randomUUID().slice(0, 8)}`
    let value: Block['value'] = ''
    let meta: BlockMeta | undefined = undefined
    if (blockType === 'checklist') value = []
    else if (blockType === 'number') value = 0
    else if (blockType === 'checkbox') value = false
    else if (blockType === 'select') { value = ''; meta = { options: ['Option 1', 'Option 2', 'Option 3'] } }

    const newBlock: Block = { id, type: blockType as Block['type'], label, key: labelToKey(label), value, meta }
    const blocks = [...card.blocks, newBlock]
    try {
      card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
      if (blockType === 'text') {
        blockDrafts[id] = ''
        editingBlockId = id
      }
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  // @ mention picker state
  let mentionVisible = $state(false)
  let mentionAnchor = $state<{ top: number; left: number } | null>(null)
  let mentionTarget = $state<{ type: 'desc' } | { type: 'text'; blockId: string } | { type: 'checklist'; blockId: string } | null>(null)
  let mentionTriggerPos = $state<number>(0)

  $effect(() => {
    loadCard()
  })

  async function loadCard() {
    loading = true
    try {
      card = await GetCard(cardId) as Card
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      titleDraft = card.title
      descriptionDraft = card.fields?.description || ''
      blockDrafts = {}
      newChecklistTexts = {}
      for (const b of card.blocks) {
        if (b.type === 'text') blockDrafts[b.id] = String(b.value ?? '')
      }
      if (autoEditTitle) editingTitle = true
      // Refresh project tags so new tags (e.g. added by AI) get their colors
      if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
        try { projectTags.list = await GetProjectLabels(nav.brandSlug, nav.streamSlug, nav.projectSlug) || [] } catch {}
      }
    } catch (e) {
      showToast(t('error.load_failed'), 'error')
    }
    loading = false
  }

  function openTypePicker() {
    showTypePicker = !showTypePicker
  }

  async function selectType(newType: string) {
    showTypePicker = false
    if (newType === card?.type) return
    try {
      card = await tracked(UpdateCardType(cardId, newType)) as Card
    } catch (e) { showToast(t('error.type_failed'), 'error'); return }
    onUpdated?.()
  }

  async function saveTitle() {
    if (!titleDraft.trim() || titleDraft === card?.title) {
      editingTitle = false
      return
    }
    try {
      card = await tracked(UpdateCardTitle(cardId, titleDraft.trim())) as Card
      editingTitle = false
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  async function handleTitleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()  // prevent handleBackdropKeydown from also calling saveTitle
      await saveTitle()
      onClose()
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      editingDescription = true  // focusOnMount on the textarea blurs the title input → saveTitle() fires once via onblur
    } else if (e.key === 'Escape') {
      e.stopPropagation()
      editingTitle = false
      titleDraft = card?.title ?? titleDraft
    }
  }

  async function saveDescription() {
    if (!editingDescription) return
    const fields = { ...(card?.fields || {}), description: descriptionDraft }
    try {
      card = await tracked(UpdateCardFields(cardId, fields)) as Card
      editingDescription = false
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  function handleDescKeydown(e: KeyboardEvent) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      editingDescription = false
      descriptionDraft = card?.fields?.description || ''
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

  async function deleteBlock(blockId: string) {
    if (!card) return
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (!block) return
    if (!await showConfirm(t('card.confirm_delete_block').replace('{name}', block.label || block.type))) return
    const blocks = card.blocks.filter((b: Block) => b.id !== blockId)
    try {
      const updated = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
      card = updated
      blockDrafts = {}
      for (const b of updated.blocks) {
        if (b.type === 'text') blockDrafts[b.id] = String(b.value ?? '')
      }
    } catch (e) { showToast(t('error.delete_failed'), 'error'); return }
    onUpdated?.()
  }

  async function renameBlockLabel(blockId: string) {
    if (editingBlockLabelId !== blockId) return
    const label = blockLabelDraft.trim()
    editingBlockLabelId = null
    if (!label || !card) return
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (!block || label === block.label) return
    const blocks = card.blocks.map((b: Block) => b.id === blockId ? { ...b, label, key: labelToKey(label) } : b)
    try {
      card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  async function saveTextBlock(blockId: string) {
    if (editingBlockId !== blockId || !card) return
    const draft = blockDrafts[blockId]
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (draft === undefined || draft === String(block?.value ?? '')) {
      editingBlockId = null
      return
    }
    const updatedBlocks = card.blocks.map((b: Block) =>
      b.id === blockId && b.type === 'text' ? { ...b, value: draft } : b
    )
    try {
      card = await tracked(UpdateCardBlocks(cardId, updatedBlocks)) as Card
      editingBlockId = null
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  async function handleTextBlockKeydown(e: KeyboardEvent, blockId: string) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      editingBlockId = null
      const block = card?.blocks.find((b: Block) => b.id === blockId)
      blockDrafts[blockId] = String(block?.value ?? '')
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()  // prevent handleBackdropKeydown from also calling saveTextBlock
      await saveTextBlock(blockId)
      onClose()
    }
  }

  function handleTextBlockInput(e: Event, blockId: string) {
    const el = e.target as HTMLTextAreaElement
    checkForMention(el, { type: 'text', blockId })
  }

  function handleChecklistInputEvent(e: Event, blockId: string) {
    const el = e.target as HTMLInputElement
    checkForMention(el, { type: 'checklist', blockId })
  }

  function checkForMention(el: HTMLTextAreaElement | HTMLInputElement, target: { type: 'desc' } | { type: 'text'; blockId: string } | { type: 'checklist'; blockId: string }) {
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
      const blockId = mentionTarget.blockId
      const el = blockTextareaEls[blockId]
      const draft = blockDrafts[blockId] || ''
      const before = draft.slice(0, mentionTriggerPos)
      const after = draft.slice(el?.selectionStart ?? mentionTriggerPos + 1)
      blockDrafts[blockId] = before + markdown + after
      mentionVisible = false
      mentionTarget = null
      const newPos = before.length + markdown.length
      setTimeout(() => { el?.focus(); el?.setSelectionRange(newPos, newPos) }, 0)
    } else if (mentionTarget.type === 'checklist') {
      const blockId = mentionTarget.blockId
      const el = checklistInputEls[blockId]
      const text = newChecklistTexts[blockId] || ''
      const before = text.slice(0, mentionTriggerPos)
      const after = text.slice(el?.selectionStart ?? mentionTriggerPos + 1)
      newChecklistTexts[blockId] = before + markdown + after
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
      setTimeout(() => blockTextareaEls[target.blockId]?.focus(), 0)
    } else if (target?.type === 'checklist') {
      setTimeout(() => checklistInputEls[target.blockId]?.focus(), 0)
    }
  }

  async function handleSelectChange(blockId: string, value: string) {
    if (!card) return
    const blocks = card.blocks.map((b: Block) => b.id === blockId ? { ...b, value } : b)
    try {
      card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  // Tag helpers

  /** Ensure a tag exists in a project's tag definitions */
  async function ensureTagInProject(tagName: string, brandSlug: string, streamSlug: string, projectSlug: string) {
    try {
      const existing = await GetProjectLabels(brandSlug, streamSlug, projectSlug) || []
      if (!existing.some((t: { name: string }) => t.name.toLowerCase() === tagName.toLowerCase())) {
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
    if (!tag || card?.tags?.some((t: string) => t.toLowerCase() === tag.toLowerCase())) { newTag = ''; return }
    const tags = [...(card?.tags || []), tag]
    try {
      card = await tracked(UpdateCardTags(cardId, tags)) as Card
      newTag = ''
      await syncTagToProjects(tag)
    } catch (e) { showToast(t('error.tag_failed'), 'error'); return }
    onUpdated?.()
  }

  let suppressPickerUntil = 0

  async function removeTag(tag: string) {
    suppressPickerUntil = Date.now() + 200
    const tags = (card?.tags || []).filter((t: string) => t !== tag)
    try {
      card = await tracked(UpdateCardTags(cardId, tags)) as Card
    } catch (e) { showToast(t('error.tag_failed'), 'error'); return }
    onUpdated?.()
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
    return (card?.tags || []).some((t: string) => t.toLowerCase() === tagName.toLowerCase())
  }

  async function toggleProjectTag(tagName: string) {
    const current = card?.tags || []
    const isAssigned = current.some((t: string) => t.toLowerCase() === tagName.toLowerCase())
    const tags = isAssigned
      ? current.filter((t: string) => t.toLowerCase() !== tagName.toLowerCase())
      : [...current, tagName]
    try {
      card = await tracked(UpdateCardTags(cardId, tags)) as Card
      if (!isAssigned) await syncTagToProjects(tagName)
    } catch (e) { showToast(t('error.tag_failed'), 'error'); return }
    onUpdated?.()
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
      e.stopPropagation()
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
      card = await tracked(UpdateCardDueDate(cardId, value)) as Card
    } catch (e) { showToast(t('error.date_failed'), 'error'); return }
    onUpdated?.()
  }

  async function toggleCurrentPin() {
    if (!currentCategoryId) return
    pinActionLoading = true
    try {
      if (isPinnedHere && currentPin) {
        const name = currentCategoryName || currentPin.categoryName
        const msg = pinBreadcrumbs.length === 1
          ? t('card.confirm_unpin_last').replace('{name}', name)
          : t('card.confirm_unpin').replace('{name}', name)
        if (!await showConfirm(msg)) { pinActionLoading = false; return }
        const unpinProject = currentPin.pinnedProjectId || currentPin.categoryId
        await UnpinCard(cardId, unpinProject, currentPin.categoryId)
      } else {
        await PinCard(cardId, effectiveCategoryId!, effectiveCategoryId!)
      }
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
    } catch (e) { showToast(t('error.pin_failed'), 'error') }
    pinActionLoading = false
  }

  function openPinPicker() {
    pinPickerMode = 'pin'
    pinPickerSourcePin = null
    showPinPicker = true
  }

  function openMovePicker(fromPin: CardPin) {
    pinPickerMode = 'move'
    pinPickerSourcePin = fromPin
    showPinPicker = true
  }

  async function handlePinSelect(target: CardPin) {
    showPinPicker = false
    pinActionLoading = true
    const wasMovingCurrentPin = pinPickerMode === 'move' && pinPickerSourcePin?.categoryId === effectiveCategoryId
    const wasPinningFromInbox = pinPickerMode === 'pin' && !currentCategoryId && !inboxPinCategoryId
    try {
      if (pinPickerMode === 'move' && pinPickerSourcePin) {
        const unpinProject = pinPickerSourcePin.pinnedProjectId || pinPickerSourcePin.categoryId
        await UnpinCard(cardId, unpinProject, pinPickerSourcePin.categoryId)
        await PinCard(cardId, target.categoryId, target.categoryId)
      } else {
        await PinCard(cardId, target.categoryId, target.categoryId)
      }
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
      // When moving the current pin or pinning from Inbox: switch context to the new location
      if (wasMovingCurrentPin || wasPinningFromInbox) {
        inboxPinCategoryId = target.categoryId
        inboxPinCategoryName = target.categoryName
        showOtherPins = false
        document.dispatchEvent(new CustomEvent('bruv:select-project', {
          detail: { brandSlug: target.brandSlug, streamSlug: target.streamSlug, projectSlug: target.projectSlug }
        }))
      }
    } catch (e) {
      showToast(t('error.pin_failed'), 'error')
    }
    pinActionLoading = false
    pinPickerSourcePin = null
  }

  function navigateToPinnedProject(pin: CardPin) {
    document.dispatchEvent(new CustomEvent('bruv:select-project', {
      detail: { brandSlug: pin.brandSlug, streamSlug: pin.streamSlug, projectSlug: pin.projectSlug }
    }))
    // Update the card dialog's context to reflect the navigated project
    inboxPinCategoryId = pin.categoryId
    inboxPinCategoryName = pin.categoryName
    showOtherPins = false
  }

  async function handleUnpin(pin: CardPin) {
    const msg = pinBreadcrumbs.length === 1
      ? t('card.confirm_unpin_last').replace('{name}', pin.categoryName)
      : t('card.confirm_unpin').replace('{name}', pin.categoryName)
    if (!await showConfirm(msg)) return
    pinActionLoading = true
    try {
      const unpinProject = pin.pinnedProjectId || pin.categoryId
      await UnpinCard(cardId, unpinProject, pin.categoryId)
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      onPin?.()
      onUpdated?.()
    } catch (e) { showToast(t('error.pin_failed'), 'error') }
    pinActionLoading = false
  }

  async function handleDelete() {
    if (!await showConfirm(t('card.delete_confirm'))) return
    try {
      await DeleteCard(cardId)
      onUpdated?.()
      onClose()
    } catch (e) { showToast(t('error.delete_failed'), 'error') }
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('modal-backdrop')) {
      onClose()
    }
  }

  async function handleBackdropKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      if (editingTitle) await saveTitle()
      if (editingDescription) await saveDescription()
      if (editingBlockId !== null) await saveTextBlock(editingBlockId)
      if (editingBlockLabelId !== null) await renameBlockLabel(editingBlockLabelId)
      onClose()
      return
    }
    if (e.key === 'Escape') {
      if (editingTitle || editingDescription || editingBlockId !== null || editingBlockLabelId !== null) return
      onClose({ escaped: true })
    }
  }
</script>

<svelte:window onkeydown={handleBackdropKeydown} onclick={handleWindowClick} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="modal" bind:this={modalEl} class:chat-open={showChat} class:main-resizing={mainResizing} style={showChat ? '' : `width: ${mainWidth}px;`} use:draggable={{ handle: '.modal-header', persistKey: 'bruv:cardDialogPos' }} use:focusTrap>
   <div class="modal-left-resize" role="separator" tabindex="-1" onmousedown={onMainResizeDown}></div>
   <div class="modal-main" style="width: {mainWidth}px;">
    {#if loading}
      <div class="modal-loading">{t('app.loading')}</div>
    {:else if card}
      <div class="modal-header">
        <div class="type-picker-wrap" bind:this={typePickerEl}>
          <button
            class="type-badge type-badge-btn"
            bind:this={typeBadgeBtnEl}
            style="background: {getCardTypeColor(card.type, cardTypes.list)}; color: {getCardTypeTextColor(card.type)}"
            onclick={openTypePicker}
            title={t('tooltip.change_card_type')}
          >{cardTypes.list.find(t => t.id === card.type)?.label || card.type || t('card.type_none')}</button>
          {#if showTypePicker && typeBadgeBtnEl}
            <div class="type-picker-dropdown" use:floatingDropdown={{ trigger: typeBadgeBtnEl }}>
              <button
                class="type-picker-option"
                class:active={!card.type}
                onclick={() => selectType('')}
              >
                <span class="type-option-badge" style="background: var(--bg-elevated); color: var(--text-muted)">{t('card.type_none')}</span>
              </button>
              {#each cardTypes.list as ct}
                <button
                  class="type-picker-option"
                  class:active={card.type === ct.id}
                  onclick={() => selectType(ct.id)}
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
            onkeydown={handleTitleKeydown}
            onblur={saveTitle}
          />
        {:else}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions a11y_no_noninteractive_tabindex -->
        <h2 class="modal-title" tabindex="0" onclick={() => { editingTitle = true }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingTitle = true } }} title={t('tooltip.edit_title')}>
            {@html renderInline(card.title)}
          </h2>
        {/if}

      </div>

      <div class="modal-subheader">
        {#if effectiveCategoryId}
          <!-- Current-category toggle -->
          <button
            class="pin-toggle"
            class:pinned={isPinnedHere}
            onclick={toggleCurrentPin}
            disabled={pinActionLoading}
            title={isPinnedHere ? `${t('tooltip.unpin')} "${effectiveCategoryName}"` : `${t('card.pin_to')} "${effectiveCategoryName}"`}
          >
            {#if isPinnedHere}
              <MapPin size={11} />
            {:else}
              <MapPinOff size={11} />
            {/if}
            <span class="pin-toggle-name">{effectiveCategoryName}</span>
          </button>
          {#if currentPin}
            <button class="btn-pin-action" onclick={() => openMovePicker(currentPin)} disabled={pinActionLoading} title={t('tooltip.move_pin')} aria-label={t('tooltip.move_pin')}><MoveRight size={11} /></button>
          {/if}

          <!-- Other pins expandable -->
          <button
            class="btn-other-pins"
            class:expanded={showOtherPins}
            onclick={() => showOtherPins = !showOtherPins}
            disabled={pinActionLoading}
          >
            {t('card.other_pins')}{otherPins.length > 0 ? ` (${otherPins.length})` : ''} {showOtherPins ? '▲' : '▼'}
          </button>

        {:else}
          <!-- No context category (inbox or search) -->
          {#if pinBreadcrumbs.length === 0}
            <!-- Inbox: card has no pins — show direct pin action -->
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}><MapPin size={11} /> {t('card.pin_to')}</button>
          {:else}
            <!-- Opened from search with existing pins — show summary + editor -->
            <span class="location-inbox"><MapPin size={11} /> {pinBreadcrumbs.length !== 1 ? t('card.pin_count_plural').replace('{count}', String(pinBreadcrumbs.length)) : t('card.pin_count').replace('{count}', String(pinBreadcrumbs.length))}</span>
            <button
              class="btn-other-pins"
              class:expanded={showOtherPins}
              onclick={() => showOtherPins = !showOtherPins}
              disabled={pinActionLoading}
            >
              {showOtherPins ? `${t('card.hide_pins')} ▲` : `${t('card.edit_pins')} ▼`}
            </button>
          {/if}
        {/if}

        <!-- Expanded pin editor (other pins panel) -->
        {#if showOtherPins}
          <div class="other-pins-panel">
            {#each otherPins as pin}
              <div class="location-pin">
                <button class="location-breadcrumb" onclick={() => navigateToPinnedProject(pin)} title={t('tooltip.go_to_project')}><MapPin size={11} />{pin.breadcrumb}</button>
                <button class="btn-pin-action btn-unpin" onclick={() => handleUnpin(pin)} disabled={pinActionLoading} title={t('tooltip.unpin')} aria-label={t('tooltip.unpin')}><MapPinOff size={11} /></button>
              </div>
            {/each}
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}>{t('card.pin_to_category')}</button>
          </div>
        {/if}
      </div>

      <div class="modal-body">
        <!-- FAB: Add block (top-right, overlapping) -->
        <button class="fab-add-block" bind:this={fabBtnEl} onclick={toggleBlockPicker} title={t('tooltip.add_block')}>
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
              use:focusOnMount
              bind:this={descTextareaEl}
              bind:value={descriptionDraft}
              onkeydown={handleDescKeydown}
              oninput={handleDescInput}
              onblur={handleDescBlur}
              rows="4"
            ></textarea>
          {:else}
            <div class="desc-display" role="button" tabindex="0" onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingDescription = true }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingDescription = true } }} title={t('tooltip.edit_description')}>
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
                  role="presentation"
                  tabindex="-1"
                  draggable="true"
                  ondragstart={(e) => handleBlockDragStart(e, blockIdx)}
                  ondragend={handleBlockDragEnd}
                  title={t('tooltip.drag_block')}
                ><GripVertical size={14} /></span>

                <section class="section block-content">
                  <!-- Editable block label -->
                  {#if editingBlockLabelId === block.id}
                    <input
                      class="block-label-input"
                      use:focusOnMount={true}
                      bind:value={blockLabelDraft}
                      use:inlineEdit={{ onCommit: () => renameBlockLabel(block.id), onCancel: () => { editingBlockLabelId = null } }}
                    />
                  {:else}
                    {#if block.type === 'checklist'}
                      {@const items = Array.isArray(block.value) ? block.value : []}
                      <!-- svelte-ignore a11y_no_noninteractive_element_interactions a11y_no_noninteractive_tabindex -->
                      <h3 class="section-title block-label-row action-reveal-parent" tabindex="0" onclick={() => { editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' } }}>
                        <span class="block-label-text">{block.label || block.type}</span>
                        {#if items.length > 0}
                          <span class="checklist-progress">{items.filter((c: any) => c.done).length}/{items.length}</span>
                        {/if}
                        <span class="block-actions">
                          <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} title={t('tooltip.rename_block')}><Pencil size={11} /></button>
                          <button class="block-action-btn action-reveal action-reveal--danger" onclick={(e) => { e.stopPropagation(); deleteBlock(block.id) }} title={t('tooltip.delete_block')}><Trash2 size={11} /></button>
                        </span>
                      </h3>
                    {:else}
                      <!-- svelte-ignore a11y_no_noninteractive_element_interactions a11y_no_noninteractive_tabindex -->
                      <h3 class="section-title block-label-row action-reveal-parent" tabindex="0" onclick={() => { editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' } }}>
                        <span class="block-label-text">{block.label || block.key || block.type}</span>
                        <span class="block-actions">
                          <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} title={t('tooltip.rename_block')}><Pencil size={11} /></button>
                          <button class="block-action-btn action-reveal action-reveal--danger" onclick={(e) => { e.stopPropagation(); deleteBlock(block.id) }} title={t('tooltip.delete_block')}><Trash2 size={11} /></button>
                        </span>
                      </h3>
                    {/if}
                  {/if}

                  {#if block.type === 'text'}
                    {#if editingBlockId === block.id}
                      <textarea
                        class="desc-textarea"
                        use:focusOnMount
                        bind:this={blockTextareaEls[block.id]}
                        bind:value={blockDrafts[block.id]}
                        onkeydown={(e) => handleTextBlockKeydown(e, block.id)}
                        oninput={(e) => handleTextBlockInput(e, block.id)}
                        onblur={() => { if (!mentionVisible) saveTextBlock(block.id) }}
                        rows="4"
                      ></textarea>
                    {:else}
                      <div class="desc-display" role="button" tabindex="0" onclick={(e) => { if ((e.target as HTMLElement).closest('a')) return; editingBlockId = block.id }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockId = block.id } }} title={t('tooltip.edit_description')}>
                        {#if block.value}
                          <div class="markdown-content">{@html renderMarkdown(String(block.value))}</div>
                        {:else}
                          <p class="placeholder">{t('card.description_placeholder')}</p>
                        {/if}
                      </div>
                    {/if}

                  {:else if block.type === 'checklist'}
                    <EditableChecklist
                      items={Array.isArray(block.value) ? block.value : []}
                      onUpdate={async (updated) => {
                        if (!card) return
                        const blocks = card.blocks.map((b: Block) => b.id === block.id ? { ...b, value: updated } : b)
                        try {
                          card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
                          onUpdated?.()
                        } catch (e) { showToast(t('error.save_failed'), 'error') }
                      }}
                    />

                  {:else if block.type === 'select' || block.type === 'radio'}
                    <select class="select-input" value={block.value || ''} onchange={(e) => handleSelectChange(block.id, (e.target as HTMLSelectElement).value)}>
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
        <span class="modal-footer-right">
          <SaveIndicator {saving} />
          <span class="meta">{t('card.created')} {card.created_at?.slice(0, 10) || '—'}</span>
        </span>
      </div>
    {/if}
   </div>

    {#if !loading && card}
      <ChatSection {cardId} bind:visible={showChat} bind:mainWidth onCardChanged={loadCard} />
    {/if}

    <div class="modal-actions">
      <button class="chat-toggle-btn" class:active={showChat} onclick={() => showChat = !showChat} title={t('tooltip.toggle_chat')}><Bot size={16} /></button>
      <button class="close-btn" onclick={() => onClose()} title={t('tooltip.close_card')}><X size={18} /></button>
    </div>
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
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="tag-picker-dropdown" role="listbox" use:floatingDropdown={{ trigger: tagInputEl }}>
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
  <div class="tag-picker-dropdown" use:floatingDropdown={{ trigger: tagInputEl }}>
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
    position: relative;
  }
  .modal.chat-open {
    width: auto;
  }
  .modal.main-resizing {
    user-select: none;
  }

  .modal-left-resize {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 5px;
    cursor: w-resize;
    z-index: 5;
    transition: background 0.15s;
    border-radius: 10px 0 0 10px;
  }
  .modal-left-resize:hover,
  .modal.main-resizing .modal-left-resize {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent-glow-1);
  }

  .modal-main {
    flex-shrink: 0;
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
    background: none;
    border: none;
    padding: 0;
    font-size: inherit;
    font-family: inherit;
    cursor: pointer;
    text-align: left;
  }
  .location-breadcrumb:hover {
    color: var(--accent);
    text-decoration: underline;
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

  .modal-actions {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    display: flex;
    align-items: center;
    gap: 4px;
    z-index: 3;
  }

  .chat-toggle-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
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
    cursor: text;
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

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .modal-footer-right {
    display: flex;
    align-items: center;
    gap: 0.75rem;
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
    cursor: text;
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
