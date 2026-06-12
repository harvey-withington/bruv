<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, projectURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { Trash2, MapPin, Plus, X, RefreshCw, Search, Paperclip, MessageSquare, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, Copy, FileJson } from 'lucide-svelte'
  import { cardToMarkdown } from '@shared/cardMarkdown'
  import { downloadBlob, sanitizeFilenameStem } from '@shared/download'
  import type { CardComment } from '@shared/types'
  import { buildCardExportPayload, cardMarkdownLabels } from '../lib/cardExport'
  import EditableText from '../components/EditableText.svelte'
  import EditableDescription from '../components/EditableDescription.svelte'
  import TagsEditor from '../components/TagsEditor.svelte'
  import BlockEditor from '../components/blocks/BlockEditor.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import PinPicker from '../components/PinPicker.svelte'
  import CardTypePicker from '../components/CardTypePicker.svelte'
  import CommentsSection from '../components/CommentsSection.svelte'
  import AttachmentsSection from '../components/AttachmentsSection.svelte'
  import SearchSheet from '../components/SearchSheet.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { repoMeta, loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import { onEvent } from '../lib/events.svelte'
  import { dragSortable, type DragMoveDetail } from '../lib/actions/dnd.svelte'
  import type { Block, Card, CardPin } from '@shared/types'

  // Card view + basic edit. Phase 1 scope:
  //   - title  (editable inline)
  //   - tags   (editable chip list)
  //   - blocks (rendered read-only via BlockView)
  //   - type, due_date  (display only — date pickers + type pickers
  //     are their own UX questions, not v1 critical)
  //
  // Block editing, comments, attachments, pin/unpin, agent config —
  // all later passes. Mobile-first, but mobile-not-everything.

  let { id }: { id: string } = $props()

  let card = $state<Card | null>(null)
  let projectKey = $state<string | undefined>(undefined)
  let pins = $state<CardPin[]>([])
  let pinPickerOpen = $state(false)
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let saveError = $state<string | null>(null)
  let activeMetaTab = $state<'attachments' | 'comments'>('attachments')
  let commentCount = $state(0)

  // Block save state. `lastSavedBlocks` is the last snapshot the
  // server confirmed; on a save failure we revert to it. The single
  // debounce timer coalesces rapid edits (e.g. typing) into one
  // persistence call. 200ms is short enough that taps feel
  // instantaneous and long enough that a textarea's per-keystroke
  // change events don't fire one save per character.
  let lastSavedBlocks = $state<Block[]>([])
  let blockSaveTimer: ReturnType<typeof setTimeout> | null = null
  let savingBlocks = $state(false)
  let savedFlash = $state(false)
  let savedFlashTimer: ReturnType<typeof setTimeout> | null = null

  // --- Block expand/collapse + accordion mode ---
  //
  // Session-only collapsed state, keyed by stable block.id (per
  // CLAUDE.md — never index-based). Mode preference (single vs multi)
  // persists across cards/sessions in localStorage; same storage key
  // as the desktop equivalent so the choice carries across platforms
  // when both use the shared backing user prefs in future.
  const BLOCK_ACCORDION_MODE_KEY = 'bruv:blockAccordionMode'
  type AccordionMode = 'single' | 'multi'
  function readBlockAccordionMode(): AccordionMode {
    const v = typeof localStorage !== 'undefined' ? localStorage.getItem(BLOCK_ACCORDION_MODE_KEY) : null
    return v === 'multi' ? 'multi' : 'single'
  }
  let collapsedBlocks = $state<Set<string>>(new Set())
  let blockAccordionMode = $state<AccordionMode>(readBlockAccordionMode())

  function isCollapsibleBlock(b: Block): boolean {
    return b.type !== 'divider'
  }

  // Number of blocks that can actually collapse — drives toolbar
  // visibility. One collapsible block doesn't need expand-all/single
  // controls; the per-block chevron is enough.
  const collapsibleBlockCount = $derived(
    (card?.blocks ?? []).filter(isCollapsibleBlock).length,
  )

  function toggleBlockCollapse(blockID: string) {
    const next = new Set(collapsedBlocks)
    if (next.has(blockID)) {
      // Expanding this block. In single mode, collapse every other
      // collapsible block first so only this one remains open.
      if (blockAccordionMode === 'single' && card) {
        for (const b of card.blocks) {
          if (b.id !== blockID && isCollapsibleBlock(b)) next.add(b.id)
        }
      }
      next.delete(blockID)
    } else {
      next.add(blockID)
    }
    collapsedBlocks = next
  }

  function expandAllBlocks() {
    collapsedBlocks = new Set()
  }

  function collapseAllBlocks() {
    if (!card) return
    collapsedBlocks = new Set(card.blocks.filter(isCollapsibleBlock).map((b) => b.id))
  }

  // Switching INTO single mode: collapse all but one currently-open
  // block (preferring the first one in card order so the user keeps
  // the same visual anchor). Switching to multi is a no-op — multi is
  // a superset of single's allowed states.
  function applySingleModeCollapse() {
    if (!card) return
    const collapsible = card.blocks.filter(isCollapsibleBlock)
    const openBlocks = collapsible.filter((b) => !collapsedBlocks.has(b.id))
    if (openBlocks.length <= 1) return
    const keepOpen = openBlocks[0].id
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

  onMount(async () => {
    try {
      card = await repoRPC<Card>('GetCard', [id])
      lastSavedBlocks = card?.blocks ?? []
      // Resolve the card's primary pin so we can load that project's
      // tag definitions for accurate chip colours. Orphaned cards (no
      // pin) fall back to the global tag colour map. Best-effort —
      // failures here just mean grey tags.
      try {
        const loc = await repoRPC<{ brandSlug: string; streamSlug: string; projectSlug: string }>(
          'GetCardLocation',
          [id],
        )
        if (loc?.brandSlug && loc.streamSlug && loc.projectSlug) {
          projectKey = makeProjectKey(loc.brandSlug, loc.streamSlug, loc.projectSlug)
          loadProjectTags(loc.brandSlug, loc.streamSlug, loc.projectSlug)
        }
      } catch {
        /* unpinned card or RPC error — global tag colours only */
      }
      // Pin breadcrumbs power the "Pinned in" section + per-pin unpin.
      // Orphan cards just get an empty array.
      try {
        pins = (await repoRPC<CardPin[]>('GetCardPinBreadcrumbs', [id])) ?? []
      } catch {
        pins = []
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('card.err_load')
    } finally {
      loading = false
    }
  })

  // --- Pin / unpin ---

  async function refreshPins() {
    try {
      pins = (await repoRPC<CardPin[]>('GetCardPinBreadcrumbs', [id])) ?? []
    } catch {
      /* leave the previous list visible on transient errors */
    }
  }

  async function pinTo(sel: { project: { id: string }; category: { id: string } }) {
    if (!card) return
    pinPickerOpen = false
    saveError = null
    try {
      await repoRPC('PinCard', [card.id, sel.category.id])
      await refreshPins()
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('card.err_pin')
    }
  }

  async function unpinAt(pin: CardPin) {
    if (!card) return
    saveError = null
    // Optimistic remove — restore on error so the UI reflects reality.
    const previous = pins
    pins = pins.filter((p) => p !== pin)
    try {
      await repoRPC('UnpinCard', [card.id, pin.categoryId])
    } catch (err) {
      pins = previous
      saveError = err instanceof Error ? err.message : t('card.err_unpin')
    }
  }

  // --- Edit handlers ---
  //
  // Each handler optimistically updates local state, calls the RPC,
  // and reverts on failure with a visible error. Keeps the UI snappy
  // while still surfacing real save problems.

  async function saveTitle(next: string) {
    if (!card) return
    const previous = card.title
    card.title = next
    saveError = null
    try {
      await repoRPC('UpdateCardTitle', [card.id, next])
    } catch (err) {
      card.title = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  async function saveTags(next: string[]) {
    if (!card) return
    const previous = card.tags
    card.tags = next
    saveError = null
    try {
      await repoRPC('UpdateCardTags', [card.id, next])
    } catch (err) {
      card.tags = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  async function saveDueDate(next: string) {
    if (!card) return
    const previous = card.due_date
    card.due_date = next || null
    saveError = null
    try {
      await repoRPC('UpdateCardDueDate', [card.id, next])
    } catch (err) {
      card.due_date = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  // Native date input wants `YYYY-MM-DD`; the model stores ISO 8601 or
  // similar. Convert both directions, leaving the original on parse fail.
  function dueInputValue(raw: string | null | undefined): string {
    if (!raw) return ''
    const d = new Date(raw)
    if (Number.isNaN(d.getTime())) return ''
    const y = d.getFullYear().toString().padStart(4, '0')
    const m = (d.getMonth() + 1).toString().padStart(2, '0')
    const day = d.getDate().toString().padStart(2, '0')
    return `${y}-${m}-${day}`
  }

  async function saveDescription(next: string) {
    if (!card) return
    const previous = card.description
    card.description = next
    saveError = null
    try {
      await repoRPC('UpdateCardDescription', [card.id, next])
    } catch (err) {
      card.description = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  function flashSaved() {
    savedFlash = true
    if (savedFlashTimer) clearTimeout(savedFlashTimer)
    savedFlashTimer = setTimeout(() => {
      savedFlash = false
    }, 1200)
  }

  async function handleBlockMove(detail: DragMoveDetail) {
    if (!card) return
    const blockID = detail.cardID
    const toPosition = detail.toPosition

    const currentIndex = card.blocks.findIndex((b) => b.id === blockID)
    const clampedPosition = Math.max(0, Math.min(toPosition, card.blocks.length - 1))
    if (currentIndex === -1 || currentIndex === clampedPosition) return

    const blocksCopy = [...card.blocks]
    const [movedBlock] = blocksCopy.splice(currentIndex, 1)
    blocksCopy.splice(clampedPosition, 0, movedBlock)

    card.blocks = blocksCopy
    saveError = null

    if (blockSaveTimer) {
      clearTimeout(blockSaveTimer)
      blockSaveTimer = null
    }
    const snapshot = card.blocks
    savingBlocks = true
    try {
      await repoRPC('UpdateCardBlocks', [card.id, snapshot])
      lastSavedBlocks = snapshot
      flashSaved()
    } catch (err) {
      if (card) card.blocks = lastSavedBlocks
      saveError = err instanceof Error ? err.message : t('card.err_save')
    } finally {
      savingBlocks = false
    }
  }

  function updateBlock(blockID: string, next: Block) {
    if (!card) return
    // Optimistic local update — replace the matching block.
    card.blocks = card.blocks.map((b) => (b.id === blockID ? next : b))
    saveError = null
    scheduleSave()
  }

  async function deleteBlock(blockID: string) {
    if (!card) return
    card.blocks = card.blocks.filter((b) => b.id !== blockID)
    saveError = null
    // Flush pending typing-style debounce so the delete commits
    // immediately rather than waiting another 200ms.
    if (blockSaveTimer) {
      clearTimeout(blockSaveTimer)
      blockSaveTimer = null
    }
    const snapshot = card.blocks
    savingBlocks = true
    try {
      await repoRPC('UpdateCardBlocks', [card.id, snapshot])
      lastSavedBlocks = snapshot
      flashSaved()
    } catch (err) {
      if (card) card.blocks = lastSavedBlocks
      saveError = err instanceof Error ? err.message : t('card.err_save')
    } finally {
      savingBlocks = false
    }
  }

  function scheduleSave() {
    if (blockSaveTimer) clearTimeout(blockSaveTimer)
    blockSaveTimer = setTimeout(async () => {
      blockSaveTimer = null
      if (!card) return
      const snapshot = card.blocks
      savingBlocks = true
      try {
        await repoRPC('UpdateCardBlocks', [card.id, snapshot])
        lastSavedBlocks = snapshot
        flashSaved()
      } catch (err) {
        // Revert to the last server-confirmed state. The user sees
        // their last change disappear + an error rail; better than
        // pretending it stuck.
        if (card) card.blocks = lastSavedBlocks
        saveError = err instanceof Error ? err.message : t('card.err_save')
      } finally {
        savingBlocks = false
      }
    }, 200)
  }

  $effect(() => () => {
    if (blockSaveTimer) clearTimeout(blockSaveTimer)
    if (savedFlashTimer) clearTimeout(savedFlashTimer)
    if (copyFeedbackTimer) clearTimeout(copyFeedbackTimer)
  })

  // Copy-as-markdown feedback — transient "Copied!" label replaces the
  // button text for 2s on success. Errors surface via saveError so they
  // share the existing rail (no separate toast system on mobile yet).
  let copyFeedback = $state<'idle' | 'success'>('idle')
  let copyFeedbackTimer: ReturnType<typeof setTimeout> | null = null

  async function copyCardAsMarkdown() {
    if (!card) return
    let comments: CardComment[] = []
    try { comments = (await repoRPC<CardComment[]>('ListCardComments', [card.id])) ?? [] }
    catch { /* optional — degrade to no comments */ }
    const md = cardToMarkdown(card, { comments, untitledLabel: t('card.untitled'), labels: cardMarkdownLabels() })
    try {
      await navigator.clipboard.writeText(md)
      copyFeedback = 'success'
      if (copyFeedbackTimer) clearTimeout(copyFeedbackTimer)
      copyFeedbackTimer = setTimeout(() => { copyFeedback = 'idle' }, 2000)
    } catch {
      saveError = t('card.copy_markdown_error')
    }
  }

  async function exportCardAsJson() {
    if (!card) return
    try {
      const payload = await buildCardExportPayload(card)
      const json = JSON.stringify(payload, null, 2)
      downloadBlob(json, `${sanitizeFilenameStem(card.title)}.bruv-card.json`, 'application/json;charset=utf-8')
    } catch {
      saveError = t('card.export_json_error')
    }
  }

  // Live updates: when the backend emits card:updated for THIS card,
  // refetch so external edits (desktop, agent, share-sheet) reflect
  // here without a manual refresh. card:deleted while open pops back
  // to the previous view. Filter by id so unrelated cards don't
  // trigger a refetch storm.
  const unsubscribe = onEvent(async (ev) => {
    const eventCardID = typeof ev.payload.cardID === 'string' ? ev.payload.cardID : null
    if (eventCardID !== id) return

    if (ev.topic === 'card:updated') {
      // Skip refetch if a local save is in flight — it would clobber
      // the user's pending input. The save's own success/failure path
      // owns the canonical state for that window.
      if (blockSaveTimer || savingBlocks) return
      try {
        const fresh = await repoRPC<Card>('GetCard', [id])
        if (fresh) {
          card = fresh
          lastSavedBlocks = fresh.blocks ?? []
        }
      } catch {
        /* transient — keep showing what we have */
      }
      // Pin/unpin mutations emit card:updated without changing card
      // fields. Refresh the "Pinned in" rail so it reflects the
      // LLM-suggestion-accept (or any cross-device pin change) too.
      void refreshPins()
    } else if (ev.topic === 'card:deleted') {
      if (window.history.length > 1) history.back()
      else navigate('/')
    }
  })

  onDestroy(unsubscribe)

  // --- Type picker / refresh ---

  let typePickerOpen = $state(false)
  let confirmingRefresh = $state(false)
  let refreshing = $state(false)
  let searchOpen = $state(false)

  async function pickType(typeID: string) {
    typePickerOpen = false
    if (!card || card.type === typeID) return
    const previous = card.type
    card.type = typeID
    saveError = null
    try {
      await repoRPC('UpdateCardType', [card.id, typeID])
    } catch (err) {
      card.type = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  async function performRefresh() {
    confirmingRefresh = false
    if (!card || refreshing) return
    refreshing = true
    saveError = null
    try {
      const fresh = await repoRPC<Card>('RefreshTypeBlocks', [card.id])
      if (fresh) {
        card = fresh
        lastSavedBlocks = fresh.blocks ?? []
      }
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('card.err_save')
    } finally {
      refreshing = false
    }
  }

  // --- Delete ---

  let confirmingDelete = $state(false)

  async function deleteCard() {
    if (!card) return
    try {
      await repoRPC<void>('DeleteCard', [card.id])
      // Pop the route stack so the user lands wherever they came from
      // (Browse / Project / Inbox) rather than seeing the now-stale
      // detail page briefly. Falls back to / if there's no history
      // (e.g. they hit the card via deep link).
      if (window.history.length > 1) {
        history.back()
      } else {
        navigate('/')
      }
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('card.err_delete')
      confirmingDelete = false
    }
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => history.back()}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <span class="topbar-title" title={card?.title ?? ''}>
    {card?.title ?? t('common.loading')}
  </span>
  <div class="topbar-right">
    <span class="save-state" aria-live="polite">
      {#if savingBlocks}
        <span class="chip">{t('card.saving')}</span>
      {:else if savedFlash}
        <span class="chip saved">{t('card.saved')}</span>
      {/if}
    </span>
    <button type="button" class="topbar-search" onclick={() => (searchOpen = true)} aria-label={t('browse.search')} title={t('browse.search')}>
      <Search size={18} />
    </button>
  </div>
</header>

{#if searchOpen}
  <SearchSheet onClose={() => (searchOpen = false)} />
{/if}

<main style:view-transition-name={`card-${id}`}>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if card}
    <section class="meta">
      <EditableText
        value={card.title}
        placeholder={t('card.untitled')}
        ariaLabel={t('card.edit_title')}
        className="title-field"
        onSave={saveTitle}
      />

      <div class="meta-row">
        <button
          type="button"
          class="type-badge"
          style:background={card.type ? getCardTypeColor(card.type, repoMeta.cardTypes) : undefined}
          style:color={card.type ? getCardTypeTextColor(card.type) : undefined}
          class:placeholder={!card.type}
          onclick={() => (typePickerOpen = true)}
          aria-label={t('card.choose_type')}
        >
          {card.type ? getCardTypeLabel(card.type, repoMeta.cardTypes) : t('card.choose_type')}
        </button>
        {#if card.type}
          <button
            type="button"
            class="type-refresh"
            onclick={() => (confirmingRefresh = true)}
            aria-label={t('card.refresh_blocks')}
            title={t('card.refresh_blocks')}
            disabled={refreshing}
          >
            <RefreshCw size={13} />
          </button>
        {/if}
        <div class="due-row">
          <span class="due-label">{t('card.due')}</span>
          <input
            type="date"
            class="due-input"
            value={dueInputValue(card.due_date)}
            oninput={(e) => saveDueDate((e.currentTarget as HTMLInputElement).value)}
          />
          {#if card.due_date}
            <button type="button" class="due-clear" onclick={() => saveDueDate('')} aria-label={t('card.clear_due')} title={t('card.clear_due')}>
              <X size={12} />
            </button>
          {/if}
        </div>
      </div>

      <TagsEditor tags={card.tags ?? []} {projectKey} onChange={saveTags} />

      <section class="pins">
        <h3 class="pins-label">
          <MapPin size={12} />
          {t('card.pinned_in')}
        </h3>
        {#if pins.length === 0}
          <p class="pins-empty">{t('card.no_pins')}</p>
        {:else}
          <ul class="pin-list">
            {#each pins as pin (pin.projectId + ':' + pin.categoryId)}
              <li class="pin">
                <button
                  type="button"
                  class="pin-text"
                  onclick={() => navigate(
                    projectURL(pin.brandSlug, pin.streamSlug, pin.projectSlug)
                      + '#cat=' + encodeURIComponent(pin.categorySlug),
                  )}
                  title={t('card.go_to_project')}
                >{pin.breadcrumb}</button>
                <button
                  type="button"
                  class="pin-remove"
                  onclick={() => unpinAt(pin)}
                  aria-label={t('card.unpin')}
                  title={t('card.unpin')}
                >
                  <X size={12} />
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <button type="button" class="pin-add" onclick={() => (pinPickerOpen = true)}>
          <Plus size={12} />
          {t('card.pin_to')}
        </button>
      </section>

      {#if saveError}
        <div class="save-error" role="alert">{saveError}</div>
      {/if}
    </section>

    <section class="description">
      <h3 class="section-title">{t('card.description')}</h3>
      <EditableDescription
        value={card.description ?? ''}
        placeholder={t('card.add_description')}
        onSave={saveDescription}
      />
    </section>

    {#if card.blocks?.length}
      {#if collapsibleBlockCount > 1}
        <div class="block-acc-toolbar" role="toolbar" aria-label={t('block.accordion_toolbar')}>
          <button
            type="button"
            class="block-tool-btn"
            onclick={expandAllBlocks}
            aria-label={t('block.expand_all')}
            title={t('block.expand_all')}
          >
            <ChevronsUpDown size={14} />
          </button>
          <button
            type="button"
            class="block-tool-btn"
            onclick={collapseAllBlocks}
            aria-label={t('block.collapse_all')}
            title={t('block.collapse_all')}
          >
            <ChevronsDownUp size={14} />
          </button>
          <button
            type="button"
            class="block-tool-btn mode-btn"
            onclick={toggleAccordionMode}
            aria-label={blockAccordionMode === 'single' ? t('block.mode_single') : t('block.mode_multi')}
            title={blockAccordionMode === 'single' ? t('block.mode_single_hint') : t('block.mode_multi_hint')}
          >
            {#if blockAccordionMode === 'single'}
              <ListCollapse size={14} />
            {:else}
              <ListTree size={14} />
            {/if}
          </button>
        </div>
      {/if}
      <section
        class="blocks"
        data-drop-target="block-list"
        use:dragSortable={{
          onMove: handleBlockMove,
          rowSelector: '.block',
          dropTargetSelector: '[data-drop-target="block-list"]',
          rowIdAttribute: 'data-block-id',
          restoreDOM: false,
        }}
      >
        {#each card.blocks as block (block.id)}
          <BlockEditor
            {block}
            cardId={card.id}
            collapsed={collapsedBlocks.has(block.id)}
            onChange={(next) => updateBlock(block.id, next)}
            onDelete={() => deleteBlock(block.id)}
            onToggleCollapse={isCollapsibleBlock(block) ? () => toggleBlockCollapse(block.id) : undefined}
          />
        {/each}
      </section>
    {/if}

    <!-- Tab bar: Attachments / Comments -->
    <div class="meta-tabs">
      <button
        type="button"
        class="meta-tab-btn"
        class:active={activeMetaTab === 'attachments'}
        onclick={() => activeMetaTab = 'attachments'}
      >
        <Paperclip size={14} />
        <span>{t('attachment.title')}</span>
        {#if card.file_attachments?.length}
          <span class="count-badge">{card.file_attachments.length}</span>
        {/if}
      </button>
      <button
        type="button"
        class="meta-tab-btn"
        class:active={activeMetaTab === 'comments'}
        onclick={() => activeMetaTab = 'comments'}
      >
        <MessageSquare size={14} />
        <span>{t('comments.title')}</span>
        {#if commentCount > 0}
          <span class="count-badge">{commentCount}</span>
        {/if}
      </button>
    </div>

    <div class="meta-tab-content">
      {#if activeMetaTab === 'attachments'}
        <AttachmentsSection
          cardId={card.id}
          attachments={card.file_attachments ?? []}
          onCardUpdated={(updated) => card = updated}
        />
      {:else}
        <CommentsSection cardId={card.id} bind:count={commentCount} />
      {/if}
    </div>

    <footer class="actions">
      <button type="button" class="action-link" onclick={copyCardAsMarkdown}>
        <Copy size={14} />
        {copyFeedback === 'success' ? t('card.copy_markdown_done') : t('card.copy_markdown')}
      </button>
      <button type="button" class="action-link" onclick={exportCardAsJson}>
        <FileJson size={14} />
        {t('card.export_json')}
      </button>
      <button type="button" class="danger-link" onclick={() => (confirmingDelete = true)}>
        <Trash2 size={14} />
        {t('card.delete')}
      </button>
    </footer>
  {/if}
</main>

{#if confirmingDelete && card}
  <ConfirmDialog
    title={t('card.delete_confirm_title')}
    body={t('card.delete_confirm_body', { title: card.title || t('card.untitled') })}
    confirmLabel={t('card.delete')}
    destructive
    onConfirm={deleteCard}
    onCancel={() => (confirmingDelete = false)}
  />
{/if}

{#if pinPickerOpen}
  <PinPicker onSelect={pinTo} onClose={() => (pinPickerOpen = false)} />
{/if}

{#if typePickerOpen && card}
  <CardTypePicker current={card.type} onPick={pickType} onClose={() => (typePickerOpen = false)} />
{/if}

{#if confirmingRefresh}
  <ConfirmDialog
    title={t('card.refresh_blocks')}
    body={t('card.refresh_blocks_body')}
    confirmLabel={t('card.refresh_blocks_confirm')}
    onConfirm={performRefresh}
    onCancel={() => (confirmingRefresh = false)}
  />
{/if}

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar-title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60vw;
  }

  .topbar-right {
    justify-self: end;
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }
  .save-state {
    min-width: 0;
  }
  .topbar-search {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    min-width: 40px;
    min-height: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .topbar-search:hover,
  .topbar-search:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  .chip {
    font-size: 0.7rem;
    color: var(--text-faint);
    background: var(--bg-elev-1);
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    white-space: nowrap;
  }

  .chip.saved {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
    animation: chip-fade 1.2s ease-out forwards;
  }

  @keyframes chip-fade {
    0% { opacity: 0; transform: translateY(-2px); }
    20% { opacity: 1; transform: translateY(0); }
    80% { opacity: 1; }
    100% { opacity: 0; }
  }

  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    justify-self: start;
  }

  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    padding: 1rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .meta {
    margin-bottom: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  /* The EditableText sets width:100% on its input/button — these rules
     just make the title visually larger when not editing. */
  .meta :global(.title-field) {
    font-size: 1.4rem;
    font-weight: 600;
    padding: 0.4rem 0.5rem;
  }

  .meta-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }

  .type-badge {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    /* background + color set inline via style: bindings */
    padding: 0.25rem 0.6rem;
    border-radius: 4px;
    font-weight: 500;
    border: none;
    cursor: pointer;
    font-family: inherit;
    touch-action: manipulation;
  }
  .type-badge.placeholder {
    background: var(--bg-elev-1);
    color: var(--text-muted);
    border: 1px dashed var(--border);
  }
  .type-refresh {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.2rem 0.35rem;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 28px;
    min-height: 26px;
  }
  .type-refresh:hover,
  .type-refresh:focus-visible {
    color: var(--accent);
    border-color: var(--accent);
    outline: none;
  }
  .type-refresh:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .due-row {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .due-label {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .due-input {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    padding: 0.3rem 0.45rem;
    color-scheme: dark light;
  }
  .due-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .due-clear {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.25rem 0.35rem;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .due-clear:hover,
  .due-clear:focus-visible {
    color: #ef4444;
    border-color: rgba(239, 68, 68, 0.35);
    outline: none;
  }

  .pins {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .pins-label {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }

  .pins-empty {
    margin: 0;
    color: var(--text-faint);
    font-size: 0.85rem;
    font-style: italic;
  }

  .pin-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .pin {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.55rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .pin-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
    background: transparent;
    border: none;
    padding: 0;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .pin-text:hover,
  .pin-text:focus-visible {
    color: var(--accent);
    outline: none;
  }

  .pin-remove {
    background: transparent;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.15rem;
    border-radius: 4px;
    display: inline-flex;
    flex-shrink: 0;
  }
  .pin-remove:hover,
  .pin-remove:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.1);
    outline: none;
  }

  .pin-add {
    align-self: flex-start;
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-muted);
    padding: 0.35rem 0.7rem;
    border-radius: 6px;
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
  }
  .pin-add:hover,
  .pin-add:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .save-error {
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.8rem;
  }

  .description {
    border-top: 1px solid var(--border);
    padding-top: 1.25rem;
    margin-bottom: 1.5rem;
  }

  .section-title {
    margin: 0 0 0.5rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .blocks {
    border-top: 1px solid var(--border);
    padding-top: 1.25rem;
    margin-bottom: 2rem;
  }

  /* When the accordion toolbar sits above the blocks list, the
     toolbar owns the top divider so the two read as one section. */
  .block-acc-toolbar {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    border-top: 1px solid var(--border);
    padding: 0.65rem 0.25rem 0.55rem;
  }
  .block-acc-toolbar + .blocks {
    border-top: none;
    padding-top: 0;
  }
  .block-tool-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 6px;
    width: 34px;
    height: 34px;
    cursor: pointer;
    padding: 0;
    touch-action: manipulation;
  }
  .block-tool-btn:hover,
  .block-tool-btn:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    background: var(--bg-elev-1);
    outline: none;
  }
  .block-tool-btn.mode-btn {
    margin-left: auto;
  }


  .actions {
    display: flex;
    justify-content: flex-start;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1.5rem;
    flex-wrap: wrap;
  }

  .action-link {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }
  .action-link:hover,
  .action-link:focus-visible {
    color: var(--text-primary);
    background: var(--bg-elevated, rgba(255, 255, 255, 0.04));
    outline: none;
  }

  .danger-link {
    background: transparent;
    border: none;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .danger-link:hover,
  .danger-link:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.08);
    outline: none;
  }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
  }

  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
  }

  .meta-tabs {
    display: flex;
    gap: 0.25rem;
    border-top: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
    padding: 0.25rem 0.5rem;
    margin: 1.5rem 0 1rem;
  }
  .meta-tab-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.8rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.5rem 0.75rem;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    position: relative;
    cursor: pointer;
    touch-action: manipulation;
  }
  .meta-tab-btn::after {
    content: '';
    position: absolute;
    bottom: -0.25rem;
    left: 0;
    right: 0;
    height: 2px;
    background: transparent;
    transition: background 0.15s;
  }
  .meta-tab-btn.active {
    color: var(--accent);
  }
  .meta-tab-btn.active::after {
    background: var(--accent);
  }
  .count-badge {
    font-size: 0.7rem;
    font-weight: 500;
    background: var(--bg-elev-2, var(--border));
    color: var(--text-muted);
    padding: 0.05rem 0.35rem;
    border-radius: 999px;
  }
  .meta-tab-btn.active .count-badge {
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
  }
  .meta-tab-content {
    min-height: 120px;
  }

  /* DnD visual states for blocks */
  .blocks :global(.dnd-source) {
    opacity: 0.35;
    transition: opacity 120ms ease;
  }
  :global(.blocks.dnd-target-active),
  .blocks :global(.dnd-target-active) {
    outline: 2px dashed var(--accent);
    outline-offset: 4px;
    border-radius: 8px;
    transition: outline-color 120ms ease;
  }
</style>
