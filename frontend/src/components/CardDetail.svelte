<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardType, RefreshTypeBlocks, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
    DeleteCard, CreateCard, PinCard, UnpinCard, GetCardPinBreadcrumbs, AddProjectLabel, GetProjectLabels, GetProjectLocation, GetCategoryAcceptedTypes, GetAgentConfig } from '../lib/api'
  import { projectTags, nav, getTagColor, getTagIcon, cardTypes } from '../lib/store.svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import { X, Trash2, Plus, Type, ListChecks, List, Film, Link, Minus, GripVertical, Pencil, MapPin, MapPinOff, MoveRight, BotMessageSquare, ChevronDown, ChevronRight, ChevronsUpDown, ChevronsDownUp, Maximize2, ArrowLeftRight, Hash, Calendar, Star, ListTree, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell } from 'lucide-svelte'
  import { renderMarkdown, renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import PinPicker from './PinPicker.svelte'
  import ChatSection from './ChatSection.svelte'
  import AgentTab from './AgentTab.svelte'
  import AgentRunsTab from './AgentRunsTab.svelte'
  import EditableChecklist from './EditableChecklist.svelte'
  import EditableList from './EditableList.svelte'
  import MediaBlock from './MediaBlock.svelte'
  import CardAttachments from './CardAttachments.svelte'
  import { showOptionsEditor } from '../lib/optionsEditor.svelte'
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
  import SaveIndicator from './SaveIndicator.svelte'
  import { draggable } from '../lib/draggable'
  import { getCardTypeColor, getCardTypeTextColor } from '../lib/cardTypes'
  import { focusOnMount, focusTrap, inlineEdit, floatingDropdown } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, Block, BlockMeta, CardPin, ChecklistItem, ListItem, MediaItem } from '../lib/types'


  let { cardId, currentCategoryId, currentCategoryName, categoryAcceptedTypes, onClose, onUpdated, onPin, autoEditTitle, initialTab }: {
    cardId: string
    currentCategoryId?: string | null
    currentCategoryName?: string | null
    categoryAcceptedTypes?: string[]
    onClose: (opts?: { escaped?: boolean }) => void
    onUpdated?: () => void
    onPin?: () => void
    autoEditTitle?: boolean
    initialTab?: 'details' | 'agent'
  } = $props()

  let card = $state<Card | null>(null)
  let loading = $state(true)
  let pinBreadcrumbs = $state<CardPin[]>([])
  let acceptedTypes = $state<string[] | undefined>(categoryAcceptedTypes)
  let hasAgent = $state(false)
  let showPinPicker = $state(false)
  let pinPickerMode = $state<'pin' | 'move'>('pin')
  let pinPickerSourcePin = $state<CardPin | null>(null)
  let pinActionLoading = $state(false)
  let showOtherPins = $state(false)
  const CHAT_VISIBLE_KEY = 'bruv:chatPanelVisible'
  let showChat = $state(localStorage.getItem(CHAT_VISIBLE_KEY) === 'true')
  let activeTab = $state<'details' | 'agent' | 'runs'>(initialTab ?? 'details')
  $effect(() => { localStorage.setItem(CHAT_VISIBLE_KEY, String(showChat)) })

  // Splitter: redistributes space between main and chat when chat is open
  const MAIN_WIDTH_KEY = 'bruv:mainPanelWidth'
  const MIN_PANEL = 350
  let mainWidth = $state(Number(localStorage.getItem(MAIN_WIDTH_KEY)) || 720)
  let splitterDragging = $state(false)

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
  // Options editor dialog for select/radio/checkbox_group blocks
  async function openOptionsEditor(block: Block) {
    const blockType = block.type as 'select' | 'radio' | 'checkbox_group'
    const result = await showOptionsEditor(
      block.label || block.key || block.type,
      blockType,
      block.meta?.options || [],
      block.meta || {},
    )
    if (result && card) {
      block.meta = { ...block.meta, ...result.meta, options: result.options }
      await tracked(UpdateCardBlocks(cardId, card.blocks))
      onUpdated?.()
    }
  }

  function getEmptyValue(type: string): Block['value'] {
    switch (type) {
      case 'checklist': case 'list': case 'media': return []
      case 'checkbox_group': return []
      case 'number': case 'rating': case 'progress': return 0
      case 'checkbox': return false
      case 'divider': return null
      default: return ''
    }
  }

  function isBlockEmpty(block: Block): boolean {
    const v = block.value
    if (v === null || v === undefined) return true
    if (v === '' || v === 0 || v === false) return true
    if (Array.isArray(v) && v.length === 0) return true
    return false
  }

  async function clearBlockValue(block: Block) {
    if (!card) return
    block.value = getEmptyValue(block.type)
    if (block.type === 'alarm') block.meta = { ...block.meta, alarm_time: undefined, alarm_fired: false }
    await tracked(UpdateCardBlocks(cardId, card.blocks))
    onUpdated?.()
  }

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
  // CSS-only visual collapse during drag (avoids DOM restructure that kills drag state)
  let dragVisualCollapse = $state(false)

  function handleBlockDragStart(e: DragEvent, idx: number) {
    draggingBlockIdx = idx
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'copyMove'
      e.dataTransfer.setData('text/plain', String(idx))
    }
    // Use CSS-only collapse — no DOM changes, preserves drag state
    requestAnimationFrame(() => { dragVisualCollapse = true })
  }

  function handleBlockDragEnd() {
    draggingBlockIdx = null
    dropBlockIdx = null
    blockCopyMode = false
    dragVisualCollapse = false
    if (autoScrollRaf) {
      cancelAnimationFrame(autoScrollRaf)
      autoScrollRaf = null
    }
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

    // Auto-scroll when near edges
    startAutoScroll(e)
  }

  function handleBlockDragOverGap(e: DragEvent, gapIdx: number) {
    if (draggingBlockIdx === null) return
    e.preventDefault()
    e.stopPropagation()
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'
    dropBlockIdx = gapIdx

    // Auto-scroll when near edges
    startAutoScroll(e)
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

    draggingBlockIdx = null
    dropBlockIdx = null
    blockCopyMode = false
    dragVisualCollapse = false
    if (autoScrollRaf) { cancelAnimationFrame(autoScrollRaf); autoScrollRaf = null }

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
  let addBlockBtnEl = $state<HTMLButtonElement | null>(null)

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as Node
    if (showBlockPicker) {
      if (!addBlockBtnEl?.contains(target) && !(target as HTMLElement).closest?.('.add-block-picker')) showBlockPicker = false
    }
    if (showTypePicker) {
      if (!typePickerEl?.contains(target) && !(target as HTMLElement).closest?.('.type-picker-dropdown')) showTypePicker = false
    }
  }

  const BLOCK_OPTIONS = [
    { type: 'text',      label: t('block.text'),      icon: 'Type' },
    { type: 'checklist', label: t('block.checklist'), icon: 'ListChecks' },
    { type: 'list',      label: t('block.list'),      icon: 'List' },
    { type: 'media',     label: t('block.media'),     icon: 'Film' },
    { type: 'url',       label: t('block.url'),       icon: 'Link' },
    { type: 'divider',   label: t('block.divider'),   icon: 'Minus' },
    { type: 'select',    label: t('block.select'),    icon: 'ChevronDown' },
    { type: 'number',    label: t('block.number'),    icon: 'Hash' },
    { type: 'date',      label: t('block.date'),      icon: 'Calendar' },
    { type: 'rating',    label: t('block.rating'),    icon: 'Star' },
    { type: 'checkbox',       label: t('block.checkbox'),       icon: 'ToggleLeft' },
    { type: 'radio',          label: t('block.radio'),          icon: 'CircleDot' },
    { type: 'checkbox_group', label: t('block.checkbox_group'), icon: 'ListChecks' },
    { type: 'image',          label: t('block.image'),          icon: 'ImageIcon' },
    { type: 'progress',       label: t('block.progress'),       icon: 'ChartColumn' },
    { type: 'alarm',          label: t('block.alarm'),          icon: 'Bell' },
  ] as const

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell,
  }

  // --- Block collapse/expand ---
  let collapsedBlocks = $state<Set<string>>(new Set())
  // Expanded text blocks (override max-height scroll)
  let expandedTextBlocks = $state<Set<string>>(new Set())

  function toggleBlockCollapse(blockId: string) {
    const next = new Set(collapsedBlocks)
    if (next.has(blockId)) next.delete(blockId)
    else next.add(blockId)
    collapsedBlocks = next
  }

  function collapseAllBlocks() {
    if (!card) return
    collapsedBlocks = new Set(card.blocks.filter(b => b.key !== 'description').map(b => b.id))
  }

  function expandAllBlocks() {
    collapsedBlocks = new Set()
  }

  function toggleTextExpand(blockId: string) {
    const next = new Set(expandedTextBlocks)
    if (next.has(blockId)) next.delete(blockId)
    else next.add(blockId)
    expandedTextBlocks = next
  }

  // Persist collapsed state in block meta
  function syncCollapsedToMeta() {
    if (!card) return
    for (const block of card.blocks) {
      const isCollapsed = collapsedBlocks.has(block.id)
      if (!block.meta) block.meta = {}
      block.meta.collapsed = isCollapsed || undefined
    }
  }

  // Restore collapsed state from block meta on load
  function restoreCollapsedFromMeta() {
    if (!card) return
    const set = new Set<string>()
    for (const b of card.blocks) {
      if (b.meta?.collapsed) set.add(b.id)
    }
    collapsedBlocks = set
  }

  // --- Text block overflow detection ---
  let textBlockOverflows = $state<Set<string>>(new Set())
  let textBlockEls = $state<Record<string, HTMLDivElement | null>>({})

  function checkTextOverflow(blockId: string) {
    const el = textBlockEls[blockId]
    if (!el) return
    const next = new Set(textBlockOverflows)
    if (el.scrollHeight > el.clientHeight + 2) {
      next.add(blockId)
    } else {
      next.delete(blockId)
    }
    textBlockOverflows = next
  }

  // Re-check overflow when card loads or blocks change
  $effect(() => {
    if (!card) return
    const blocks = card.blocks
    // Small delay to let DOM render
    setTimeout(() => {
      for (const b of blocks) {
        if (b.type === 'text' && b.value) checkTextOverflow(b.id)
      }
    }, 50)
  })

  // --- Auto-scroll during drag ---
  let blocksListEl = $state<HTMLDivElement | null>(null)
  let autoScrollRaf = $state<number | null>(null)

  function startAutoScroll(e: DragEvent) {
    if (!blocksListEl) return
    const scrollContainer = blocksListEl.closest('.modal-body') as HTMLElement | null
    if (!scrollContainer) return

    const rect = scrollContainer.getBoundingClientRect()
    const edgeZone = 50
    const speed = 8

    if (e.clientY < rect.top + edgeZone) {
      scrollContainer.scrollTop -= speed
    } else if (e.clientY > rect.bottom - edgeZone) {
      scrollContainer.scrollTop += speed
    }
  }

  async function addBlock(blockType: string) {
    showBlockPicker = false
    if (!card) return
    const label = BLOCK_OPTIONS.find(o => o.type === blockType)?.label || blockType
    const id = `blk-${crypto.randomUUID().slice(0, 8)}`
    let value: Block['value'] = ''
    let meta: BlockMeta | undefined = undefined
    if (blockType === 'checklist') value = []
    else if (blockType === 'list') value = []
    else if (blockType === 'media') value = []
    else if (blockType === 'divider') value = null
    else if (blockType === 'select') { value = ''; meta = { options: ['Option 1', 'Option 2', 'Option 3'] } }
    else if (blockType === 'number') value = 0
    else if (blockType === 'date') value = ''
    else if (blockType === 'rating') { value = 0; meta = { max: 5 } }
    else if (blockType === 'checkbox') value = false
    else if (blockType === 'radio') { value = ''; meta = { options: ['Option 1', 'Option 2', 'Option 3'] } }
    else if (blockType === 'checkbox_group') { value = []; meta = { options: ['Option 1', 'Option 2', 'Option 3'] } }
    else if (blockType === 'image') value = null
    else if (blockType === 'progress') value = 0
    else if (blockType === 'alarm') { value = null; meta = { alarm_channels: 'in-app,system' } }

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
      restoreCollapsedFromMeta()
      if (autoEditTitle) editingTitle = true
      // Check if card has an agent configured
      try { const af = await GetAgentConfig(cardId); hasAgent = af?.config?.enabled ?? false } catch { hasAgent = false }
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

  async function refreshType() {
    if (!card?.type) return
    try {
      card = await tracked(RefreshTypeBlocks(cardId)) as Card
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

  // --- Keyboard navigation ---
  function handleBlockKeydown(e: KeyboardEvent, blockIdx: number) {
    if (e.key === 'Tab' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const blocks = card?.blocks.filter(b => b.key !== 'description') || []
      if (e.shiftKey) {
        if (blockIdx > 0) {
          e.preventDefault()
          const prevBlock = blocks[blockIdx - 1]
          focusBlock(prevBlock.id)
        }
      } else {
        if (blockIdx < blocks.length - 1) {
          e.preventDefault()
          const nextBlock = blocks[blockIdx + 1]
          focusBlock(nextBlock.id)
        }
      }
    }
  }

  function focusBlock(blockId: string) {
    setTimeout(() => {
      const el = document.querySelector(`[data-block-id="${blockId}"]`) as HTMLElement
      if (el) {
        const focusable = el.querySelector('textarea, input, [tabindex="0"]') as HTMLElement
        if (focusable) focusable.focus()
        else el.focus()
      }
    }, 0)
  }

  // --- Attachment handler ---
  function handleAttachmentUpdate(updatedCard: Card) {
    card = updatedCard
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
    await addTagBatch(newTag)
  }

  async function addTagBatch(rawInput: string) {
    const segments = rawInput.split(',').map(s => s.trim()).filter(Boolean)
    if (segments.length === 0) { newTag = ''; return }
    const existingLower = new Set((card?.tags || []).map((t: string) => t.toLowerCase()))
    const incoming = segments.filter(s => !existingLower.has(s.toLowerCase()))
    if (incoming.length === 0) { newTag = ''; return }
    const tags = [...(card?.tags || []), ...incoming]
    try {
      card = await tracked(UpdateCardTags(cardId, tags)) as Card
      newTag = ''
      for (const tag of incoming) await syncTagToProjects(tag)
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

  async function handleTagKeydown(e: KeyboardEvent) {
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
    } else if (e.key === ',') {
      // Comma immediately commits whatever is typed before it as a tag
      const before = newTag.split(',')[0]?.trim()
      if (before) {
        e.preventDefault()
        await addTagBatch(before)
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
        // Refresh accepted types for the new category
        try {
          acceptedTypes = (await GetCategoryAcceptedTypes(target.categoryId)) || undefined
        } catch { acceptedTypes = undefined }
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
  <div class="modal" class:chat-open={showChat} class:agent-tab-active={activeTab === 'agent' || activeTab === 'runs'} class:splitter-dragging={splitterDragging} use:draggable={{ handle: '.modal-header' }} use:focusTrap>
   <div class="modal-main" style={showChat ? `width: ${mainWidth}px;` : ''}>
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
          >{cardTypes.list.find(t => t.id === card.type)?.label || card.type || t('card.type_none')}{#if card.type}<span class="refresh-type-btn" role="button" tabindex="-1" onclick={(e) => { e.stopPropagation(); refreshType() }} title={t('tooltip.refresh_type')}><ArrowLeftRight size={10} /></span>{/if}<ChevronDown size={10} class="type-chevron" /></button>
          {#if showTypePicker && typeBadgeBtnEl}
            {@const filteredTypes = acceptedTypes?.length
              ? cardTypes.list.filter(ct => acceptedTypes!.includes(ct.id))
              : cardTypes.list}
            <div class="type-picker-dropdown" use:floatingDropdown={{ trigger: typeBadgeBtnEl }}>
              <button
                class="type-picker-option"
                class:active={!card.type}
                onclick={() => selectType('')}
              >
                <span class="type-option-badge" style="background: var(--bg-elevated); color: var(--text-muted)">{t('card.type_none')}</span>
              </button>
              {#each filteredTypes as ct}
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
            {t('card.other_pins')} ({otherPins.length}) {showOtherPins ? '▲' : '▼'}
          </button>

        {:else}
          <!-- No context category (inbox) — show pins list directly -->
          {#if pinBreadcrumbs.length === 0}
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}><MapPin size={11} /> {t('card.pin_to')}</button>
          {:else}
            {#each pinBreadcrumbs as pin}
              <div class="location-pin">
                <button class="location-breadcrumb" onclick={() => navigateToPinnedProject(pin)} title={t('tooltip.go_to_project')}><MapPin size={11} />{pin.breadcrumb}</button>
                <button class="btn-pin-action btn-unpin" onclick={() => handleUnpin(pin)} disabled={pinActionLoading} title={t('tooltip.unpin')} aria-label={t('tooltip.unpin')}><MapPinOff size={11} /></button>
              </div>
            {/each}
            <button class="btn-pin" onclick={openPinPicker} disabled={pinActionLoading}>{t('card.pin_to_category')}</button>
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

      <!-- Tab bar: Details / Agent -->
      <div class="card-tab-bar">
        <button class="card-tab" class:active={activeTab === 'details'} onclick={() => activeTab = 'details'}>{t('card.tab_details')}</button>
        <button class="card-tab" class:active={activeTab === 'agent'} onclick={() => activeTab = 'agent'}>
          {t('card.tab_agent')}
          {#if hasAgent}
            <span class="agent-dot"></span>
          {/if}
        </button>
        <button class="card-tab" class:active={activeTab === 'runs'} onclick={() => activeTab = 'runs'}>
          {t('card.tab_runs')}
        </button>
      </div>

      {#if activeTab === 'agent'}
        <div class="modal-body">
          <AgentTab {cardId} />
        </div>
      {:else if activeTab === 'runs'}
        <div class="modal-body">
          <AgentRunsTab {cardId} />
        </div>
      {:else}
      <div class="modal-body">
        <!-- Standard fields: compact 2-column grid + FAB in third column -->
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
                {@const icon = getTagIcon(tag)}
                <span class="tag-chip" style:background={getTagColor(tag)}>
                  {#if icon}
                    <span class="tag-chip-icon"><DynamicIcon name={icon} size={11} /></span>
                  {/if}
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

        <!-- Block toolbar: divider line with expand/collapse when multiple blocks -->
        <div class="block-toolbar-divider">
          {#if (card.blocks || []).filter(b => b.key !== 'description' && b.type !== 'divider').length > 1}
            <div class="block-toolbar-group">
              <button class="block-toolbar-btn" onclick={expandAllBlocks} title={t('block.expand_all')}>
                <ChevronsUpDown size={12} />
              </button>
              <button class="block-toolbar-btn" onclick={collapseAllBlocks} title={t('block.collapse_all')}>
                <ChevronsDownUp size={12} />
              </button>
            </div>
          {/if}
        </div>

        <!-- Blocks (excluding description block) -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="blocks-list" class:drag-visual-collapse={dragVisualCollapse} role="list" ondrop={handleBlockDrop} ondragover={(e) => { if (draggingBlockIdx !== null) e.preventDefault() }} bind:this={blocksListEl}>
          {#each (card.blocks || []) as block, blockIdx}
            {#if block.key !== 'description'}
              <!-- Drop indicator before this block -->
              {#if draggingBlockIdx !== null && dropBlockIdx === blockIdx}
                <div class="block-drop-indicator" class:copy-mode={blockCopyMode}></div>
              {/if}

              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="block-wrapper"
                class:block-collapsed={collapsedBlocks.has(block.id)}
                role="listitem"
                class:block-dragging={draggingBlockIdx === blockIdx}
                ondragover={(e) => handleBlockDragOver(e, blockIdx)}
                onkeydown={(e) => handleBlockKeydown(e, blockIdx)}
                data-block-id={block.id}
              >
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="block-drag-handle"
                  role="presentation"
                  tabindex={-1}
                  draggable={true}
                  ondragstart={(e) => handleBlockDragStart(e, blockIdx)}
                  ondragend={handleBlockDragEnd}
                  title={t('tooltip.drag_block')}
                ><GripVertical size={14} /></div>

                <section class="section block-content">
                  <!-- Editable block label with collapse toggle -->
                  {#if editingBlockLabelId === block.id}
                    <input
                      class="block-label-input"
                      use:focusOnMount={true}
                      bind:value={blockLabelDraft}
                      use:inlineEdit={{ onCommit: () => renameBlockLabel(block.id), onCancel: () => { editingBlockLabelId = null } }}
                    />
                  {:else}
                    <div class="section-title block-label-row action-reveal-parent">
                      {#if block.type !== 'divider'}
                        <button class="block-collapse-btn" onclick={() => toggleBlockCollapse(block.id)} title={collapsedBlocks.has(block.id) ? t('tooltip.expand_block') : t('tooltip.collapse_block')}>
                          {#if collapsedBlocks.has(block.id)}<ChevronRight size={14} />{:else}<ChevronDown size={14} />{/if}
                        </button>
                      {/if}
                      <!-- svelte-ignore a11y_no_static_element_interactions -->
                      <span class="block-label-text" tabindex={0} role="button" onclick={() => { editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' } }}>{block.label || block.key || block.type}</span>
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
                          <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); openOptionsEditor(block) }} title={t('block.edit_options')}><ListTree size={11} /></button>
                        {/if}
                        {#if block.type !== 'divider' && !isBlockEmpty(block)}
                          <button class="block-action-btn action-reveal" onclick={(e) => { e.stopPropagation(); clearBlockValue(block) }} title={t('tooltip.clear_block')}><X size={11} /></button>
                        {/if}
                        <button class="block-action-btn action-reveal action-reveal--edit" onclick={(e) => { e.stopPropagation(); editingBlockLabelId = block.id; blockLabelDraft = block.label || '' }} title={t('tooltip.rename_block')}><Pencil size={11} /></button>
                        <button class="block-action-btn action-reveal action-reveal--danger" onclick={(e) => { e.stopPropagation(); deleteBlock(block.id) }} title={t('tooltip.delete_block')}><Trash2 size={11} /></button>
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
                            onkeydown={(e) => handleTextBlockKeydown(e, block.id)}
                            oninput={(e) => handleTextBlockInput(e, block.id)}
                            onblur={() => { if (!mentionVisible) saveTextBlock(block.id) }}
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
                            <button class="text-expand-btn" onclick={() => toggleTextExpand(block.id)}>
                              <Maximize2 size={11} /> {t('block.scroll_expand')}
                            </button>
                          {/if}
                          {#if expandedTextBlocks.has(block.id)}
                            <button class="text-expand-btn" onclick={() => toggleTextExpand(block.id)}>
                              {t('block.collapse')}
                            </button>
                          {/if}
                        {/if}

                      {:else if block.type === 'checklist'}
                        <EditableChecklist
                          items={Array.isArray(block.value) ? block.value as ChecklistItem[] : []}
                          onUpdate={async (updated) => {
                            if (!card) return
                            const blocks = card.blocks.map((b: Block) => b.id === block.id ? { ...b, value: updated } : b)
                            try {
                              card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
                              onUpdated?.()
                            } catch (e) { showToast(t('error.save_failed'), 'error') }
                          }}
                          onPromote={async (text) => {
                            try {
                              const newCard = await CreateCard(card?.type || 'task', text)
                              if (newCard && currentCategoryId) {
                                await PinCard(newCard.id, currentCategoryId, currentCategoryId)
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
                            const blocks = card.blocks.map((b: Block) => b.id === block.id ? { ...b, value: updated } : b)
                            try {
                              card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
                              onUpdated?.()
                            } catch (e) { showToast(t('error.save_failed'), 'error') }
                          }}
                        />

                      {:else if block.type === 'media'}
                        <MediaBlock
                          items={Array.isArray(block.value) ? block.value as MediaItem[] : []}
                          onUpdate={async (updated) => {
                            if (!card) return
                            const blocks = card.blocks.map((b: Block) => b.id === block.id ? { ...b, value: updated } : b)
                            try {
                              card = await tracked(UpdateCardBlocks(cardId, blocks)) as Card
                              onUpdated?.()
                            } catch (e) { showToast(t('error.save_failed'), 'error') }
                          }}
                        />

                      {:else if block.type === 'url'}
                        {#if block.value}
                          <a href={String(block.value)} target="_blank" rel="noopener" class="block-link">{String(block.value)}</a>
                        {:else}
                          <span class="block-value">—</span>
                        {/if}

                      {:else if block.type === 'divider'}
                        <hr class="block-divider" />

                      {:else if block.type === 'select'}
                        <SelectBlock
                          value={block.value as string | string[]}
                          meta={block.meta || { options: [] }}
                          onUpdate={(val, newMeta) => {
                            if (!card) return
                            block.value = val
                            if (newMeta) block.meta = { ...block.meta, ...newMeta }
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'number'}
                        <NumberBlock
                          value={block.value as number | null}
                          meta={block.meta || {}}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'date'}
                        <DateBlock
                          value={block.value as string | null}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'rating'}
                        <RatingBlock
                          value={(block.value as number) || 0}
                          meta={block.meta || {}}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />

                      {:else if block.type === 'checkbox'}
                        <CheckboxBlock
                          value={!!block.value}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'radio'}
                        <RadioBlock
                          value={(block.value as string) || ''}
                          meta={block.meta || { options: [] }}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'checkbox_group'}
                        <CheckboxGroupBlock
                          value={(block.value as string[]) || []}
                          meta={block.meta || { options: [] }}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'image'}
                        <ImageBlock
                          value={block.value as string | { url: string; caption?: string } | null}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'progress'}
                        <ProgressBlock
                          value={(block.value as number) || 0}
                          onUpdate={(val) => {
                            if (!card) return
                            block.value = val
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />
                      {:else if block.type === 'alarm'}
                        <AlarmBlock
                          value={block.value as string | null}
                          meta={block.meta || {}}
                          onUpdate={(val, newMeta) => {
                            if (!card) return
                            block.value = val
                            if (newMeta) block.meta = { ...block.meta, ...newMeta }
                            tracked(UpdateCardBlocks(cardId, card.blocks))
                            onUpdated?.()
                          }}
                        />

                      {:else}
                        <!-- Legacy/unknown block type: show value as read-only text -->
                        <span class="block-value">{block.value ?? ''}</span>
                      {/if}
                    </div>
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

      <!-- Card-level attachments (pinned between scrollable body and footer) -->
      <div class="card-attachments-pinned">
        <div class="add-block-toolbar">
          <button class="add-block-btn" bind:this={addBlockBtnEl} onclick={() => showBlockPicker = !showBlockPicker} title={t('tooltip.add_block')}>
            <Plus size={12} />
            <span>{t('tooltip.add_block')}</span>
          </button>
          {#if showBlockPicker && addBlockBtnEl}
            <div class="add-block-picker" use:floatingDropdown={{ trigger: addBlockBtnEl }}>
              {#each BLOCK_OPTIONS as opt}
                {@const Icon = BLOCK_ICON_MAP[opt.icon]}
                <button class="block-picker-item" onclick={() => { addBlock(opt.type); showBlockPicker = false }} title={opt.label}>
                  <Icon size={14} />
                  <span>{opt.label}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <CardAttachments
          {cardId}
          attachments={card.file_attachments || []}
          onCardUpdated={handleAttachmentUpdate}
        />
      </div>
      {/if}

      <div class="modal-footer">
        <button class="btn-delete" onclick={handleDelete} title={t('tooltip.delete_card')}><Trash2 size={14} /> {t('card.delete')}</button>
        <span class="modal-footer-right">
          <SaveIndicator {saving} />
          <span class="meta">{t('card.created')} {card.created_at?.slice(0, 10) || '—'}</span>
        </span>
      </div>
    {/if}
    {#if showChat}
      <button class="chat-toggle-btn chat-toggle-docked active" onclick={() => showChat = !showChat} title={t('tooltip.toggle_chat')}><BotMessageSquare size={16} /></button>
    {/if}
   </div>

    {#if !loading && card}
      <ChatSection {cardId} bind:visible={showChat} bind:mainWidth bind:splitterDragging onCardChanged={loadCard} />
    {/if}

    <div class="modal-actions">
      {#if !showChat}
        <button class="chat-toggle-btn" onclick={() => showChat = !showChat} title={t('tooltip.toggle_chat')}><BotMessageSquare size={16} /></button>
      {/if}
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
    width: 720px;
    max-width: 95vw;
    max-height: 85vh;
    display: flex;
    flex-direction: row;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
    position: relative;
  }
  .modal.agent-tab-active {
    width: 880px;
  }
  .modal.chat-open {
    width: auto;
  }
  .modal.splitter-dragging {
    user-select: none;
  }

  .modal-main {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-width: 350px;
    position: relative;
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

  .card-tab-bar {
    display: flex;
    gap: 0;
    border-bottom: 1px solid var(--border-muted);
    padding: 0 1.25rem;
    flex-shrink: 0;
  }
  .card-tab {
    padding: 0.4rem 0.75rem;
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-muted);
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .card-tab:hover { color: var(--text); }
  .card-tab.active {
    color: var(--text);
    border-bottom-color: var(--accent);
  }

  .agent-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    margin-left: 0.3rem;
    vertical-align: middle;
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
    transition: opacity 0.15s, border-color 0.15s;
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
    transition: color 0.15s, background 0.15s;
    flex-shrink: 0;
  }
  .refresh-type-btn:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.2);
  }
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
    top: 0.55rem;
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

  .chat-toggle-docked {
    position: absolute;
    top: 0.55rem;
    right: 0.5rem;
    z-index: 6;
  }

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

  .tag-chip-icon {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
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

  .card-attachments-pinned {
    flex-shrink: 0;
    padding: 0 1.25rem;
    border-top: 1px solid var(--border-muted);
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

  .block-toolbar-divider {
    position: relative;
    height: 0;
    border-top: 1px solid var(--border-muted);
    margin: 0.75rem 0;
  }
  .block-toolbar-group {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    gap: 0.15rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    padding: 0 0.1rem;
  }
  .block-toolbar-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.1rem;
    border: none;
    width: 18px;
    height: 18px;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.12s, background 0.12s;
  }
  .block-toolbar-btn:hover {
    color: var(--text-strong);
    background: var(--bg-elevated);
  }

  .add-block-toolbar {
    position: relative;
    height: 0;
  }
  .add-block-btn {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #22c55e;
    border: 1px solid #22c55e;
    border-radius: 4px;
    padding: 0.15rem 0.55rem 0.15rem 0.35rem;
    color: #fff;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
    transition: background 0.15s, transform 0.1s;
    z-index: 1;
    box-shadow: 0 1px 4px rgba(0,0,0,0.15);
  }
  .add-block-btn:hover {
    background: #16a34a;
    border-color: #16a34a;
  }

  :global(.add-block-picker) {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.35rem;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    min-width: 220px;
  }

  :global(.add-block-picker .block-picker-item) {
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
  :global(.add-block-picker .block-picker-item:hover) { background: var(--bg-elevated); color: var(--text-primary); }

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
    gap: var(--block-gap, 0.75rem);
  }

  .block-wrapper {
    display: flex;
    align-items: flex-start;
    gap: 0;
    position: relative;
    transition: opacity 0.15s;
    border-radius: 6px;
  }
  .block-wrapper:focus-within {
    outline: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    outline-offset: 2px;
  }

  .block-wrapper.block-collapsed .block-content {
    /* Collapsed: only show the header */
  }

  .block-wrapper.block-dragging {
    opacity: 0.35;
  }

  /* CSS-only visual collapse during drag — hides block bodies without removing DOM nodes */
  .blocks-list.drag-visual-collapse .block-wrapper:not(.block-dragging) .block-body {
    max-height: 0;
    overflow: hidden;
    opacity: 0;
    margin: 0;
    padding: 0;
    transition: max-height 0.15s ease, opacity 0.1s ease;
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
    transition: background 0.1s, color 0.1s;
  }
  .block-collapse-btn:hover {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }

  .block-body {
    padding-left: 22px; /* indent content past the drag-handle column */
  }

  .text-scroll-wrap {
    position: relative;
  }

  .desc-display.text-scroll {
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
