<script lang="ts">
  /**
   * CardBlocks — the card's custom-blocks region. Owns:
   *   - the block toolbar (expand/collapse all + accordion mode)
   *   - the blocks list with drag-and-drop reorder/copy + drop indicators
   *   - per-block editing state (drafts, label rename, collapse/expand,
   *     text-overflow detection) and all block mutation logic
   *   - the @-mention picker for text-block and checklist inputs (the
   *     description's mention picker stays in CardDetail)
   *
   * Extracted from CardDetail. The parent provides `track` (save-
   * indicator wrapper) and receives updated cards via onCardUpdated;
   * `addBlock` / `restoreCollapsedFromMeta` / `resetDrafts` /
   * `hasActiveEdit` / `commitActiveEdit` are exported for the parent's
   * add-block button, load path, and modal-level keyboard handling.
   */
  import { UpdateCardBlocks } from '@shared/api'
  import { ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import BlockItem from './BlockItem.svelte'
  import { showOptionsEditor } from '../lib/optionsEditor.svelte'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, Block, BlockMeta } from '@shared/types'

  let { card, cardId, currentCategoryId, track, onCardUpdated, onUpdated, onClose }: {
    card: Card
    cardId: string
    currentCategoryId?: string | null
    track: <T>(promise: Promise<T>) => Promise<T>
    onCardUpdated: (card: Card) => void
    onUpdated?: () => void
    /** Ctrl+Enter inside a text block saves and closes the dialog. */
    onClose: () => void
  } = $props()

  // Options editor dialog for select/radio/checkbox_group blocks
  async function openOptionsEditor(block: Block) {
    const blockType = block.type as 'select' | 'radio' | 'checkbox_group'
    const result = await showOptionsEditor(
      block.label || block.key || block.type,
      blockType,
      block.meta?.options || [],
      block.meta || {},
    )
    if (result) {
      block.meta = { ...block.meta, ...result.meta, options: result.options }
      await track(UpdateCardBlocks(cardId, card.blocks))
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
    // Confirm before wiping the value — a one-click X next to a long
    // text block or a populated checklist is easy to hit by accident
    // and there's no undo path.
    const name = block.label || block.key || block.type
    if (!await showConfirm(t('card.clear_block_confirm').replace('{name}', name))) return
    block.value = getEmptyValue(block.type)
    if (block.type === 'alarm') block.meta = { ...block.meta, alarm_time: undefined, alarm_fired: false }
    await track(UpdateCardBlocks(cardId, card.blocks))
    onUpdated?.()
  }

  // Block label editing
  let editingBlockLabelId = $state<string | null>(null)
  let blockLabelDraft = $state('')

  // Block editing state: keyed by block ID (stable across reorders)
  let editingBlockId = $state<string | null>(null)
  let blockDrafts = $state<Record<string, string>>({})
  let blockTextareaEls = $state<Record<string, HTMLTextAreaElement | null>>({})
  let checklistInputEls = $state<Record<string, HTMLInputElement | null>>({})
  let newChecklistTexts = $state<Record<string, string>>({})

  /** True while a block value or label edit is in progress — the parent
   *  uses this to gate modal-level Escape/silent-reload behaviour. */
  export function hasActiveEdit(): boolean {
    return editingBlockId !== null || editingBlockLabelId !== null
  }

  /** Commit any in-progress block text / label edit (modal Ctrl+Enter). */
  export async function commitActiveEdit(): Promise<void> {
    if (editingBlockId !== null) await saveTextBlock(editingBlockId)
    if (editingBlockLabelId !== null) await renameBlockLabel(editingBlockLabelId)
  }

  /** Re-seed text/url drafts from the current card (called by the parent
   *  after every card load; also runs on mount below). */
  export function resetDrafts() {
    blockDrafts = {}
    newChecklistTexts = {}
    for (const b of card.blocks) {
      if (b.type === 'text' || b.type === 'url') blockDrafts[b.id] = String(b.value ?? '')
    }
  }

  // Block drag-and-drop state, keyed by stable block.id rather than
  // array index. Indices shift on reorder/delete (see CLAUDE.md); IDs
  // don't. dropBeforeBlockId either holds the id of the block the
  // drop indicator appears ABOVE, the sentinel DROP_END for drop-
  // after-last, or null when no drop target is active.
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
      // Compact drag ghost: snapshot just the block's header row instead
      // of the full expanded block. The native snapshot is taken before
      // the deferred visual collapse below, so without this an expanded
      // block drags as a huge ghost that obscures the drop targets.
      const headerEl = (e.target as HTMLElement)
        ?.closest?.('.block-wrapper')
        ?.querySelector('.block-label-row')
      if (headerEl instanceof HTMLElement) {
        e.dataTransfer.setDragImage(headerEl, 12, 12)
      }
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
    const blocks = card.blocks || []
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
    if (draggingBlockId === null || dropBeforeBlockId === null) {
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

    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, blocks))
      blockDrafts = {}
      for (const b of updated.blocks) {
        if (b.type === 'text') blockDrafts[b.id] = String(b.value ?? '')
      }
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  function labelToKey(label: string): string {
    return label.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
  }

  // --- Block collapse/expand + accordion mode ---
  //
  // Session-only collapsed state (Set<block.id>); accordion mode is
  // persisted across sessions in localStorage. Shared key with mobile
  // so the preference can converge through user prefs in future.
  // Dividers are always shown — BlockItem ignores their collapsed
  // flag — so we filter them out of bulk collapse / single-mode logic.
  const BLOCK_ACCORDION_MODE_KEY = 'bruv:blockAccordionMode'
  type AccordionMode = 'single' | 'multi'
  function readBlockAccordionMode(): AccordionMode {
    const v = localStorage.getItem(BLOCK_ACCORDION_MODE_KEY)
    return v === 'multi' ? 'multi' : 'single'
  }
  let collapsedBlocks = $state<Set<string>>(new Set())
  let blockAccordionMode = $state<AccordionMode>(readBlockAccordionMode())
  // Expanded text blocks (override max-height scroll)
  let expandedTextBlocks = $state<Set<string>>(new Set())

  function isCollapsibleBlock(b: Block): boolean {
    return b.key !== 'description' && b.type !== 'divider'
  }

  function toggleBlockCollapse(blockId: string) {
    const next = new Set(collapsedBlocks)
    if (next.has(blockId)) {
      // Expanding. In single mode, collapse every other collapsible
      // block first so only this one stays open.
      if (blockAccordionMode === 'single') {
        for (const b of card.blocks) {
          if (b.id !== blockId && isCollapsibleBlock(b)) next.add(b.id)
        }
      }
      next.delete(blockId)
    } else {
      next.add(blockId)
    }
    collapsedBlocks = next
  }

  function collapseAllBlocks() {
    collapsedBlocks = new Set(card.blocks.filter(b => b.key !== 'description').map(b => b.id))
  }

  function expandAllBlocks() {
    collapsedBlocks = new Set()
  }

  // Switching INTO single mode: keep the topmost currently-open block
  // open, collapse the rest. Switching to multi is a no-op.
  function applySingleModeCollapse() {
    const collapsible = card.blocks.filter(isCollapsibleBlock)
    const open = collapsible.filter(b => !collapsedBlocks.has(b.id))
    if (open.length <= 1) return
    const keepOpen = open[0].id
    const next = new Set(collapsedBlocks)
    for (const b of collapsible) {
      if (b.id !== keepOpen) next.add(b.id)
    }
    collapsedBlocks = next
  }

  function toggleAccordionMode() {
    blockAccordionMode = blockAccordionMode === 'single' ? 'multi' : 'single'
    try { localStorage.setItem(BLOCK_ACCORDION_MODE_KEY, blockAccordionMode) } catch { /* private mode, etc. */ }
    if (blockAccordionMode === 'single') applySingleModeCollapse()
  }

  function toggleTextExpand(blockId: string) {
    const next = new Set(expandedTextBlocks)
    if (next.has(blockId)) next.delete(blockId)
    else next.add(blockId)
    expandedTextBlocks = next
  }

  /** Restore collapsed state from block meta on load (parent calls this
   *  on non-silent reloads; also runs on mount below). */
  export function restoreCollapsedFromMeta() {
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

  export async function addBlock(blockType: Block['type'], label: string) {
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

    const newBlock: Block = { id, type: blockType, label, key: labelToKey(label), value, meta }
    const blocks = [...card.blocks, newBlock]
    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, blocks))
      if (blockType === 'text') {
        blockDrafts[id] = ''
        editingBlockId = id
      }
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  // @ mention picker state (text blocks + checklist inputs; the
  // description's mention picker lives in CardDetail)
  let mentionVisible = $state(false)
  let mentionAnchor = $state<{ top: number; left: number } | null>(null)
  let mentionTarget = $state<{ type: 'text'; blockId: string } | { type: 'checklist'; blockId: string } | null>(null)
  let mentionTriggerPos = $state<number>(0)

  async function deleteBlock(blockId: string) {
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (!block) return
    if (!await showConfirm(t('card.confirm_delete_block').replace('{name}', block.label || block.type))) return
    const blocks = card.blocks.filter((b: Block) => b.id !== blockId)
    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, blocks))
      blockDrafts = {}
      for (const b of updated.blocks) {
        if (b.type === 'text') blockDrafts[b.id] = String(b.value ?? '')
      }
    } catch (e) { showToast(t('error.delete_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  async function renameBlockLabel(blockId: string) {
    if (editingBlockLabelId !== blockId) return
    const label = blockLabelDraft.trim()
    editingBlockLabelId = null
    if (!label) return
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (!block || label === block.label) return
    const blocks = card.blocks.map((b: Block) => b.id === blockId ? { ...b, label, key: labelToKey(label) } : b)
    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, blocks))
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  async function saveUrlBlock(blockId: string) {
    if (editingBlockId !== blockId) return
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
    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, updatedBlocks))
      blockDrafts[blockId] = draft
      editingBlockId = null
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  async function saveTextBlock(blockId: string) {
    if (editingBlockId !== blockId) return
    const draft = blockDrafts[blockId]
    const block = card.blocks.find((b: Block) => b.id === blockId)
    if (draft === undefined || draft === String(block?.value ?? '')) {
      editingBlockId = null
      return
    }
    const updatedBlocks = card.blocks.map((b: Block) =>
      b.id === blockId && b.type === 'text' ? { ...b, value: draft } : b
    )
    let updated: Card
    try {
      updated = await track(UpdateCardBlocks(cardId, updatedBlocks))
      editingBlockId = null
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onCardUpdated(updated)
  }

  async function handleTextBlockKeydown(e: KeyboardEvent, blockId: string) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      editingBlockId = null
      const block = card.blocks.find((b: Block) => b.id === blockId)
      blockDrafts[blockId] = String(block?.value ?? '')
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()  // prevent the modal's backdrop keydown from also calling saveTextBlock
      await saveTextBlock(blockId)
      onClose()
    }
  }

  function handleTextBlockInput(e: Event, blockId: string) {
    const el = e.target as HTMLTextAreaElement
    checkForMention(el, { type: 'text', blockId })
  }

  function checkForMention(el: HTMLTextAreaElement | HTMLInputElement, target: { type: 'text'; blockId: string } | { type: 'checklist'; blockId: string }) {
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
    if (mentionTarget.type === 'text') {
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
    if (target?.type === 'text') {
      setTimeout(() => blockTextareaEls[target.blockId]?.focus(), 0)
    } else if (target?.type === 'checklist') {
      setTimeout(() => checklistInputEls[target.blockId]?.focus(), 0)
    }
  }

  // --- Keyboard navigation ---
  function handleBlockKeydown(e: KeyboardEvent, blockIdx: number) {
    if (e.key === 'Tab' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const blocks = card.blocks.filter(b => b.key !== 'description')
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

  // Seed drafts and collapsed-from-meta state on mount. The component
  // mounts after each non-silent card load (the loading placeholder
  // tears it down), and the parent's bind:this ref isn't set yet during
  // the first load — so the component seeds itself here; the parent
  // calls resetDrafts()/restoreCollapsedFromMeta() on later reloads.
  resetDrafts()
  restoreCollapsedFromMeta()
</script>

<!-- Block toolbar: divider line with expand/collapse + single/multi mode when multiple blocks -->
<div class="block-toolbar-divider">
  {#if (card.blocks || []).filter(b => b.key !== 'description' && b.type !== 'divider').length > 1}
    <div class="block-toolbar-group">
      <button class="block-toolbar-btn" onclick={expandAllBlocks} title={t('block.expand_all')}>
        <ChevronsUpDown size={12} />
      </button>
      <button class="block-toolbar-btn" onclick={collapseAllBlocks} title={t('block.collapse_all')}>
        <ChevronsDownUp size={12} />
      </button>
      <button
        class="block-toolbar-btn"
        onclick={toggleAccordionMode}
        aria-label={blockAccordionMode === 'single' ? t('block.mode_single') : t('block.mode_multi')}
        title={blockAccordionMode === 'single' ? t('block.mode_single_hint') : t('block.mode_multi_hint')}
      >
        {#if blockAccordionMode === 'single'}
          <ListCollapse size={12} />
        {:else}
          <ListTree size={12} />
        {/if}
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
        tracked={track}
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

<MentionPicker
  visible={mentionVisible}
  anchor={mentionAnchor}
  onSelect={handleMentionSelect}
  onClose={handleMentionClose}
/>

<style>
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
</style>
