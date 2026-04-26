<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardType, RefreshTypeBlocks, UpdateCardFields, UpdateCardBlocks, UpdateCardTags, UpdateCardDueDate,
    DeleteCard, PinCard, UnpinCard, GetCardPinBreadcrumbs, AddProjectLabel, GetProjectLabels, GetCategoryAcceptedTypes, GetAgentConfig } from '../lib/api'
  import { onEvent } from '../lib/events'
  import { projectTags, nav, getTagColor, getTagIcon, cardTypes } from '../lib/store.svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import { X, Trash2, Plus, Type, ListChecks, List, Film, Link, Minus, BotMessageSquare, ChevronDown, ChevronsUpDown, ChevronsDownUp, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell, ClipboardList } from 'lucide-svelte'
  import { renderMarkdown } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import PinPicker from './PinPicker.svelte'
  import PinPanel from './PinPanel.svelte'
  import BlockItem from './BlockItem.svelte'
  import CardHeader from './CardHeader.svelte'
  import DescriptionSection from './DescriptionSection.svelte'
  import ChatSection from './ChatSection.svelte'
  import AgentTab from './AgentTab.svelte'
  import AgentRunsTab from './AgentRunsTab.svelte'
  import CardAttachments from './CardAttachments.svelte'
  import { showOptionsEditor } from '../lib/optionsEditor.svelte'
  import CardComments from './CardComments.svelte'
  import SaveIndicator from './SaveIndicator.svelte'
  import { draggable } from '../lib/draggable'
  import { computeReorder, wouldReorder, DROP_END as REORDER_END } from '../lib/reorder'
  import { fade } from 'svelte/transition'
  import { focusOnMount, focusTrap, floatingDropdown } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, Block, BlockMeta, CardPin } from '../lib/types'


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
  // notFound = the card doesn't exist (or no longer exists). Hit when
  // a stale link in Inbox / Activity points at a deleted card. We
  // surface a friendly "Card no longer exists" panel rather than an
  // empty modal + cryptic toast — the user knows immediately why
  // nothing's loading and can close out.
  let notFound = $state(false)
  let pinBreadcrumbs = $state<CardPin[]>([])
  // svelte-ignore state_referenced_locally
  let acceptedTypes = $state<string[] | undefined>(categoryAcceptedTypes)
  let hasAgent = $state(false)
  let showPinPicker = $state(false)
  let pinPickerMode = $state<'pin' | 'move'>('pin')
  let pinPickerSourcePin = $state<CardPin | null>(null)
  let pinActionLoading = $state(false)
  let showOtherPins = $state(false)
  const CHAT_VISIBLE_KEY = 'bruv:chatPanelVisible'
  let showChat = $state(localStorage.getItem(CHAT_VISIBLE_KEY) === 'true')
  // Tracks whether the chat panel is physically present in the DOM —
  // this lags behind `showChat` during the close animation so the
  // modal's `.chat-open` class and modal-main's inline width stay in
  // effect while the panel is animating out. Without this the modal
  // snaps to its narrow width the instant the user clicks close, then
  // the panel animates in the new narrow layout, which looks wrong.
  // Must match the `slideOutWidth` duration in ChatSection.svelte.
  const CHAT_OUT_DURATION = 240
  // svelte-ignore state_referenced_locally
  let chatInDom = $state(showChat)
  $effect(() => {
    if (showChat) {
      chatInDom = true
    } else if (chatInDom) {
      const timer = setTimeout(() => { chatInDom = false }, CHAT_OUT_DURATION)
      return () => clearTimeout(timer)
    }
  })
  // Persist the active tab across component remounts and app restarts.
  // Without this, any event that causes CardDetail to re-render from
  // scratch (e.g. an agent completing a run and the parent refreshing
  // the board) bounces the user back to Details — confusing mid-flow.
  // Follows the same pattern as CHAT_VISIBLE_KEY above.
  const ACTIVE_TAB_KEY = 'bruv:cardDetailTab'
  function readStoredTab(): 'details' | 'agent' | 'runs' | null {
    const v = localStorage.getItem(ACTIVE_TAB_KEY)
    return v === 'details' || v === 'agent' || v === 'runs' ? v : null
  }
  // svelte-ignore state_referenced_locally
  let activeTab = $state<'details' | 'agent' | 'runs'>(initialTab ?? readStoredTab() ?? 'details')
  $effect(() => { localStorage.setItem(CHAT_VISIBLE_KEY, String(showChat)) })
  $effect(() => { localStorage.setItem(ACTIVE_TAB_KEY, activeTab) })

  // Splitter: redistributes space between main and chat when chat is open.
  // Default is 880px so Details/Agent/Runs tabs all render at the same
  // width — users used to see a jump from 720→880 when switching to the
  // Agent or Runs tabs, which was distracting.
  const MAIN_WIDTH_KEY = 'bruv:mainPanelWidth'
  let mainWidth = $state(Number(localStorage.getItem(MAIN_WIDTH_KEY)) || 880)
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

  // Block drag-and-drop state, keyed by stable block.id rather than
  // array index. Indices shift on reorder/delete (see CLAUDE.md); IDs
  // don't. dropBeforeBlockId either holds the id of the block the
  // drop indicator appears ABOVE, the sentinel DROP_END for drop-
  // after-last, or null when no drop target is active.
  const DROP_END = REORDER_END
  let draggingBlockId = $state<string | null>(null)
  let dropBeforeBlockId = $state<string | null>(null)
  let blockCopyMode = $state(false)
  // CSS-only visual collapse during drag (avoids DOM restructure that kills drag state)
  let dragVisualCollapse = $state(false)

  function handleBlockDragStart(e: DragEvent, block: Block) {
    draggingBlockId = block.id
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'copyMove'
      e.dataTransfer.setData('text/plain', block.id)
    }
    // Use CSS-only collapse — no DOM changes, preserves drag state
    requestAnimationFrame(() => { dragVisualCollapse = true })
  }

  function handleBlockDragEnd() {
    draggingBlockId = null
    dropBeforeBlockId = null
    blockCopyMode = false
    dragVisualCollapse = false
    if (autoScrollRaf) {
      cancelAnimationFrame(autoScrollRaf)
      autoScrollRaf = null
    }
  }

  function handleBlockDragOver(e: DragEvent, block: Block, idx: number) {
    if (draggingBlockId === null) return
    e.preventDefault()
    e.stopPropagation()
    blockCopyMode = e.ctrlKey
    if (e.dataTransfer) e.dataTransfer.dropEffect = e.ctrlKey ? 'copy' : 'move'

    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    const blocks = card?.blocks || []
    let candidate: string | typeof DROP_END
    if (e.clientY < midY) {
      candidate = block.id
    } else {
      // Below midpoint = insert AFTER this block = before the next
      // block, or at the end of the list if this is the last block.
      const nextBlock = blocks[idx + 1]
      candidate = nextBlock ? nextBlock.id : DROP_END
    }

    // Only paint the drop indicator when the drop would actually
    // change the order — otherwise the user sees a line that does
    // nothing on release, which reads as a bug. (Covers "drop on
    // self" and "drop into the slot immediately after self".)
    const mode: 'move' | 'copy' = e.ctrlKey ? 'copy' : 'move'
    dropBeforeBlockId = wouldReorder(blocks, draggingBlockId, candidate, mode) ? candidate : null

    // Auto-scroll when near edges
    startAutoScroll(e)
  }

  async function handleBlockDrop(e: DragEvent) {
    e.preventDefault()
    if (draggingBlockId === null || dropBeforeBlockId === null || !card) {
      handleBlockDragEnd()
      return
    }

    const copy = e.ctrlKey
    const fromId = draggingBlockId
    const toTarget = dropBeforeBlockId

    draggingBlockId = null
    dropBeforeBlockId = null
    blockCopyMode = false
    dragVisualCollapse = false
    if (autoScrollRaf) { cancelAnimationFrame(autoScrollRaf); autoScrollRaf = null }

    const blocks = computeReorder(card.blocks, fromId, toTarget, {
      mode: copy ? 'copy' : 'move',
      newId: () => `blk-${crypto.randomUUID().slice(0, 8)}`,
    })
    // computeReorder returns the same reference on a no-op; skip the save.
    if (blocks === card.blocks) return

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
    { type: 'survey',         label: t('block.survey'),         icon: 'ClipboardList' },
  ] as const

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const BLOCK_ICON_MAP: Record<string, any> = {
    Type, ListChecks, List, Film, Link, Minus, ChevronDown, Hash, Calendar, Star, ToggleLeft, CircleDot, ImageIcon, ChartColumn, Bell, ClipboardList,
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
    else if (blockType === 'survey') value = []

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

  // Reload the open card when the backend signals it was modified out-of-band
  // (agent run, alarm fire, etc). The effect re-binds whenever `cardId` changes
  // so the listener is always scoped to the currently-displayed card.
  //
  // We avoid clobbering active edits by skipping the reload while the user is
  // mid-edit on the title, description, or any block — they'll pick up the
  // change when they finish editing or navigate away.
  $effect(() => {
    const watchedCardId = cardId
    const unsubscribe = onEvent<{ cardID?: string }>('card:updated', (data) => {
      if (!data || data.cardID !== watchedCardId) return
      if (editingTitle || editingDescription || editingBlockId !== null || editingBlockLabelId !== null) return
      // Silent refresh — don't wipe the visible card with the loading
      // placeholder. Without this, the dialog flashes whenever the agent
      // writes an update to the card mid-run.
      loadCard(true)
    })
    return () => {
      if (typeof unsubscribe === 'function') unsubscribe()
    }
  })

  async function loadCard(silent: boolean = false) {
    if (!silent) loading = true
    try {
      card = await GetCard(cardId) as Card
      notFound = false
      pinBreadcrumbs = await GetCardPinBreadcrumbs(cardId) || []
      titleDraft = card.title
      descriptionDraft = card.fields?.description || ''
      blockDrafts = {}
      newChecklistTexts = {}
      for (const b of card.blocks) {
        if (b.type === 'text' || b.type === 'url') blockDrafts[b.id] = String(b.value ?? '')
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
      // Backend returns "card %q not found" for missing cards (the
      // common case from a stale Inbox / Activity link). Treat any
      // failed load as not-found from the user's perspective —
      // network / auth blips are rare and the not-found panel
      // points the user at the same recovery action (close + try
      // something else) for both cases.
      card = null
      notFound = true
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
      // Save explicitly. The previous "mount the description
      // textarea, let focusOnMount blur the title, fire saveTitle
      // via onblur" dance worked only when the Details tab was
      // active — on the Agent / Runs tab the description block isn't
      // rendered, so no textarea mounts, no blur fires, and the user
      // is stuck with an unsavable title.
      await saveTitle()
      // After save, if the user is on Details, follow up by focusing
      // the description for the natural top-down editing flow.
      if (activeTab === 'details') editingDescription = true
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

  async function saveUrlBlock(blockId: string) {
    if (editingBlockId !== blockId || !card) return
    const raw = (blockDrafts[blockId] ?? '').trim()
    const block = card.blocks.find((b: Block) => b.id === blockId)
    // Normalise: add https:// if the user typed a bare host. Empty is fine — stores as empty.
    const draft = raw && !/^https?:\/\//i.test(raw) && !raw.startsWith('/') ? `https://${raw}` : raw
    if (draft === String(block?.value ?? '')) {
      editingBlockId = null
      return
    }
    const updatedBlocks = card.blocks.map((b: Block) =>
      b.id === blockId && b.type === 'url' ? { ...b, value: draft } : b
    )
    try {
      card = await tracked(UpdateCardBlocks(cardId, updatedBlocks)) as Card
      blockDrafts[blockId] = draft
      editingBlockId = null
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
<div class="modal-backdrop" role="presentation" onclick={handleBackdropClick} out:fade={{ duration: 150 }}>
  <div class="modal" class:chat-open={chatInDom} class:splitter-dragging={splitterDragging} style:--modal-base="{mainWidth}px" use:draggable={{ handle: '.modal-header' }} use:focusTrap>
   <div class="modal-main" style={chatInDom ? `width: ${mainWidth}px;` : ''}>
    {#if loading}
      <div class="modal-loading">{t('app.loading')}</div>
    {:else if notFound}
      <div class="modal-not-found">
        <h2>{t('error.card_not_found_title')}</h2>
        <p>{t('error.card_not_found_body')}</p>
        <button class="btn-primary" onclick={() => onClose?.()}>{t('error.card_not_found_close')}</button>
      </div>
    {:else if card}
      <CardHeader
        {card}
        cardTypesList={cardTypes.list}
        {acceptedTypes}
        bind:editingTitle
        bind:titleDraft
        bind:showTypePicker
        bind:typePickerEl
        bind:typeBadgeBtnEl
        onSaveTitle={saveTitle}
        onTitleKeydown={handleTitleKeydown}
        onOpenTypePicker={openTypePicker}
        onSelectType={selectType}
        onRefreshType={refreshType}
      />

      <PinPanel
        {pinBreadcrumbs}
        {currentPin}
        {isPinnedHere}
        {otherPins}
        {effectiveCategoryId}
        {effectiveCategoryName}
        {pinActionLoading}
        bind:showOtherPins
        onToggleCurrentPin={toggleCurrentPin}
        onOpenMovePicker={openMovePicker}
        onOpenPinPicker={openPinPicker}
        onUnpin={handleUnpin}
        onNavigateToPinnedProject={navigateToPinnedProject}
      />

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

      <div class="modal-body" hidden={activeTab !== 'agent'}>
        <AgentTab {cardId} />
      </div>
      <div class="modal-body" hidden={activeTab !== 'runs'}>
        <AgentRunsTab {cardId} />
      </div>
      <div class="modal-body" hidden={activeTab !== 'details'}>
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

        <DescriptionSection
          {card}
          bind:editingDescription
          bind:descriptionDraft
          bind:descTextareaEl
          onKeydown={handleDescKeydown}
          onInput={handleDescInput}
          onBlur={handleDescBlur}
        />

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
        <div class="blocks-list" class:drag-visual-collapse={dragVisualCollapse} role="list" ondrop={handleBlockDrop} ondragover={(e) => { if (draggingBlockId !== null) e.preventDefault() }} bind:this={blocksListEl}>
          {#each (card.blocks || []) as block, blockIdx (block.id)}
            {#if block.key !== 'description'}
              <!-- Drop indicator before this block -->
              {#if draggingBlockId !== null && dropBeforeBlockId === block.id}
                <div class="block-drop-indicator" class:copy-mode={blockCopyMode}></div>
              {/if}

              <BlockItem
                {block}
                {blockIdx}
                {card}
                {cardId}
                {currentCategoryId}
                bind:editingBlockId
                bind:editingBlockLabelId
                bind:blockLabelDraft
                bind:blockDrafts
                {collapsedBlocks}
                {expandedTextBlocks}
                {draggingBlockId}
                {mentionVisible}
                {textBlockOverflows}
                bind:blockTextareaEls
                bind:textBlockEls
                {tracked}
                {onUpdated}
                onDragStart={handleBlockDragStart}
                onDragEnd={handleBlockDragEnd}
                onDragOver={handleBlockDragOver}
                onKeydown={handleBlockKeydown}
                onToggleCollapse={toggleBlockCollapse}
                onRenameLabel={renameBlockLabel}
                onOpenOptionsEditor={openOptionsEditor}
                onClearValue={clearBlockValue}
                onDelete={deleteBlock}
                onTextKeydown={handleTextBlockKeydown}
                onTextInput={handleTextBlockInput}
                onSaveText={saveTextBlock}
                onSaveUrl={saveUrlBlock}
                onToggleTextExpand={toggleTextExpand}
                {isBlockEmpty}
              />
            {/if}
          {/each}

          <!-- Drop indicator after last block -->
          {#if draggingBlockId !== null && dropBeforeBlockId === DROP_END}
            <div class="block-drop-indicator" class:copy-mode={blockCopyMode}></div>
          {/if}
        </div>

        <!-- Card comments — first-class thread, not a block -->
        <CardComments {cardId} />

      </div>

      <!-- Card-level attachments (pinned between scrollable body and footer) -->
      {#if activeTab === 'details'}
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
      <!-- Silent reload on LLM replies — otherwise `loading` flips
           true, the outer {#if !loading && card} tears ChatSection
           out of the DOM, and its slide-in animation replays every
           time the assistant responds. -->
      <ChatSection {cardId} bind:visible={showChat} bind:mainWidth bind:splitterDragging onCardChanged={() => loadCard(true)} />
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
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .modal {
    background: var(--bg-surface);
    border-radius: 10px;
    /* Base width tracks the user's card-body preference (--modal-base,
       set inline from `mainWidth`). Default 880px so Details, Agent
       and Runs tabs all render at the same width — switching tabs no
       longer jumps the modal. This also makes the close animation's
       final state match the steady-state width, so the modal doesn't
       snap when .chat-open is removed at t=240ms. */
    width: var(--modal-base, 880px);
    max-width: 95vw;
    /* Fixed height (not max-height) so the dialog is a stable size
       regardless of which tab is active. Tab content scrolls
       internally via .modal-body's overflow-y: auto. */
    height: 85vh;
    display: flex;
    flex-direction: row;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
    position: relative;
    animation: fade-in-scale var(--duration-slow) var(--ease-out);
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
    animation: fade-in var(--duration-moderate) var(--ease-out);
  }

  .modal-not-found {
    padding: 2.5rem 2rem;
    text-align: center;
    animation: fade-in var(--duration-moderate) var(--ease-out);
  }
  .modal-not-found h2 {
    margin: 0 0 0.5rem;
    font-size: 1.05rem;
    color: var(--text-strong);
  }
  .modal-not-found p {
    margin: 0 0 1.25rem;
    color: var(--text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
    max-width: 32rem;
    margin-left: auto;
    margin-right: auto;
  }
  .modal-not-found .btn-primary {
    padding: 0.5rem 1.25rem;
    background: var(--accent);
    color: #fff;
    border: 1px solid transparent;
    border-radius: 6px;
    font: inherit;
    font-weight: 500;
    cursor: pointer;
  }
  .modal-not-found .btn-primary:hover { background: var(--accent-hover, var(--accent)); }

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
    transition: color var(--duration-normal) var(--ease-out),
                border-color var(--duration-moderate) var(--ease-out);
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
  .modal-body[hidden] {
    display: none;
  }

  /* Shared description/text-block styles. Marked :global so they
     apply both to the DescriptionSection component and to text
     blocks rendered inside BlockItem — both use the same .desc-*
     class names and should look identical. */
  :global(.section-title) {
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

  :global(.desc-display) {
    cursor: text;
    padding: 0.5rem;
    border-radius: 6px;
    transition: background var(--duration-fast);
  }
  :global(.desc-display:hover) { background: var(--bg-elevated); }
  :global(.desc-display p) { margin: 0; color: var(--text-body); font-size: 0.9rem; line-height: 1.5; }
  :global(.desc-display .placeholder) { white-space: pre-wrap; color: var(--text-faint); font-style: italic; }

  :global(.desc-textarea) {
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
  :global(.desc-textarea:focus) { border-color: var(--accent); }

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
    transition: background var(--duration-normal), transform var(--duration-fast);
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

  .blocks-list {
    display: flex;
    flex-direction: column;
    gap: var(--block-gap, 0.75rem);
  }

  .block-drop-indicator {
    height: 3px;
    background: var(--accent);
    border-radius: 2px;
    margin: 2px 22px 2px 22px;
    transition: background var(--duration-fast);
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
