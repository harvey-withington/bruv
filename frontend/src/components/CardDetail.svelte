<script lang="ts">
  import { GetCard, UpdateCardTitle, UpdateCardType, RefreshTypeBlocks, UpdateCardDescription, UpdateCardDueDate,
    DeleteCard, PinCard, UnpinCard, GetCardPinBreadcrumbs, GetProjectLabels, GetCategoryAcceptedTypes, GetAgentConfig, GetProjectMembers } from '@shared/api'
  import { onEvent } from '../lib/events'
  import { projectTags, nav, cardTypes } from '../lib/store.svelte'
  import { X, Trash2, BotMessageSquare, ClipboardList, History, Timer } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import MentionPicker from './MentionPicker.svelte'
  import PinPicker from './PinPicker.svelte'
  import PinPanel from './PinPanel.svelte'
  import CardHeader from './CardHeader.svelte'
  import DescriptionSection from './DescriptionSection.svelte'
  import ChatSection from './ChatSection.svelte'
  import AgentTab from './AgentTab.svelte'
  import AgentRunsTab from './AgentRunsTab.svelte'
  import CardShareMenu from './CardShareMenu.svelte'
  import CardMetaPanel from './CardMetaPanel.svelte'
  import CardTagsField from './CardTagsField.svelte'
  import CardBlocks from './CardBlocks.svelte'
  import SaveIndicator from './SaveIndicator.svelte'
  import { draggable } from '../lib/draggable'
  import { fade } from 'svelte/transition'
  import { focusTrap } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, CardPin, ProjectMember } from '@shared/types'


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
  let notFound = $state(false)
  let pinBreadcrumbs = $state<CardPin[]>([])
  let projectMembers = $state<ProjectMember[]>([])
  let assignedMembers = $derived(
    (card?.members || []).map(mId => {
      const pm = projectMembers.find(m => m.id === mId)
      return pm ? pm.fullName || pm.username || mId : mId
    })
  )
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
  // Type picker
  let showTypePicker = $state(false)
  let typePickerEl = $state<HTMLDivElement | null>(null)
  let typeBadgeBtnEl = $state<HTMLButtonElement | null>(null)

  // Description (standard field, always at top)
  let descriptionDraft = $state('')
  let editingDescription = $state(false)
  let descTextareaEl = $state<HTMLTextAreaElement | null>(null)

  function handleWindowClick(e: MouseEvent) {
    const target = e.target as Node
    if (showTypePicker) {
      if (!typePickerEl?.contains(target) && !(target as HTMLElement).closest?.('.type-picker-dropdown')) showTypePicker = false
    }
  }

  // Custom-blocks region — block editing, DnD, collapse state and the
  // text/checklist mention picker all live in CardBlocks. The ref gives
  // access to its exported add/restore/commit helpers.
  let cardBlocksRef = $state<CardBlocks | null>(null)

  // @ mention picker state (description only — block/checklist mentions
  // are handled by CardBlocks' own picker)
  let mentionVisible = $state(false)
  let mentionAnchor = $state<{ top: number; left: number } | null>(null)
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
      if (editingTitle || editingDescription || cardBlocksRef?.hasActiveEdit()) return
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
      descriptionDraft = card.description || ''
      // CardBlocks re-seeds its drafts on every reload. On non-silent
      // loads the ref is null (the loading placeholder tears the
      // component down) and CardBlocks seeds itself on remount instead.
      cardBlocksRef?.resetDrafts()
      // Only seed collapsed-state from meta on the initial open. Silent
      // reloads fire after EVERY save (including toggling a checklist
      // item) and would otherwise wipe the user's session-level collapse
      // choices — meta.collapsed isn't actually written anywhere today,
      // so re-reading it on a silent refresh just resets the set.
      if (!silent) cardBlocksRef?.restoreCollapsedFromMeta()
      if (autoEditTitle) editingTitle = true
      // Check if card has an agent configured
      try { const af = await GetAgentConfig(cardId); hasAgent = af?.config?.enabled ?? false } catch { hasAgent = false }
      // Refresh project tags so new tags (e.g. added by AI) get their colors
      if (nav.brandSlug && nav.streamSlug && nav.projectSlug) {
        try { projectTags.list = await GetProjectLabels(nav.brandSlug, nav.streamSlug, nav.projectSlug) || [] } catch {}
        try { projectMembers = await GetProjectMembers(nav.brandSlug, nav.streamSlug, nav.projectSlug) || [] } catch {}
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
    try {
      card = await tracked(UpdateCardDescription(cardId, descriptionDraft)) as Card
      editingDescription = false
    } catch (e) { showToast(t('error.save_failed'), 'error'); return }
    onUpdated?.()
  }

  function handleDescKeydown(e: KeyboardEvent) {
    if (mentionVisible) return
    if (e.key === 'Escape') {
      e.stopPropagation()
      editingDescription = false
      descriptionDraft = card?.description || ''
    }
  }

  function handleDescBlur() {
    if (!editingDescription || mentionVisible) return
    saveDescription()
  }

  function handleDescInput(e: Event) {
    const el = e.target as HTMLTextAreaElement
    const pos = el.selectionStart ?? 0
    const text = el.value
    if (pos > 0 && text[pos - 1] === '@') {
      if (pos === 1 || /\s/.test(text[pos - 2])) {
        mentionTriggerPos = pos - 1
        const rect = el.getBoundingClientRect()
        mentionAnchor = { top: rect.bottom + 4, left: rect.left }
        mentionVisible = true
      }
    }
  }

  function handleMentionSelect(markdown: string) {
    const before = descriptionDraft.slice(0, mentionTriggerPos)
    const after = descriptionDraft.slice(descTextareaEl?.selectionStart ?? mentionTriggerPos + 1)
    descriptionDraft = before + markdown + after
    mentionVisible = false
    const newPos = before.length + markdown.length
    setTimeout(() => { descTextareaEl?.focus(); descTextareaEl?.setSelectionRange(newPos, newPos) }, 0)
  }

  function handleMentionClose() {
    mentionVisible = false
    // Refocus the description so the user can continue editing
    setTimeout(() => descTextareaEl?.focus(), 0)
  }

  // Applies a card snapshot returned by a child component's mutation
  // (attachments panel, tags field, blocks region) and notifies the
  // parent board.
  function applyCardUpdate(updatedCard: Card) {
    card = updatedCard
    onUpdated?.()
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
        await UnpinCard(cardId, currentPin.categoryId)
      } else {
        await PinCard(cardId, effectiveCategoryId!)
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
        await UnpinCard(cardId, pinPickerSourcePin.categoryId)
        await PinCard(cardId, target.categoryId)
      } else {
        await PinCard(cardId, target.categoryId)
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
      await UnpinCard(cardId, pin.categoryId)
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
      await cardBlocksRef?.commitActiveEdit()
      onClose()
      return
    }
    if (e.key === 'Escape') {
      if (editingTitle || editingDescription || cardBlocksRef?.hasActiveEdit()) return
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
        <button class="card-tab" class:active={activeTab === 'details'} onclick={() => activeTab = 'details'}>
          <ClipboardList size={13} />
          <span>{t('card.tab_details')}</span>
        </button>
        <button class="card-tab" class:active={activeTab === 'agent'} onclick={() => activeTab = 'agent'}>
          <Timer size={13} />
          <span>{t('card.tab_agent')}</span>
          {#if hasAgent}
            <span class="agent-dot"></span>
          {/if}
        </button>
        <button class="card-tab" class:active={activeTab === 'runs'} onclick={() => activeTab = 'runs'}>
          <History size={13} />
          <span>{t('card.tab_runs')}</span>
        </button>
      </div>

      <div class="modal-body" hidden={activeTab !== 'agent'}>
        <AgentTab {cardId} />
      </div>
      <div class="modal-body" hidden={activeTab !== 'runs'}>
        <AgentRunsTab {cardId} />
      </div>
      <div class="modal-body" hidden={activeTab !== 'details'}>
        <!-- Standard fields: compact flex layout for responsiveness -->
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
          {#if assignedMembers.length > 0}
            <div class="field-cell">
              <span class="field-label">{t('card.assignees')}</span>
              <div class="assignees-list">
                {#each assignedMembers as name}
                  <span class="assignee-chip" title={name}>
                    <span class="assignee-avatar">{name.slice(0, 2).toUpperCase()}</span>
                    <span class="assignee-name">{name}</span>
                  </span>
                {/each}
              </div>
            </div>
          {/if}
          <div class="field-cell tags-cell">
            <span class="field-label">{t('card.tags')}</span>
            <CardTagsField {card} {cardId} {pinBreadcrumbs} track={tracked} onCardUpdated={applyCardUpdate} />
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

        <!-- Custom blocks region: toolbar + DnD list + block mention picker -->
        <CardBlocks
          bind:this={cardBlocksRef}
          {card}
          {cardId}
          {currentCategoryId}
          track={tracked}
          onCardUpdated={applyCardUpdate}
          {onUpdated}
          onClose={() => onClose()}
        />
      </div>

      <!-- Card-level attachments & comments tabbed panel (pinned between scrollable body and footer) -->
      {#if activeTab === 'details'}
        <CardMetaPanel {cardId} {card} onAttachmentsUpdated={applyCardUpdate} onAddBlock={(type, label) => cardBlocksRef?.addBlock(type, label)} />
      {/if}

      <div class="modal-footer">
        <span class="modal-footer-left">
          <button class="btn-delete" onclick={handleDelete} title={t('tooltip.delete_card')}><Trash2 size={14} /> {t('card.delete')}</button>
          <CardShareMenu {card} />
        </span>
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
    gap: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
    padding: 0 1.25rem;
    flex-shrink: 0;
  }
  .card-tab {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.5rem 0.6rem;
    display: flex;
    align-items: center;
    gap: 0.3rem;
    position: relative;
    transition: color var(--duration-fast);
  }
  .card-tab::after {
    content: '';
    position: absolute;
    bottom: -1px;
    left: 0;
    right: 0;
    height: 2px;
    background: transparent;
    transition: background var(--duration-fast);
  }
  .card-tab:hover {
    color: var(--text-primary);
  }
  .card-tab.active {
    color: var(--accent);
  }
  .card-tab.active::after {
    background: var(--accent);
  }

  .agent-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex-shrink: 0;
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
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem 1.5rem;
    align-items: start;
    margin-bottom: 0.5rem;
  }

  .field-cell {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .tags-cell {
    flex: 1;
    min-width: 200px;
  }

  .assignees-list {
    display: flex;
    gap: 0.35rem;
    flex-wrap: wrap;
    align-items: center;
  }

  .assignee-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 0.15rem 0.6rem 0.15rem 0.2rem;
    font-size: 0.75rem;
    color: var(--text-body);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  }

  .assignee-avatar {
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--accent);
    color: #fff;
    font-size: 0.65rem;
    font-weight: 600;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .assignee-name {
    font-weight: 500;
    white-space: nowrap;
  }

  .field-label {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .modal-footer-left {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    position: relative;
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
</style>
