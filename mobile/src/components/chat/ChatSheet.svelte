<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Send, X, Trash2, MessageCircle, PencilLine, ListChecks } from 'lucide-svelte'
  import { repoRPC, machineRPC } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'
  import { navigate, cardURL } from '../../lib/router.svelte'
  import ChatMessage from './ChatMessage.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import type { ChatScope } from './scope'
  import type {
    ChatMessage as ChatMsg,
    ChatFile,
    LLMConfig,
    AIMode,
    ProjectContextLevel,
  } from './types'

  let { scope, onClose }: { scope: ChatScope; onClose: () => void } = $props()

  let messages = $state<ChatMsg[]>([])
  let inputText = $state('')
  let loading = $state(true)
  let sending = $state(false)
  let configured = $state(true)
  let aiMode = $state<AIMode>('edit')
  let saveError = $state<string | null>(null)

  // Project-chat context level (persisted globally — same key the
  // desktop uses, so a setting from one surface follows to the other).
  const PROJECT_CONTEXT_KEY = 'bruv:projectChatContextLevel'
  let projectContextLevel = $state<ProjectContextLevel>(
    (localStorage.getItem(PROJECT_CONTEXT_KEY) as ProjectContextLevel | null) ?? 'all',
  )

  function setContextLevel(level: ProjectContextLevel) {
    projectContextLevel = level
    localStorage.setItem(PROJECT_CONTEXT_KEY, level)
  }

  let messagesEl = $state<HTMLDivElement | null>(null)
  let textareaEl = $state<HTMLTextAreaElement | null>(null)
  let sheetEl = $state<HTMLElement | null>(null)

  // Close-by-swipe: pointer drag from the header that exceeds 40% of
  // viewport height OR a fast downward velocity dismisses the sheet.
  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  async function load() {
    loading = true
    try {
      const cfgPromise = machineRPC<LLMConfig>('GetLLMConfig').catch(() => null)
      const isConfiguredPromise = machineRPC<boolean>('IsLLMConfigured').catch(() => false)

      let history: ChatFile | null = null
      if (scope.kind === 'card') {
        history = await repoRPC<ChatFile>('LoadChatHistory', [scope.cardID])
      } else {
        history = await repoRPC<ChatFile>('LoadProjectChatHistory', [scope.brand, scope.stream, scope.project])
      }
      messages = history?.messages ?? []

      const [cfg, isConfigured] = await Promise.all([cfgPromise, isConfiguredPromise])
      configured = !!isConfigured
      const mode = cfg?.ai_mode
      aiMode = mode === 'chat' || mode === 'suggest' ? mode : 'edit'
    } catch (err) {
      console.error('chat load failed:', err)
      saveError = err instanceof Error ? err.message : t('chat.err_send')
    } finally {
      loading = false
    }
    await tick()
    requestAnimationFrame(scrollToBottom)
  }

  function scrollToBottom() {
    if (messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight
  }

  function isNearBottom(): boolean {
    const c = messagesEl
    if (!c) return true
    return c.scrollHeight - c.scrollTop - c.clientHeight < 120
  }

  // Sticky-to-bottom: only follow new messages when the user was
  // already near the bottom; otherwise leave them alone (they're
  // re-reading something earlier).
  $effect(() => {
    void messages.length
    void sending
    const follow = isNearBottom()
    tick().then(() => {
      if (follow) scrollToBottom()
    })
  })

  function autoGrow() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, 160)}px`
  }

  function resetTextareaHeight() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
  }

  async function send() {
    const text = inputText.trim()
    if (!text || sending) return
    sending = true
    saveError = null
    inputText = ''
    resetTextareaHeight()

    // Optimistic user bubble so the thinking indicator appears
    // immediately. The real response replaces the whole list.
    const tempID = `temp-${Date.now()}`
    messages = [
      ...messages,
      { id: tempID, role: 'user', content: text, timestamp: new Date().toISOString() },
    ]
    await tick()
    scrollToBottom()

    try {
      let result: ChatFile | null = null
      if (scope.kind === 'card') {
        result = await repoRPC<ChatFile>('SendChatMessage', [scope.cardID, text])
      } else {
        result = await repoRPC<ChatFile>('SendProjectChatMessage', [
          scope.brand,
          scope.stream,
          scope.project,
          text,
          projectContextLevel,
        ])
      }
      messages = result?.messages ?? []
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('chat.err_send')
      // Drop the optimistic bubble so the user can retry.
      messages = messages.filter((m) => m.id !== tempID)
    } finally {
      sending = false
    }
  }

  let confirmingClear = $state(false)

  async function performClear() {
    confirmingClear = false
    try {
      if (scope.kind === 'card') {
        await repoRPC('ClearCardChatHistory', [scope.cardID])
      } else {
        await repoRPC('ClearProjectChatHistory', [scope.brand, scope.stream, scope.project])
      }
      messages = []
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('chat.err_send')
    }
  }

  async function applyEdits(msgID: string, acceptIDs: string[]) {
    saveError = null
    try {
      let result: ChatFile | null
      if (scope.kind === 'card') {
        result = await repoRPC<ChatFile>('ApplyPendingEdits', [scope.cardID, msgID, acceptIDs])
      } else {
        result = await repoRPC<ChatFile>('ApplyProjectPendingEdits', [
          scope.brand,
          scope.stream,
          scope.project,
          msgID,
          acceptIDs,
        ])
      }
      messages = result?.messages ?? []
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('chat.err_apply')
    }
  }

  async function acceptPin(msgID: string) {
    if (scope.kind !== 'card') return
    try {
      await repoRPC('AcceptPinSuggestion', [scope.cardID, msgID])
      messages = messages.map((m) =>
        m.id === msgID && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'accepted' } }
          : m,
      )
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('chat.err_send')
    }
  }

  async function rejectPin(msgID: string) {
    if (scope.kind !== 'card') return
    try {
      await repoRPC('RejectPinSuggestion', [scope.cardID, msgID])
      messages = messages.map((m) =>
        m.id === msgID && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'rejected' } }
          : m,
      )
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('chat.err_send')
    }
  }

  async function toggleMode() {
    const cycle: Record<AIMode, AIMode> = { edit: 'suggest', suggest: 'chat', chat: 'edit' }
    aiMode = cycle[aiMode]
    try {
      const cfg = (await machineRPC<LLMConfig>('GetLLMConfig')) ?? ({} as LLMConfig)
      await machineRPC('SetLLMConfig', [{ ...cfg, ai_mode: aiMode }])
    } catch (err) {
      console.error('mode save failed:', err)
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      e.stopPropagation()
      void send()
    }
  }

  // --- Swipe-to-dismiss on the header ---
  function onHeaderPointerDown(e: PointerEvent) {
    dragStartY = e.clientY
    dragCurrentY = e.clientY
    dragging = true
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function onHeaderPointerMove(e: PointerEvent) {
    if (!dragging) return
    dragCurrentY = e.clientY
    const dy = Math.max(0, dragCurrentY - dragStartY) // only positive (down)
    translateY = dy
  }

  function onHeaderPointerUp() {
    if (!dragging) return
    dragging = false
    const dy = dragCurrentY - dragStartY
    const vh = window.innerHeight
    if (dy > vh * 0.4) {
      // Snap closed.
      translateY = vh
      // Match the slide-out animation length so the close happens
      // after the visual settles.
      setTimeout(onClose, 180)
    } else {
      translateY = 0
    }
  }

  onMount(() => {
    void load()
    autoGrow()
    // Push a synthetic history entry so hardware Back closes the sheet.
    history.pushState({ chat: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      // If the sheet is closing for any other reason (button tap),
      // drop the history entry we pushed so we don't leave dangling.
      if (history.state?.chat) history.back()
    }
  })

  function openCard(id: string) {
    onClose()
    navigate(cardURL(id))
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}></div>

<aside
  class="sheet"
  bind:this={sheetEl}
  role="dialog"
  aria-label={scope.kind === 'card' ? t('chat.title') : t('chat.project_title')}
  style:transform={translateY > 0 ? `translateY(${translateY}px)` : undefined}
  style:transition={dragging ? 'none' : undefined}
>
  <header
    class="header"
    onpointerdown={onHeaderPointerDown}
    onpointermove={onHeaderPointerMove}
    onpointerup={onHeaderPointerUp}
    onpointercancel={onHeaderPointerUp}
  >
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">
      {scope.kind === 'card' ? t('chat.title') : t('chat.project_title')}
      {#if messages.length > 0}<span class="count"> ({messages.length})</span>{/if}
    </span>
    {#if messages.length > 0}
      <button type="button" class="icon-btn" onclick={() => (confirmingClear = true)} aria-label={t('chat.clear')}>
        <Trash2 size={16} />
      </button>
    {/if}
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('chat.dismiss')}>
      <X size={18} />
    </button>
  </header>

  {#if scope.kind === 'project'}
    <div class="ctx" role="group" aria-label={t('chat.context_label')}>
      <span class="ctx-label">{t('chat.context_label')}</span>
      <div class="ctx-segments">
        <button
          type="button"
          class="ctx-segment"
          class:active={projectContextLevel === 'all'}
          onclick={() => setContextLevel('all')}
          title={t('chat.context_all_hint')}
        >{t('chat.context_all')}</button>
        <button
          type="button"
          class="ctx-segment"
          class:active={projectContextLevel === 'metadata'}
          onclick={() => setContextLevel('metadata')}
          title={t('chat.context_metadata_hint')}
        >{t('chat.context_metadata')}</button>
        <button
          type="button"
          class="ctx-segment"
          class:active={projectContextLevel === 'none'}
          onclick={() => setContextLevel('none')}
          title={t('chat.context_none_hint')}
        >{t('chat.context_none')}</button>
      </div>
    </div>
  {/if}

  {#if !configured}
    <div class="banner">{t('chat.not_configured')}</div>
  {/if}

  {#if saveError}
    <div class="banner banner-error" role="alert">{saveError}</div>
  {/if}

  <div class="messages" bind:this={messagesEl}>
    {#if loading}
      <p class="status">{t('chat.loading')}</p>
    {:else if messages.length === 0}
      <p class="status">{scope.kind === 'card' ? t('chat.empty') : t('chat.empty_project')}</p>
    {:else}
      {#each messages as m (m.id)}
        <ChatMessage
          msg={m}
          projectMode={scope.kind === 'project'}
          onAcceptPin={acceptPin}
          onRejectPin={rejectPin}
          onApplyEdits={applyEdits}
          onOpenCard={openCard}
        />
      {/each}
      {#if sending}
        <div class="thinking" aria-label={t('chat.thinking')}>
          <span class="dot"></span><span class="dot"></span><span class="dot"></span>
        </div>
      {/if}
    {/if}
  </div>

  <footer class="composer">
    <button
      type="button"
      class="mode-btn mode-{aiMode}"
      onclick={toggleMode}
      title={aiMode === 'edit'
        ? t('chat.mode_edit_hint')
        : aiMode === 'suggest'
          ? t('chat.mode_suggest_hint')
          : t('chat.mode_chat_hint')}
    >
      {#if aiMode === 'edit'}
        <PencilLine size={12} />
        {t('chat.mode_edit')}
      {:else if aiMode === 'suggest'}
        <ListChecks size={12} />
        {t('chat.mode_suggest')}
      {:else}
        <MessageCircle size={12} />
        {t('chat.mode_chat')}
      {/if}
    </button>
    <textarea
      bind:this={textareaEl}
      bind:value={inputText}
      onkeydown={handleKeydown}
      oninput={autoGrow}
      placeholder={t('chat.placeholder')}
      disabled={sending}
      rows="2"
    ></textarea>
    <button
      type="button"
      class="send-btn"
      disabled={!inputText.trim() || sending}
      onclick={send}
      aria-label={t('chat.send')}
    >
      <Send size={16} />
    </button>
  </footer>
</aside>

{#if confirmingClear}
  <ConfirmDialog
    title={t('chat.clear')}
    body={t('chat.clear_confirm')}
    confirmLabel={t('chat.clear')}
    destructive
    onConfirm={performClear}
    onCancel={() => (confirmingClear = false)}
  />
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 200ms ease forwards;
  }

  .sheet {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    height: 90vh;
    background: var(--bg);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-up 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-bottom: env(safe-area-inset-bottom);
    transition: transform 180ms cubic-bezier(0.16, 1, 0.3, 1);
  }

  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem 0.6rem;
    border-bottom: 1px solid var(--border);
    cursor: grab;
    touch-action: none;
  }
  .header:active { cursor: grabbing; }
  .grabber {
    position: absolute;
    top: 6px;
    left: 50%;
    transform: translateX(-50%);
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--text-faint);
    opacity: 0.5;
  }
  .title {
    flex: 1;
    margin-top: 0.4rem;
    font-weight: 600;
    color: var(--text);
    font-size: 0.95rem;
  }
  .count { color: var(--text-faint); font-weight: 400; }
  .icon-btn {
    margin-top: 0.4rem;
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
  }
  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  .ctx {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.85rem;
    border-bottom: 1px solid var(--border);
  }
  .ctx-label {
    font-size: 0.7rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .ctx-segments {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-elev-1);
  }
  .ctx-segment {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.75rem;
    padding: 0.35rem 0.7rem;
    cursor: pointer;
    border-right: 1px solid var(--border);
  }
  .ctx-segment:last-child { border-right: none; }
  .ctx-segment.active {
    background: var(--accent);
    color: #fff;
  }

  .banner {
    padding: 0.55rem 0.85rem;
    background: var(--bg-elev-1);
    color: var(--text-muted);
    font-size: 0.78rem;
    border-bottom: 1px solid var(--border);
  }
  .banner-error {
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border-color: rgba(239, 68, 68, 0.4);
  }

  .messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 0.85rem;
    background: var(--bg);
  }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 1rem 0.5rem;
    font-size: 0.85rem;
  }

  .thinking {
    align-self: flex-start;
    display: inline-flex;
    gap: 4px;
    padding: 10px 14px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: bounce 1.2s infinite;
  }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }

  .composer {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    padding: 0.65rem 0.75rem 0.75rem;
    border-top: 1px solid var(--border);
    background: var(--bg);
  }
  .composer textarea {
    flex: 1;
    min-height: 44px;
    max-height: 160px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    line-height: 1.4;
    padding: 0.6rem 0.75rem;
    resize: none;
    overflow-y: auto;
  }
  .composer textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .mode-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 999px;
    padding: 0.4rem 0.65rem;
    font: inherit;
    font-size: 0.72rem;
    cursor: pointer;
    flex-shrink: 0;
    align-self: flex-end;
    margin-bottom: 1px;
  }
  .mode-btn.mode-edit { color: #ef4444; border-color: rgba(239, 68, 68, 0.5); }
  .mode-btn.mode-suggest { color: #f59e0b; border-color: rgba(245, 158, 11, 0.5); }
  .mode-btn.mode-chat { color: #22c55e; border-color: rgba(34, 197, 94, 0.5); }

  .send-btn {
    background: var(--accent);
    border: none;
    color: #fff;
    border-radius: 10px;
    width: 44px;
    height: 44px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .send-btn:disabled { opacity: 0.4; cursor: default; }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes slide-up {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }
  @keyframes bounce {
    0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
    30% { transform: translateY(-4px); opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .backdrop,
    .sheet {
      animation: fade-in 120ms ease forwards;
    }
    .sheet { transition: none; }
  }
</style>
