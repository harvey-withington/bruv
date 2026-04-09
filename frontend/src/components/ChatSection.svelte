<script lang="ts">
  import { tick } from 'svelte'
  import { Send, MapPin, Check, X, Wrench, ChevronUp, ChevronDown, MessageCircle, PencilLine, ListChecks, Trash2 } from 'lucide-svelte'
  import { LoadChatHistory, SendChatMessage, IsLLMConfigured, AcceptPinSuggestion, RejectPinSuggestion, GetLLMConfig, SetLLMConfig, ApplyPendingEdits, ClearCardChatHistory } from '../lib/api'
  import { showConfirm } from '../lib/confirm.svelte'
  import { renderMarkdown } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'

  interface PendingEdit {
    id: string
    tool: string
    input: Record<string, unknown>
    label: string
    detail: string
    status: 'pending' | 'accepted' | 'rejected'
  }

  interface ChatMessage {
    id: string
    role: string
    content: string
    timestamp: string
    tool_actions?: { tool: string; input: unknown; result: string }[]
    pin_suggestion?: { category_id: string; breadcrumb: string; reason?: string; status: string }
    pending_edits?: PendingEdit[]
  }

  let {
    cardId,
    visible = $bindable(false),
    mainWidth = $bindable(720),
    splitterDragging = $bindable(false),
    onCardChanged,
    projectMode = false,
    loadFn,
    sendFn,
    reloadKey,
    clearFn,
  }: {
    cardId: string
    visible: boolean
    mainWidth?: number
    splitterDragging?: boolean
    onCardChanged?: () => void
    projectMode?: boolean
    loadFn?: () => Promise<{ messages: ChatMessage[] }>
    sendFn?: (text: string) => Promise<{ messages: ChatMessage[] }>
    reloadKey?: string
    clearFn?: () => Promise<void>
  } = $props()

  // Resizable chat width (splitter between main and chat)
  const CHAT_WIDTH_KEY = 'bruv:chatPanelWidth'
  const MIN_PANEL = 300
  let chatWidth = $state(Number(localStorage.getItem(CHAT_WIDTH_KEY)) || 380)
  let resizing = $state(false)

  /** Splitter: redistributes space between main and chat, outer edges stay fixed */
  function onSplitterDown(e: MouseEvent) {
    e.preventDefault()
    resizing = true
    splitterDragging = true
    const startX = e.clientX
    const startChat = chatWidth
    const startMain = mainWidth

    function onMove(ev: MouseEvent) {
      const delta = ev.clientX - startX
      const newMain = startMain + delta
      const newChat = startChat - delta
      if (newMain >= MIN_PANEL && newChat >= MIN_PANEL) {
        mainWidth = newMain
        chatWidth = newChat
      }
    }
    function onUp() {
      resizing = false
      splitterDragging = false
      localStorage.setItem(CHAT_WIDTH_KEY, String(chatWidth))
      localStorage.setItem('bruv:mainPanelWidth', String(mainWidth))
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  // Fixed-position tooltip for edit details (avoids scroll-container clipping)
  let tooltipText = $state('')
  let tooltipX = $state(0)
  let tooltipY = $state(0)
  let tooltipVisible = $state(false)

  function showEditTooltip(e: MouseEvent, detail: string) {
    if (!detail) return
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    tooltipText = detail
    tooltipX = rect.left
    tooltipY = rect.bottom + 4
    tooltipVisible = true
  }

  function hideEditTooltip() {
    tooltipVisible = false
  }

  // Local checkbox state per message — not persisted, purely UI
  // Initialised to all-checked when a message with pending edits first renders
  let checkedEdits = $state<Record<string, Record<string, boolean>>>({})

  $effect(() => {
    for (const msg of messages) {
      if (!msg.pending_edits?.length) continue
      if (checkedEdits[msg.id]) continue
      const initial: Record<string, boolean> = {}
      for (const e of msg.pending_edits) {
        if (e.status === 'pending') initial[e.id] = true
      }
      checkedEdits[msg.id] = initial
    }
  })

  function pendingEditsOf(msg: ChatMessage) {
    return msg.pending_edits?.filter(e => e.status === 'pending') ?? []
  }

  function allChecked(msg: ChatMessage): boolean {
    const pending = pendingEditsOf(msg)
    return pending.length > 0 && pending.every(e => checkedEdits[msg.id]?.[e.id])
  }

  function someChecked(msg: ChatMessage): boolean {
    return pendingEditsOf(msg).some(e => checkedEdits[msg.id]?.[e.id])
  }

  function toggleAll(msg: ChatMessage) {
    const shouldCheck = !allChecked(msg)
    const updated = { ...checkedEdits[msg.id] }
    for (const e of pendingEditsOf(msg)) updated[e.id] = shouldCheck
    checkedEdits[msg.id] = updated
  }

  function toggleEdit(msgId: string, editId: string) {
    checkedEdits[msgId] = { ...checkedEdits[msgId], [editId]: !checkedEdits[msgId]?.[editId] }
  }

  async function applyEdits(msgId: string) {
    const msg = messages.find(m => m.id === msgId)
    const checked = checkedEdits[msgId] ?? {}
    const acceptIDs = pendingEditsOf(msg!).filter(e => checked[e.id]).map(e => e.id)
    const hasPinEdit = msg?.pending_edits?.some(e => e.tool === 'suggest_pin' && checked[e.id] && e.status === 'pending')
    try {
      const result = await ApplyPendingEdits(cardId, msgId, acceptIDs)
      messages = result?.messages || []
      onCardChanged?.()
      if (hasPinEdit) {
        document.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
        document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      }
    } catch (e) {
      showToast(t('error.edit_apply_failed'), 'error')
      console.error('Failed to apply edits:', e)
    }
  }

  function hasPendingEdits(msg: ChatMessage): boolean {
    return pendingEditsOf(msg).length > 0
  }

  let messages = $state<ChatMessage[]>([])
  let inputText = $state('')
  let loading = $state(true)
  let sending = $state(false)
  let configured = $state(true)
  let aiMode = $state<'edit' | 'suggest' | 'chat'>('edit')
  let messagesContainerEl = $state<HTMLDivElement | null>(null)
  let textareaEl = $state<HTMLTextAreaElement | null>(null)

  async function loadChat() {
    loading = true
    try {
      if (projectMode && loadFn) {
        const [result, isConfigured, llmCfg] = await Promise.all([loadFn(), IsLLMConfigured(), GetLLMConfig()])
        messages = result?.messages || []
        configured = isConfigured
        const mode = llmCfg?.ai_mode
        aiMode = (mode === 'chat' || mode === 'suggest') ? mode : 'edit'
      } else {
        const [result, isConfigured, llmCfg] = await Promise.all([
          LoadChatHistory(cardId),
          IsLLMConfigured(),
          GetLLMConfig(),
        ])
        messages = result?.messages || []
        configured = isConfigured
        const mode = llmCfg?.ai_mode
        aiMode = (mode === 'chat' || mode === 'suggest') ? mode : 'edit'
      }
    } catch (e) {
      console.error('Failed to load chat:', e)
    }
    loading = false
  }

  async function toggleMode() {
    const cycle: Record<'edit' | 'suggest' | 'chat', 'edit' | 'suggest' | 'chat'> = {
      edit: 'suggest', suggest: 'chat', chat: 'edit',
    }
    const next = cycle[aiMode]
    aiMode = next
    try {
      const llmCfg = await GetLLMConfig()
      await SetLLMConfig({ ...llmCfg, ai_mode: next })
    } catch (e) {
      console.error('Failed to save AI mode:', e)
    }
  }

  async function send() {
    const text = inputText.trim()
    if (!text || sending) return
    sending = true
    inputText = ''
    resetTextareaHeight()
    // Optimistic user message so the thinking indicator appears immediately
    messages = [...messages, { id: `temp-${Date.now()}`, role: 'user', content: text, timestamp: new Date().toISOString() }]
    await tick()
    scrollToBottom()
    try {
      const result = projectMode && sendFn
        ? await sendFn(text)
        : await SendChatMessage(cardId, text)
      messages = result?.messages || []
      if (projectMode) {
        // Refresh board and sidebar after project-level tool actions
        const lastMsg = messages[messages.length - 1]
        if (lastMsg?.tool_actions?.length) {
          document.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
          document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
          document.dispatchEvent(new CustomEvent('bruv:board-changed'))
        }
      } else {
        // Notify parent that card data may have changed (AI may have set type, blocks, tags)
        onCardChanged?.()
        // Refresh sidebar in case LLM created new hierarchy
        document.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
        document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      }
    } catch (e) {
      console.error('Failed to send message:', e)
    }
    sending = false
  }

  async function acceptPin(msgId: string) {
    try {
      await AcceptPinSuggestion(cardId, msgId)
      // Update the local message state
      messages = messages.map(m =>
        m.id === msgId && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'accepted' } }
          : m
      )
      onCardChanged?.()
      // Fire events so inbox and sidebar re-fetch
      document.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      document.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
    } catch (e) {
      console.error('Failed to accept pin:', e)
    }
  }

  async function rejectPin(msgId: string) {
    try {
      await RejectPinSuggestion(cardId, msgId)
      messages = messages.map(m =>
        m.id === msgId && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'rejected' } }
          : m
      )
    } catch (e) {
      console.error('Failed to reject pin:', e)
    }
  }


  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  function formatTime(ts: string): string {
    try {
      const d = new Date(ts)
      return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    } catch {
      return ''
    }
  }

  function toolActionLabel(action: { tool: string; input: unknown; result: string }): string {
    const inp = (action.input ?? {}) as Record<string, unknown>
    switch (action.tool) {
      case 'set_title': return `Title: ${inp.title || '?'}`
      case 'set_due_date': return inp.due_date ? `Due: ${inp.due_date}` : 'Cleared due date'
      case 'set_card_type': return `Set type: ${inp.card_type || '?'}`
      case 'set_fields':
      case 'update_blocks': {
        const fields = (inp.fields || inp.blocks) as Record<string, unknown> | undefined
        const keys = fields ? Object.keys(fields) : []
        return `Updated: ${keys.join(', ') || '?'}`
      }
      case 'add_tags': return `Added tags: ${(inp.tags as string[] || []).join(', ')}`
      case 'add_field': return `Added field: ${inp.label || inp.key || '?'} (${inp.field_type || '?'})`
      case 'suggest_pin': return `Suggested pin: ${action.result || '?'}`
      // Project-level tools
      case 'create_card': return `Created card: ${inp.title || '?'}`
      case 'add_tags_to_cards': return `Tagged ${(inp.card_ids as string[] || []).length} cards: ${(inp.tags as string[] || []).join(', ')}`
      case 'move_card': return `Moved card`
      case 'update_card': return action.result || 'Updated card'
      default: return action.tool
    }
  }

  /** Extract card ID from a create_card tool result string */
  function extractCardId(result: string): string | null {
    const match = result.match(/\(ID: ([^)]+)\)/)
    return match ? match[1] : null
  }

  function openCreatedCard(cardId: string) {
    document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'card', id: cardId } }))
  }

  async function clearChat() {
    const confirmed = await showConfirm(t('chat.clear_confirm'))
    if (!confirmed) return
    try {
      if (clearFn) {
        await clearFn()
      } else {
        await ClearCardChatHistory(cardId)
      }
      messages = []
      showToast(t('chat.cleared'), 'success')
    } catch (e) {
      showToast(t('error.delete_failed'), 'error')
    }
  }

  function scrollToBottom() {
    if (messagesContainerEl) messagesContainerEl.scrollTop = messagesContainerEl.scrollHeight
  }

  function autoGrowTextarea() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, 150)}px`
  }

  function resetTextareaHeight() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
  }

  function getUserMessageEls(): HTMLElement[] {
    if (!messagesContainerEl) return []
    return Array.from(messagesContainerEl.querySelectorAll('.chat-msg-user'))
  }

  function scrollToPrevQuestion() {
    const container = messagesContainerEl
    if (!container) return
    const userMsgs = getUserMessageEls()
    if (userMsgs.length === 0) return
    const scrollTop = container.scrollTop
    for (let i = userMsgs.length - 1; i >= 0; i--) {
      const top = userMsgs[i].offsetTop - container.offsetTop
      if (top < scrollTop - 2) {
        userMsgs[i].scrollIntoView({ behavior: 'smooth', block: 'start' })
        return
      }
    }
    userMsgs[0].scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  function scrollToNextQuestion() {
    const container = messagesContainerEl
    if (!container) return
    const userMsgs = getUserMessageEls()
    if (userMsgs.length === 0) return
    const scrollTop = container.scrollTop
    for (let i = 0; i < userMsgs.length; i++) {
      const top = userMsgs[i].offsetTop - container.offsetTop
      if (top > scrollTop + 2) {
        userMsgs[i].scrollIntoView({ behavior: 'smooth', block: 'start' })
        return
      }
    }
    scrollToBottom()
  }

  // Auto-scroll to bottom when messages change or sending state changes.
  $effect(() => {
    void messages.length
    void sending
    tick().then(scrollToBottom)
  })

  $effect(() => {
    void reloadKey  // re-run when project changes
    if (visible) loadChat()
  })
</script>

{#if tooltipVisible && tooltipText}
  <div
    class="edit-tooltip-fixed"
    style="left: {tooltipX}px; top: {tooltipY}px;"
  >{tooltipText}</div>
{/if}

{#if visible}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="chat-panel" class:resizing style="width: {chatWidth}px;">
    <div class="chat-resize-handle left" class:active={resizing} role="separator" tabindex="-1" onmousedown={onSplitterDown}></div>
    <div class="chat-header">
      <span class="chat-title">{projectMode ? t('chat.project_title') : t('chat.title')}{messages.length > 0 ? ` (${messages.length})` : ''}</span>
    </div>

    {#if !configured}
      <div class="chat-banner">
        {@html t('chat.not_configured')}
      </div>
    {/if}

    <div class="chat-messages" bind:this={messagesContainerEl}>
      {#if loading}
        <div class="chat-empty">{t('chat.loading')}</div>
      {:else if messages.length === 0}
        <div class="chat-empty">{t('chat.empty')}</div>
      {:else}
        {#each messages as msg (msg.id)}
          <div class="chat-msg chat-msg-{msg.role}">
            <div class="chat-msg-content">{@html renderMarkdown(msg.content)}</div>

            {#if msg.tool_actions?.length}
              <div class="tool-actions">
                {#each msg.tool_actions as action}
                  <div class="tool-action">
                    <Wrench size={10} />
                    <span>{toolActionLabel(action)}</span>
                    {#if action.tool === 'create_card' && extractCardId(action.result)}
                      <button class="open-card-link" onclick={() => openCreatedCard(extractCardId(action.result)!)}>
                        {t('chat.open_card')}
                      </button>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}

            {#if !projectMode && msg.pending_edits?.length}
              <div class="pending-edits">
                <div class="pending-edits-header">
                  {#if hasPendingEdits(msg)}
                    <input
                      type="checkbox"
                      class="select-all-cb"
                      checked={allChecked(msg)}
                      indeterminate={someChecked(msg) && !allChecked(msg)}
                      onchange={() => toggleAll(msg)}
                      aria-label="Select all"
                    />
                  {:else}
                    <ListChecks size={12} class="resolved-icon" />
                  {/if}
                  <span class="pending-edits-title">{t('chat.suggest_edits_review')}</span>
                  {#if hasPendingEdits(msg)}
                    <button class="apply-btn" onclick={() => applyEdits(msg.id)} disabled={!someChecked(msg)}>
                      {t('chat.apply')}
                    </button>
                  {/if}
                </div>

                {#each msg.pending_edits as edit (edit.id)}
                  <div
                    class="pending-edit-row"
                    class:edit-accepted={edit.status === 'accepted'}
                    class:edit-rejected={edit.status === 'rejected'}
                  >
                    {#if edit.status === 'pending'}
                      <input
                        type="checkbox"
                        class="edit-cb"
                        checked={checkedEdits[msg.id]?.[edit.id] ?? false}
                        onchange={() => toggleEdit(msg.id, edit.id)}
                      />
                    {:else if edit.status === 'accepted'}
                      <Check size={11} class="edit-resolved-icon accepted" />
                    {:else}
                      <X size={11} class="edit-resolved-icon rejected" />
                    {/if}
                    <span class="edit-label">{edit.label}</span>
                    {#if edit.detail}
                      <span
                        class="edit-preview"
                        role="tooltip"
                        onmouseenter={(e) => showEditTooltip(e, edit.detail)}
                        onmouseleave={hideEditTooltip}
                      >{edit.detail}</span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}

            {#if !projectMode && msg.pin_suggestion}
              <div class="pin-suggestion" class:accepted={msg.pin_suggestion.status === 'accepted'} class:rejected={msg.pin_suggestion.status === 'rejected'}>
                <div class="pin-suggestion-header">
                  <MapPin size={12} />
                  <span class="pin-suggestion-label">{t('chat.pin_suggestion')}</span>
                </div>
                <div class="pin-suggestion-breadcrumb">{msg.pin_suggestion.breadcrumb}</div>
                {#if msg.pin_suggestion.reason}
                  <div class="pin-suggestion-reason">{msg.pin_suggestion.reason}</div>
                {/if}
                {#if msg.pin_suggestion.status === 'pending'}
                  <div class="pin-suggestion-actions">
                    <button class="pin-btn pin-accept" onclick={() => acceptPin(msg.id)} title={t('tooltip.accept_pin')}>
                      <Check size={12} /> {t('chat.pin_accept')}
                    </button>
                    <button class="pin-btn pin-reject" onclick={() => rejectPin(msg.id)} title={t('tooltip.dismiss_pin')}>
                      <X size={12} /> {t('chat.pin_dismiss')}
                    </button>
                  </div>
                {:else if msg.pin_suggestion.status === 'accepted'}
                  <div class="pin-suggestion-resolved">{t('chat.pin_accepted')}</div>
                {:else}
                  <div class="pin-suggestion-resolved">{t('chat.pin_dismissed')}</div>
                {/if}
              </div>
            {/if}

            <span class="chat-msg-time">{formatTime(msg.timestamp)}</span>
          </div>
        {/each}
        {#if sending}
          <div class="chat-msg chat-msg-assistant thinking">
            <span class="dot"></span><span class="dot"></span><span class="dot"></span>
          </div>
        {/if}
      {/if}
    </div>

    <div class="chat-input-wrapper">
      <div class="chat-nav-row">
        <button class="chat-nav-btn" onclick={scrollToPrevQuestion} title={t('chat.prev_question')}>
          <ChevronUp size={14} />
        </button>
        <button class="chat-nav-btn" onclick={scrollToNextQuestion} title={t('chat.next_question')}>
          <ChevronDown size={14} />
        </button>
        {#if messages.length > 0}
          <button class="chat-nav-btn chat-nav-clear" onclick={clearChat} title={t('chat.clear')}>
            <Trash2 size={12} />
          </button>
        {/if}
      </div>

      <div class="chat-input-row">
        <textarea
          bind:this={textareaEl}
          bind:value={inputText}
          onkeydown={handleKeydown}
          oninput={autoGrowTextarea}
          placeholder={t('chat.placeholder')}
          disabled={sending}
          rows="2"
        ></textarea>
        <div class="chat-actions-col">
        <button
          class="mode-btn"
          class:mode-chat={aiMode === 'chat'}
          class:mode-suggest={aiMode === 'suggest'}
          class:mode-edit={aiMode === 'edit'}
          onclick={toggleMode}
          title={aiMode === 'edit' ? t('chat.mode_edit_hint') : aiMode === 'suggest' ? t('chat.mode_suggest_hint') : t('chat.mode_chat_hint')}
        >
          {#if aiMode === 'edit'}
            <PencilLine size={11} />
            {t('chat.mode_edit')}
          {:else if aiMode === 'suggest'}
            <ListChecks size={11} />
            {t('chat.mode_suggest')}
          {:else}
            <MessageCircle size={11} />
            {t('chat.mode_chat')}
          {/if}
        </button>
        <button class="chat-send-btn" onclick={send} disabled={!inputText.trim() || sending}>
          <Send size={14} />
          {t('chat.send')}
        </button>
      </div>
    </div>
    </div>
  </div>
{/if}

<style>
  .chat-panel {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border-muted);
    background: var(--bg-surface);
    position: relative;
  }

  .chat-panel.resizing {
    user-select: none;
  }

  .chat-resize-handle {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 5px;
    z-index: 5;
    transition: background 0.15s;
  }
  .chat-resize-handle.left {
    left: 0;
    cursor: col-resize;
    border-radius: 0;
  }
  .chat-resize-handle:hover,
  .chat-resize-handle.active {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent-glow-1);
  }

  .chat-header {
    padding: 0.65rem 0.75rem 0.65rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .chat-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .chat-actions-col {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex-shrink: 0;
    width: 72px;
  }

  .mode-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 4px 0;
    border-radius: 6px;
    font-size: 0.7rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-muted);
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    width: 100%;
  }
  .mode-btn:hover {
    color: var(--text-primary);
  }
  .mode-btn.mode-chat {
    color: #22c55e;
    border-color: rgba(34, 197, 94, 0.5);
  }
  .mode-btn.mode-suggest {
    color: #f59e0b;
    border-color: rgba(245, 158, 11, 0.5);
  }
  .mode-btn.mode-edit {
    color: #ef4444;
    border-color: rgba(239, 68, 68, 0.5);
  }

  .chat-banner {
    padding: 0.5rem 0.75rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    background: var(--bg-elevated);
    border-bottom: 1px solid var(--border-muted);
    line-height: 1.4;
    flex-shrink: 0;
  }

  .chat-messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 0.75rem;
  }

  .chat-empty {
    text-align: center;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 2rem 0.5rem;
  }

  .chat-msg {
    max-width: 90%;
    padding: 6px 10px;
    border-radius: 8px;
    font-size: 0.85rem;
    line-height: 1.4;
  }
  .chat-msg-user {
    align-self: flex-end;
    background: var(--accent);
    color: #fff;
  }
  .chat-msg-assistant {
    align-self: flex-start;
    background: var(--bg-elevated);
    color: var(--text-primary);
  }
  .chat-msg-system {
    align-self: center;
    background: none;
    color: var(--text-muted);
    font-size: 0.75rem;
    font-style: italic;
    max-width: 100%;
  }

  .chat-msg-content :global(p) {
    margin: 0;
  }
  .chat-msg-content :global(p + p) {
    margin-top: 4px;
  }

  .chat-msg-time {
    display: block;
    font-size: 0.65rem;
    color: var(--text-muted);
    margin-top: 2px;
  }
  .chat-msg-user .chat-msg-time {
    color: rgba(255,255,255,0.6);
    text-align: right;
  }

  /* Tool actions */
  .tool-actions {
    margin-top: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .tool-action {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.7rem;
    color: var(--text-muted);
    padding: 2px 0;
  }
  .open-card-link {
    background: none;
    border: none;
    color: var(--accent);
    font-size: 0.7rem;
    font-weight: 500;
    cursor: pointer;
    padding: 0 2px;
    text-decoration: underline;
  }
  .open-card-link:hover {
    color: var(--accent-hover, var(--accent));
  }

  /* Pin suggestion */
  .pin-suggestion {
    margin-top: 6px;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-surface);
  }
  .pin-suggestion.accepted {
    border-color: var(--success, #22c55e);
    background: color-mix(in srgb, var(--success, #22c55e) 8%, var(--bg-surface));
  }
  .pin-suggestion.rejected {
    opacity: 0.5;
  }

  .pin-suggestion-header {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--accent);
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .pin-suggestion-breadcrumb {
    font-size: 0.8rem;
    color: var(--text-primary);
    margin-top: 2px;
    font-weight: 500;
  }
  .pin-suggestion-reason {
    font-size: 0.72rem;
    color: var(--text-muted);
    margin-top: 2px;
    line-height: 1.3;
  }
  .pin-suggestion-actions {
    display: flex;
    gap: 6px;
    margin-top: 6px;
  }
  .pin-btn {
    display: flex;
    align-items: center;
    gap: 3px;
    padding: 3px 8px;
    border-radius: 4px;
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
  }
  .pin-accept:hover {
    border-color: var(--success, #22c55e);
    color: var(--success, #22c55e);
    background: color-mix(in srgb, var(--success, #22c55e) 10%, var(--bg-elevated));
  }
  .pin-reject:hover {
    border-color: var(--danger, #ef4444);
    color: var(--danger, #ef4444);
  }
  .pin-suggestion-resolved {
    font-size: 0.7rem;
    color: var(--text-muted);
    margin-top: 4px;
    font-style: italic;
  }

  /* Pending edits (Suggest mode) */
  .pending-edits {
    margin-top: 6px;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-surface);
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .pending-edits-header {
    display: flex;
    align-items: center;
    gap: 5px;
    margin-bottom: 2px;
  }

  .pending-edits-title {
    flex: 1;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--accent);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  :global(.resolved-icon) {
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .select-all-cb,
  .edit-cb {
    flex-shrink: 0;
    width: 13px;
    height: 13px;
    cursor: pointer;
    accent-color: var(--accent);
  }

  .apply-btn {
    flex-shrink: 0;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elevated));
    color: var(--accent);
    transition: background 0.15s;
  }
  .apply-btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--accent) 22%, var(--bg-elevated));
  }
  .apply-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .pending-edit-row {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 2px 0;
    min-width: 0;
  }

  .pending-edit-row.edit-rejected .edit-label,
  .pending-edit-row.edit-rejected .edit-preview {
    text-decoration: line-through;
    opacity: 0.45;
  }

  .pending-edit-row.edit-accepted .edit-label,
  .pending-edit-row.edit-accepted .edit-preview {
    opacity: 0.6;
  }

  :global(.edit-resolved-icon) {
    flex-shrink: 0;
  }
  :global(.edit-resolved-icon.accepted) {
    color: var(--success, #22c55e);
  }
  :global(.edit-resolved-icon.rejected) {
    color: var(--text-muted);
    opacity: 0.45;
  }

  .edit-label {
    flex-shrink: 0;
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
  }

  .edit-preview {
    flex: 1;
    font-size: 0.72rem;
    color: var(--text-muted);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: default;
  }

  :global(.edit-tooltip-fixed) {
    position: fixed;
    min-width: 160px;
    max-width: 260px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 5px 8px;
    font-size: 0.72rem;
    color: var(--text-secondary);
    white-space: pre-wrap;
    line-height: 1.4;
    z-index: 9999;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
    pointer-events: none;
  }

  /* Thinking indicator */
  .thinking {
    display: flex;
    gap: 4px;
    padding: 10px 14px;
  }
  .thinking .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: bounce 1.2s infinite;
  }
  .thinking .dot:nth-child(2) { animation-delay: 0.2s; }
  .thinking .dot:nth-child(3) { animation-delay: 0.4s; }

  @keyframes bounce {
    0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
    30% { transform: translateY(-4px); opacity: 1; }
  }

  .chat-input-wrapper {
    position: relative;
    flex-shrink: 0;
    border-top: 1px solid var(--border-muted);
  }

  .chat-nav-row {
    position: absolute;
    top: -12px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 4px;
    z-index: 2;
  }
  .chat-nav-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 18px;
    background: var(--bg-surface);
    border: 1px solid color-mix(in srgb, var(--border) 40%, transparent);
    border-radius: 4px;
    color: color-mix(in srgb, var(--text-muted) 40%, transparent);
    cursor: pointer;
    padding: 0;
    transition: color 0.15s ease, border-color 0.15s ease;
  }
  .chat-nav-btn:hover {
    color: var(--text-primary);
    border-color: var(--accent);
  }
  .chat-nav-clear:hover {
    color: var(--color-error, #ef4444);
    border-color: var(--color-error, #ef4444);
  }

  .chat-input-row {
    display: flex;
    gap: 6px;
    align-items: stretch;
    padding: 0.65rem 0.75rem 0.5rem;
  }

  .chat-input-row textarea {
    flex: 1;
    resize: none;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    padding: 6px 10px;
    font-size: 0.85rem;
    font-family: inherit;
    line-height: 1.4;
    min-height: 52px;
    max-height: 150px;
    overflow-y: auto;
  }
  .chat-input-row textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .chat-send-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    width: 100%;
    flex: 1;
    background: var(--accent);
    border: none;
    border-radius: 6px;
    color: #fff;
    font-size: 0.7rem;
    font-weight: 500;
    cursor: pointer;
  }
  .chat-send-btn:hover:not(:disabled) {
    background: var(--accent-hover);
  }
  .chat-send-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
