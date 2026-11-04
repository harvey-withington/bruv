<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { Send, Trash2 } from 'lucide-svelte'
  import { aiModeOf } from '@shared/aiMode'
  import { repoRPC, machineRPC } from '../../lib/auth'
  import { onReconnect } from '../../lib/connectivity.svelte'
  import { onEvent } from '../../lib/events.svelte'
  import { ChatTurn } from '@shared/chatTurn.svelte'
  import { cardDraftScope, loadDraft, projectDraftScope, saveDraft } from '@shared/drafts'
  import type { ChatProgress } from '@shared/types'
  import { inlineEdit } from '@shared/inlineEdit'
  import { EditScope } from '@shared/editScope'
  import { t } from '../../lib/i18n.svelte'
  import { replace, cardURL } from '../../lib/router.svelte'
  import ChatMessage from './ChatMessage.svelte'
  import ChatModelChip from './ChatModelChip.svelte'
  import ChatTurnStatus from './ChatTurnStatus.svelte'
  import ChatSheetShell from './ChatSheetShell.svelte'
  import ChatModeButton from './ChatModeButton.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import type { ChatScope } from './scope'
  import type {
    ChatMessage as ChatMsg,
    ChatFile,
    LLMConfig,
    AIMode,
    ProjectContextLevel,
  } from './types'

  let {
    scope,
    onClose,
    engineFallback = '',
  }: {
    scope: ChatScope
    onClose: () => void
    /** Why this chat runs here although the user chose the UNCLI engine
     *  (shared/uncliChat.resolveChatEngine); '' when they didn't. */
    engineFallback?: string
  } = $props()

  // Keyboard entry contract. The composer registers while focused;
  // Escape closes the sheet only when it isn't mid-entry. Named
  // editScope because `scope` is already taken by the chat scope prop.
  const editScope = new EditScope()
  editScope.requestClose = () => onClose()

  // Capture phase: the sheet overlays a page (CardPage) that listens
  // for Escape/Ctrl+Enter on window bubble — the sheet must consume
  // those keys before the page underneath reacts to them.
  function onWindowKeydownCapture(e: KeyboardEvent) {
    // The clear-history ConfirmDialog owns Escape while open.
    if (confirmingClear) return
    if (e.key === 'Escape') {
      // An active entry's own handler cancels + consumes Escape (the
      // event targets the focused field, whose listener runs after
      // this capture handler and stops propagation).
      if (editScope.hasActive()) return
      e.preventDefault()
      e.stopPropagation()
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      // Chat exception: with the composer focused, Ctrl+Enter sends
      // WITHOUT closing (the field's own handler consumes the chord).
      if (editScope.hasActive()) return
      e.preventDefault()
      e.stopPropagation()
      editScope.commitAll()
      onClose()
    }
  }

  let messages = $state<ChatMsg[]>([])
  let inputText = $state('')
  // The half-typed message survives closing the sheet and reloads: each
  // chat's draft is loaded when it opens and saved as it changes.
  const draftScope = $derived(
    scope.kind === 'card' ? cardDraftScope(scope.cardID) : projectDraftScope(`${scope.brand}/${scope.stream}/${scope.project}`),
  )
  $effect(() => {
    const key = draftScope
    untrack(() => { inputText = loadDraft(key) })
  })
  $effect(() => saveDraft(draftScope, inputText))
  let loading = $state(true)
  // The turn in flight: elapsed time, live progress, Stop.
  const turn = new ChatTurn(() =>
    scope.kind === 'card' ? { cardId: scope.cardID } : { projectPath: `${scope.brand}/${scope.stream}/${scope.project}` },
  )
  const sending = $derived(turn.running)
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
      saveError = null

      const [cfg, isConfigured] = await Promise.all([cfgPromise, isConfiguredPromise])
      configured = !!isConfigured
      aiMode = aiModeOf(cfg?.ai_mode)
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
    turn.start()
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
      // Drop the optimistic bubble and give the text back so the user can
      // retry — losing a typed message to a failed send is data loss.
      messages = messages.filter((m) => m.id !== tempID)
      if (!inputText.trim()) inputText = text
    } finally {
      turn.finish()
    }
  }

  // Stop the running turn. Its pending send then returns with the turn
  // closed by a "stopped" notice; false means it had already finished.
  async function stop() {
    turn.stopping = true
    try {
      const stopped =
        scope.kind === 'card'
          ? await repoRPC<boolean>('StopChatMessage', [scope.cardID])
          : await repoRPC<boolean>('StopProjectChatMessage', [scope.brand, scope.stream, scope.project])
      if (!stopped) turn.stopping = false
    } catch (err) {
      turn.stopping = false
      saveError = err instanceof Error ? err.message : t('chat.err_stop')
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
      // The batch succeeds as a call even when a tool refused an edit;
      // those rows come back `failed` with the reason on the row.
      const failed =
        messages
          .find((m) => m.id === msgID)
          ?.pending_edits?.filter((e) => acceptIDs.includes(e.id) && e.status === 'failed').length ?? 0
      if (failed > 0) saveError = t('chat.err_apply_some', { count: failed })
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

  // Composer keyboard behaviour comes from the shared inlineEdit action,
  // mobile multiline variant (WhatsApp-style: Enter inserts a newline,
  // the Send button sends; hardware Ctrl+Enter still sends). Cancel
  // (Escape / Back) only dismisses the keyboard — it must NOT destroy
  // the draft: an accidental back-swipe mid-message would be data loss.
  // closeOnCtrlEnter is off — the chat exception: Ctrl+Enter sends
  // without closing the sheet (closing would hide the reply).
  function cancelCompose() {
    textareaEl?.blur()
  }

  onMount(() => {
    void load()
    autoGrow()
    // Replies that landed while the link was down (a send whose response
    // was lost, an agent turn) arrive with the connection. The composer's
    // draft is separate state, so a reload never touches it.
    const offReconnect = onReconnect(() => {
      if (!sending) void load()
    })
    const offProgress = onEvent((ev) => {
      if (ev.topic === 'chat:progress') turn.progress(ev.payload as ChatProgress)
    })
    return () => {
      offReconnect()
      offProgress()
      turn.finish()
    }
  })

  // Set when a card link navigates: replace() consumes the sheet's
  // history entry, so the shell mustn't go back as well.
  let navigatedAway = false

  function openCard(id: string) {
    navigatedAway = true
    onClose()
    replace(cardURL(id))
  }
</script>

<ChatSheetShell
  title={scope.kind === 'card' ? t('chat.title') : t('chat.project_title')}
  note={messages.length > 0 ? `(${messages.length})` : ''}
  {onClose}
  onKeydown={onWindowKeydownCapture}
  navigatingAway={() => navigatedAway}
>
  {#snippet actions()}
    {#if messages.length > 0}
      <button type="button" class="icon-btn" onclick={() => (confirmingClear = true)} aria-label={t('chat.clear')}>
        <Trash2 size={16} />
      </button>
    {/if}
  {/snippet}

  <ChatModelChip {scope} onerror={(message) => (saveError = message)} />

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

  {#if engineFallback}
    <div class="banner" role="status">{engineFallback}</div>
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
        <ChatTurnStatus {turn} onStop={stop} />
      {/if}
    {/if}
  </div>

  <footer class="composer">
    <textarea
      bind:this={textareaEl}
      bind:value={inputText}
      use:inlineEdit={{
        serial: true,
        multiline: true,
        enterInsertsNewline: true,
        closeOnCtrlEnter: false,
        onCommit: () => { void send() },
        onCancel: cancelCompose,
        scope: editScope,
      }}
      oninput={autoGrow}
      placeholder={t('chat.placeholder')}
      disabled={sending}
      rows="2"
    ></textarea>
    <div class="composer-actions">
      <ChatModeButton bind:mode={aiMode} onerror={(message) => (saveError = message)} />
      <button
        type="button"
        class="send-btn"
        disabled={!inputText.trim() || sending}
        onclick={send}
        aria-label={t('chat.send')}
      >
        <Send size={16} />
      </button>
    </div>
  </footer>
</ChatSheetShell>

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

  .composer-actions {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex-shrink: 0;
    width: 80px;
  }


  .send-btn {
    background: var(--accent);
    border: none;
    color: #fff;
    border-radius: 10px;
    width: 100%;
    height: 44px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .send-btn:disabled { opacity: 0.4; cursor: default; }
</style>
