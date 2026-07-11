<script lang="ts">
  import { onMount, onDestroy, setContext } from 'svelte'
  import { EditScope, EDIT_SCOPE_KEY } from '@shared/editScope'
  import { repoRPC, NetworkError } from '../lib/auth'
  import { onReconnect } from '../lib/connectivity.svelte'
  import { navigate, projectURL } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import { Trash2, MapPin, Plus, X, RefreshCw, Search, Paperclip, MessageSquare, ChevronsUpDown, ChevronsDownUp, ListCollapse, ListTree, Copy, Download, FileJson } from 'lucide-svelte'
  import { cardToMarkdown } from '@shared/cardMarkdown'
  import { downloadBlob, sanitizeFilenameStem } from '@shared/download'
  import type { CardComment } from '@shared/types'
  import { buildCardExportPayload, cardMarkdownLabels } from '../lib/cardExport'
  import { showToast } from '../lib/toast.svelte'
  import EditableText from '../components/EditableText.svelte'
  import EditableDescription from '../components/EditableDescription.svelte'
  import TagsEditor from '../components/TagsEditor.svelte'
  import BlockEditor from '../components/blocks/BlockEditor.svelte'
  import BlockTypePicker from '../components/BlockTypePicker.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import ErrorState from '../components/ErrorState.svelte'
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

  // Keyboard entry contract (see UI-CONVENTIONS "Keyboard entry"):
  // every in-flight edit on this page registers here via context.
  // Escape closes the page only when nothing is being edited;
  // Ctrl+Enter commits every active edit and closes.
  const editScope = new EditScope()
  editScope.requestClose = () => closePage()
  setContext(EDIT_SCOPE_KEY, editScope)

  /** Pop back to wherever the user came from; fall back to home for
   *  deep links with no history (same pattern as deleteCard). */
  function closePage() {
    if (window.history.length > 1) history.back()
    else navigate('/')
  }

  // --- Back = Escape layering (UI-CONVENTIONS §8 mobile variant) ---
  //
  // While any edit is active, a Back activation cancels it and the card
  // STAYS open; a second Back navigates. Two entry points:
  //
  //  • The ← button. Intercepted on pointerdown — it fires BEFORE the
  //    tap moves focus (a blur would commit, not cancel). The follow-up
  //    click is swallowed once via backTapConsumed.
  //
  //  • System/gesture/hardware back (popstate). By the time popstate
  //    fires the entry is already popped, so we cancel the edits and
  //    push the card's own URL straight back — stateless: no guard
  //    entries to leak, and the stack ends up exactly as before the
  //    back press. The router re-reads location inside its (async)
  //    view-transition callback, so it never sees the intermediate
  //    URL; the synthetic popstate covers browsers without the View
  //    Transitions API, where the router's route state flipped
  //    synchronously and needs correcting.
  //
  // Overlays (sheets/pickers) push their own history entries and own
  // the Back that closes them — while one is open we don't interfere
  // (mirrors handleWindowKeydown; no CardPage edit can be active then
  // anyway, since opening an overlay blurs it).

  // Captured at init — the card page's own URL (/m/c/<id>), used to
  // restore the just-popped entry.
  const pageURL = window.location.pathname
  let suppressPopstate = false
  let backTapConsumed = false

  function cancelActiveEdits() {
    editScope.cancelAll()
    // Always-mounted fields (draftEdit) stay focused through a scope
    // cancel; blur so the keyboard dismisses and the next focus starts
    // a fresh session. After cancelAll, the fields' settled latches
    // make this blur's commit a no-op.
    ;(document.activeElement as HTMLElement | null)?.blur()
  }

  function onBackPointerDown() {
    if (!editScope.hasActive()) return
    cancelActiveEdits()
    backTapConsumed = true
  }

  function onBackPointerUp() {
    // The click (when the tap completes) fires before timers run; this
    // clears a consumed flag left behind by a tap that never became a
    // click (drag-away), so the next ← tap isn't swallowed.
    setTimeout(() => (backTapConsumed = false), 0)
  }

  function onBackClick() {
    if (backTapConsumed) {
      backTapConsumed = false
      return
    }
    history.back()
  }

  function handlePopstate() {
    if (suppressPopstate) {
      suppressPopstate = false
      return
    }
    if (
      searchOpen ||
      pinPickerOpen ||
      typePickerOpen ||
      blockPickerOpen ||
      confirmingDelete ||
      confirmingRefresh
    ) return
    if (!editScope.hasActive()) return
    cancelActiveEdits()
    // Restore the just-popped card entry so the user stays put; the
    // next Back (nothing active any more) navigates for real. Scroll
    // stays put too: the router sets history.scrollRestoration =
    // 'manual', so the traversal we're undoing never re-applies the
    // underlying entry's saved scroll offset.
    suppressPopstate = true
    window.history.pushState({}, '', pageURL)
    window.dispatchEvent(new PopStateEvent('popstate'))
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    // Child overlays own their keyboard handling while open (ConfirmDialog
    // consumes Escape itself; SearchSheet runs its own scope; the pickers
    // are tap-to-choose). Escape/Ctrl+Enter must not also close the page
    // underneath them.
    if (
      searchOpen ||
      pinPickerOpen ||
      typePickerOpen ||
      blockPickerOpen ||
      confirmingDelete ||
      confirmingRefresh
    ) return
    editScope.handleWindowKeydown(e)
  }

  let card = $state<Card | null>(null)
  let projectKey = $state<string | undefined>(undefined)
  let pins = $state<CardPin[]>([])
  let pinPickerOpen = $state(false)
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let saveError = $state<string | null>(null)
  // Saves that failed because we were offline, keyed by field so repeated
  // edits to the same thing collapse to a single retry. Flushed on
  // reconnect (see the onMount handler) — the optimistic edit stays on
  // screen meanwhile, so nothing is lost.
  const pendingSaves = new Map<string, () => Promise<void>>()

  /** Shared failure path for every save handler. A network failure keeps
   *  the optimistic edit and queues a retry (the global overlay already
   *  tells the user we're offline — no inline rail). A genuine server
   *  error reverts and shows the inline error. */
  function onSaveFailed(
    err: unknown,
    key: string,
    retry: () => Promise<void>,
    revert: () => void,
  ): void {
    if (err instanceof NetworkError) {
      pendingSaves.set(key, retry)
      return
    }
    revert()
    saveError = err instanceof Error ? err.message : t('card.err_save')
  }

  async function flushPendingSaves(): Promise<void> {
    // Reconnecting clears any inline error left by a failed save —
    // including from one-shot actions (refresh/delete) that don't retry.
    saveError = null
    if (pendingSaves.size === 0) return
    const retries = [...pendingSaves.values()]
    pendingSaves.clear()
    // Each retry routes its own failures back through onSaveFailed, so a
    // still-bad connection simply re-queues rather than throwing here.
    for (const retry of retries) await retry()
  }
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

  async function loadCard() {
    // Reset so a retry shows the spinner and clears a prior error
    // (e.g. after reconnecting to Tailscale).
    loading = true
    errorMsg = null
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
  }

  onMount(() => {
    void loadCard()
    window.addEventListener('popstate', handlePopstate)
    // On reconnect: if the card never loaded, fetch it; otherwise keep the
    // on-screen edit session and flush any saves that failed while offline.
    const offReconnect = onReconnect(() => {
      if (!card) void loadCard()
      else void flushPendingSaves()
    })
    return () => {
      window.removeEventListener('popstate', handlePopstate)
      offReconnect()
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
      onSaveFailed(err, 'pin', () => pinTo(sel), () => {})
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
      onSaveFailed(err, `unpin:${pin.categoryId}`, () => unpinAt(pin), () => { pins = previous })
    }
  }

  // --- Edit handlers ---
  //
  // Every single-field edit goes through persistField: optimistic local
  // update, RPC, and on failure the shared onSaveFailed path (keep + retry
  // on a network drop, revert + inline error on a server rejection). Block
  // edits (saveBlocksNow) and pin/unpin have their own entry points —
  // different shapes that don't fit a single-field model.
  async function persistField<K extends keyof Card>(
    key: K,
    value: Card[K],
    method: string,
    arg: unknown = value,
  ): Promise<void> {
    if (!card) return
    const previous = card[key]
    card[key] = value
    saveError = null
    try {
      await repoRPC(method, [card.id, arg])
    } catch (err) {
      onSaveFailed(
        err,
        String(key),
        () => persistField(key, value, method, arg),
        () => {
          if (card) card[key] = previous
        },
      )
    }
  }

  const saveTitle = (next: string) => persistField('title', next, 'UpdateCardTitle')
  const saveTags = (next: string[]) => persistField('tags', next, 'UpdateCardTags')
  // Stored as null when cleared; the RPC takes the raw input string.
  const saveDueDate = (next: string) => persistField('due_date', next || null, 'UpdateCardDueDate', next)
  const saveDescription = (next: string) => persistField('description', next, 'UpdateCardDescription')

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
    await saveBlocksNow()
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
    await saveBlocksNow()
  }

  function scheduleSave() {
    if (blockSaveTimer) clearTimeout(blockSaveTimer)
    blockSaveTimer = setTimeout(() => void saveBlocksNow(), 200)
  }

  // Single persistence path for every block mutation (edit / move / delete
  // / add). Flushes any pending debounce, persists the current snapshot,
  // and on failure routes through onSaveFailed — so a network drop keeps
  // the optimistic blocks and retries on reconnect rather than reverting
  // and losing the edit.
  async function saveBlocksNow() {
    if (!card) return
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
      onSaveFailed(err, 'blocks', saveBlocksNow, () => {
        if (card) card.blocks = lastSavedBlocks
      })
    } finally {
      savingBlocks = false
    }
  }

  // Empty checklist/list rows are editing placeholders, not content. Drop
  // any that remain when leaving the card (e.g. a row added then Back-ed
  // out of before it blurred). Done on unmount specifically — stripping
  // during live editing would let the post-save card:updated echo refetch
  // and remove the row the user is mid-typing into.
  function withoutEmptyItems(blocks: Block[]): { blocks: Block[]; changed: boolean } {
    let changed = false
    const out = blocks.map((b) => {
      if ((b.type === 'checklist' || b.type === 'list') && Array.isArray(b.value)) {
        const items = b.value as Array<{ text?: unknown }>
        const filtered = items.filter(
          (it) => typeof it?.text === 'string' && (it.text as string).trim() !== '',
        )
        if (filtered.length !== items.length) {
          changed = true
          return { ...b, value: filtered as unknown as Block['value'] }
        }
      }
      return b
    })
    return { blocks: out, changed }
  }

  $effect(() => () => {
    if (blockSaveTimer) clearTimeout(blockSaveTimer)
    if (savedFlashTimer) clearTimeout(savedFlashTimer)
    if (card) {
      const { blocks: cleaned, changed } = withoutEmptyItems(card.blocks)
      if (changed) void repoRPC('UpdateCardBlocks', [card.id, cleaned])
    }
  })

  async function buildMarkdown(): Promise<string> {
    if (!card) return ''
    let comments: CardComment[] = []
    try { comments = (await repoRPC<CardComment[]>('ListCardComments', [card.id])) ?? [] }
    catch { /* optional — degrade to no comments */ }
    return cardToMarkdown(card, { comments, untitledLabel: t('card.untitled'), labels: cardMarkdownLabels() })
  }

  async function copyCardAsMarkdown() {
    if (!card) return
    const md = await buildMarkdown()
    try {
      await navigator.clipboard.writeText(md)
      showToast(t('card.copy_markdown_done'), 'success')
    } catch {
      showToast(t('card.copy_markdown_error'), 'error')
    }
  }

  // Save-as-Markdown-file exists on mobile too (desktop parity): the
  // shared downloadBlob anchor-download works in mobile browsers exactly
  // like the JSON export below, so there's no reason to asymmetrise.
  async function exportCardAsMarkdown() {
    if (!card) return
    const md = await buildMarkdown()
    downloadBlob(md, `${sanitizeFilenameStem(card.title)}.md`, 'text/markdown;charset=utf-8')
    showToast(t('card.export_markdown_done'), 'success')
  }

  // JSON export fetches + base64-encodes every attachment — seconds on
  // big cards, so the footer button shows a busy state meanwhile.
  let exportingJson = $state(false)

  async function exportCardAsJson() {
    if (!card) return
    exportingJson = true
    try {
      const payload = await buildCardExportPayload(card)
      const json = JSON.stringify(payload, null, 2)
      downloadBlob(json, `${sanitizeFilenameStem(card.title)}.bruv-card.json`, 'application/json;charset=utf-8')
      showToast(t('card.export_json_done'), 'success')
    } catch {
      showToast(t('card.export_json_error'), 'error')
    } finally {
      exportingJson = false
    }
  }

  // --- Add block ---
  //
  // Appends a fresh block of the chosen type and persists immediately.
  // Default value/meta per type mirror the desktop's add-block picker so
  // a block created on either surface looks identical. User-added blocks
  // get an empty key (the model convention — keys are for schema fields).
  let blockPickerOpen = $state(false)

  async function addBlock(blockType: string) {
    blockPickerOpen = false
    if (!card) return
    const id = `blk-${crypto.randomUUID().slice(0, 8)}`
    let value: Block['value'] = ''
    let meta: Block['meta']
    switch (blockType) {
      case 'checklist':
      case 'list':
      case 'media':
      case 'survey':
        value = []
        break
      case 'checkbox_group':
        value = []
        meta = { options: ['Option 1', 'Option 2', 'Option 3'] }
        break
      case 'select':
      case 'radio':
        value = ''
        meta = { options: ['Option 1', 'Option 2', 'Option 3'] }
        break
      case 'number':
      case 'progress':
        value = 0
        break
      case 'rating':
        value = 0
        meta = { max: 5 }
        break
      case 'checkbox':
        value = false
        break
      case 'divider':
      case 'image':
        value = null
        break
      case 'alarm':
        value = null
        meta = { alarm_channels: 'in-app,system' }
        break
      // text, url, date → '' (default)
    }

    const newBlock: Block = {
      id,
      type: blockType as Block['type'],
      label: t('block.type.' + blockType),
      key: '',
      value,
      meta,
    }
    card.blocks = [...card.blocks, newBlock]
    saveError = null

    // In single-expand mode, keep only the new block open.
    if (blockAccordionMode === 'single') {
      const next = new Set<string>()
      for (const b of card.blocks) {
        if (b.id !== id && isCollapsibleBlock(b)) next.add(b.id)
      }
      collapsedBlocks = next
    }

    // Persist now (saveBlocksNow flushes any pending typing debounce).
    await saveBlocksNow()

    // Scroll the new block into view so the user sees it land.
    queueMicrotask(() => {
      document
        .querySelector(`[data-block-id="${id}"]`)
        ?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
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
      // Cancel any in-flight edit first — the Back = Escape popstate
      // layer would otherwise treat this programmatic back() as a
      // cancel-and-stay and strand the user on a deleted card.
      cancelActiveEdits()
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

  function pickType(typeID: string) {
    typePickerOpen = false
    if (!card || card.type === typeID) return
    void persistField('type', typeID, 'UpdateCardType')
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
      // Offline is communicated by the global overlay; don't auto-retry an
      // AI refresh (it costs tokens) — the user can re-trigger it.
      if (!(err instanceof NetworkError)) {
        saveError = err instanceof Error ? err.message : t('card.err_save')
      }
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
      // Offline is shown by the global overlay; don't auto-retry a delete.
      if (!(err instanceof NetworkError)) {
        saveError = err instanceof Error ? err.message : t('card.err_delete')
      }
      confirmingDelete = false
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<header class="topbar">
  <!-- Back = Escape: pointerdown runs before the tap blurs an active
       edit (blur would commit), so an edit-cancelling ← works. -->
  <button
    type="button"
    class="back"
    onpointerdown={onBackPointerDown}
    onpointerup={onBackPointerUp}
    onpointercancel={() => (backTapConsumed = false)}
    onclick={onBackClick}
  >
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <span class="topbar-title" title={card?.title ?? ''}>
    {@html renderInline(card?.title ?? t('common.loading'))}
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
    <ErrorState message={errorMsg} />
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
          handleSelector: '.block-toolbar',
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

    <!-- Add block — always available, including when the card has none. -->
    <div class="add-block-row">
      <button type="button" class="add-block-btn" onclick={() => (blockPickerOpen = true)}>
        <Plus size={16} />
        <span>{t('block.add')}</span>
      </button>
    </div>

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
        {t('card.copy_markdown')}
      </button>
      <button type="button" class="action-link" onclick={exportCardAsMarkdown}>
        <Download size={14} />
        {t('card.export_markdown')}
      </button>
      <button type="button" class="action-link" onclick={exportCardAsJson} disabled={exportingJson}>
        <FileJson size={14} />
        {exportingJson ? t('common.working') : t('card.export_json')}
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

{#if blockPickerOpen}
  <BlockTypePicker onPick={addBlock} onClose={() => (blockPickerOpen = false)} />
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
    /* No double-tap-zoom click delay — the pointerdown edit-cancel and
       its follow-up click must land in one predictable sequence. */
    touch-action: manipulation;
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

  .add-block-row {
    display: flex;
    margin-bottom: 2rem;
  }
  .add-block-btn {
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    padding: 0.7rem 0.9rem;
    border-radius: 8px;
    cursor: pointer;
    touch-action: manipulation;
  }
  .add-block-btn:hover,
  .add-block-btn:focus-visible {
    color: var(--text);
    border-color: var(--accent);
    background: var(--bg-elev-1);
    outline: none;
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

  .action-link:disabled {
    opacity: 0.55;
    cursor: default;
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
