<script lang="ts">
  // A chat on the UNCLI engine (opt-in, Settings → AI): UNCLI's
  // <uncli-chat> in a card's chat panel or the project side panel,
  // showing the chat's session (shared/uncliChat.openChatSession). The
  // element shows UNCLI's own download and sign-in steps until its CLI is
  // ready, and approvals inline. BRUV supplies its sign-in, @mentions of
  // cards, bruv: links and what the chat is about as each turn's context
  // (chatHost). The header names the chat, so it's clear which one you're
  // typing into, and New chat starts it afresh.
  import { untrack } from 'svelte'
  import { BotMessageSquare, SquarePen } from 'lucide-svelte'
  import { GetLLMConfig, SearchCards } from '@shared/api'
  import { httpBase, resolveTransportInfo } from '@shared/adapters/cloud'
  import { aiModeOf, aiModeTurnContext, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import type { ChatHistory } from '@shared/types'
  import { chatHost, openChatSession, startNewChat, uncliSrc, UNCLI_IDLE, UNCLI_LINK_SCHEMES, type ChatSessions, type UncliChatElement } from '@shared/uncliChat'
  import ChatPanelShell from './ChatPanelShell.svelte'
  import ChatModeButton from './ChatModeButton.svelte'
  import UncliPendingEdits from './UncliPendingEdits.svelte'
  import { followBruv } from '../lib/bruvLinks'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { theme } from '../lib/theme.svelte'
  import { t } from '../lib/i18n.svelte'

  let {
    chatKey,
    title,
    sessions,
    context,
    history,
    onEditsApplied,
    visible,
    hosted = false,
    mainWidth = $bindable(720),
    splitterDragging = $bindable(false),
  }: {
    /** Which chat this is (a card id, a project path): a new key reopens. */
    chatKey: string
    /** What the chat is about, shown in its header. */
    title: string
    sessions: ChatSessions
    /** Told to the model with each turn, with the chat mode. */
    context: () => string | null
    /** The chat's history, where Suggest mode stages its edits for review
     *  (UncliPendingEdits). */
    history: {
      load: () => Promise<ChatHistory>
      apply: (messageId: string, acceptIDs: string[]) => Promise<ChatHistory>
    }
    /** Reviewed edits were applied. */
    onEditsApplied?: () => void
    visible: boolean
    /** In the project SidePanel, which owns geometry. */
    hosted?: boolean
    mainWidth?: number
    splitterDragging?: boolean
  } = $props()

  let src = $state('')
  let token = ''
  let error = $state('')
  let node = $state<HTMLElement | null>(null)
  let mode = $state<AIMode>('edit')
  // Bumped when a turn finishes, so the review banner looks again.
  let turns = $state(0)

  // The element (about 200 KB) and the server's address load when a
  // chat first opens, not with the app.
  async function prepare() {
    try {
      const [info, cfg] = await Promise.all([resolveTransportInfo(), GetLLMConfig(), import('@uncli/chat/element')])
      token = info.token
      mode = aiModeOf(cfg?.ai_mode)
      src = uncliSrc(httpBase(info))
    } catch (e) {
      error = t('uncli.load_failed', { detail: errorDetail(e) })
    }
  }
  $effect(() => {
    if (visible && !src && !error) untrack(() => void prepare())
  })

  // The host before src: the element connects as soon as it has a src,
  // and its first requests must already carry BRUV's sign-in. Then the
  // chat's session, again whenever the chat changes.
  $effect(() => {
    const el = node as UncliChatElement | null
    void chatKey
    if (!el || !src) return
    untrack(() => {
      el.host = chatHost({
        token: () => token,
        follow: followBruv,
        searchCards: q => SearchCards(q, 8),
        context: () => [context(), aiModeTurnContext(mode)].filter(Boolean).join('\n'),
        cardLabel: t('uncli.mention_card'),
        enter: 'send', // as ChatSection: Enter sends, Shift+Enter a new line
      })
      el.setAttribute('src', src)
      openChatSession(el, sessions).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
    })
  })

  // A finished turn may have staged edits for review.
  $effect(() => {
    const el = node
    if (!el) return
    const onIdle = () => turns++
    el.addEventListener(UNCLI_IDLE, onIdle)
    return () => el.removeEventListener(UNCLI_IDLE, onIdle)
  })

  async function newChat() {
    const el = node as UncliChatElement | null
    if (!el || !(await showConfirm(t('uncli.new_chat_confirm')))) return
    try {
      await startNewChat(el, sessions)
    } catch (e) {
      showToast(t('uncli.new_chat_failed', { detail: errorDetail(e) }), 'error')
    }
  }
</script>

<ChatPanelShell {visible} {hosted} bind:mainWidth bind:splitterDragging>
  <div class="uncli-header" class:hosted>
    <BotMessageSquare size={15} />
    <span class="uncli-title" {title}>{title}</span>
    <span class="uncli-engine" title={t('uncli.engine_hint')}>{t('uncli.engine_badge')}</span>
    {#if src && !error}
      <button type="button" class="uncli-new" onclick={newChat} title={t('uncli.new_chat')} aria-label={t('uncli.new_chat')}>
        <SquarePen size={14} />
      </button>
    {/if}
  </div>
  {#if error}
    <p class="uncli-error" role="alert">{error}</p>
  {:else if src}
    <!-- src is set by the effect above, after the host. Slotted children
         stay in BRUV's DOM and styles. -->
    <uncli-chat bind:this={node} scheme={theme.mode} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-chat">
      <div slot="header-extra">
        <UncliPendingEdits load={history.load} apply={history.apply} reloadKey={`${chatKey}:${turns}`} onApplied={onEditsApplied} />
      </div>
      <div slot="below-composer" class="uncli-mode">
        <ChatModeButton bind:mode />
      </div>
    </uncli-chat>
  {:else}
    <p class="uncli-loading">{t('chat.loading')}</p>
  {/if}
</ChatPanelShell>

<style>
  /* Matches ChatSection's header: icon + Proper Case title (UI-CONVENTIONS
     §13). A card chat's header keeps clear of CardDetail's close button
     at the top right; the hosted (project) one has none. */
  .uncli-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.65rem 2.75rem 0.65rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    min-width: 0;
    color: var(--text-strong);
  }
  .uncli-header.hosted {
    padding: 0.55rem 0.75rem;
  }
  .uncli-title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.82rem;
    font-weight: 600;
  }
  .uncli-engine {
    flex-shrink: 0;
    font-size: 0.7rem;
    color: var(--text-secondary);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }
  .uncli-new {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.3rem;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .uncli-new:hover,
  .uncli-new:focus-visible {
    background: var(--bg-hover);
    color: var(--text-strong);
    outline: none;
  }
  /* The mode button, under the composer, sized like ChatSection's. */
  .uncli-mode {
    display: flex;
    justify-content: flex-end;
    padding: 0 0.75rem 0.5rem;
  }
  .uncli-mode > :global(*) {
    width: 72px;
  }
  uncli-chat {
    display: block;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }
  .uncli-error,
  .uncli-loading {
    margin: 1rem;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .uncli-error {
    color: var(--danger-light);
  }
</style>
