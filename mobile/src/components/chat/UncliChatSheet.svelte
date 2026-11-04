<script lang="ts">
  // A chat on the UNCLI engine (opted into in the desktop's Settings → AI),
  // a card's or a project's: UNCLI's <uncli-chat> in the chat sheet,
  // showing the chat's session (shared/uncliChat.openChatSession), in its
  // compact layout by its width. The element shows UNCLI's own download
  // and sign-in steps until its CLI is ready, and approvals inline. BRUV
  // supplies its sign-in, @mentions of cards, bruv: links and what the
  // chat is about as each turn's context (chatHost). The header names the
  // chat, so it's clear which one you're typing into, and New chat starts
  // it afresh.
  import { onMount, untrack } from 'svelte'
  import { SquarePen, X } from 'lucide-svelte'
  import { aiModeOf, aiModeTurnContext, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import { chatHost, openChatSession, startNewChat, uncliSrc, UNCLI_IDLE, UNCLI_LINK_SCHEMES, type ChatSessions, type UncliChatElement } from '@shared/uncliChat'
  import type { LLMConfig, SearchResult } from '@shared/types'
  import type { ChatFile } from './types'
  import { machineRPC, readEnrolment, repoRPC } from '../../lib/auth'
  import { replace } from '../../lib/router.svelte'
  import { followBruv } from '../../lib/mentions.svelte'
  import { theme } from '../../lib/theme.svelte'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheetShell from './ChatSheetShell.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import ChatModeButton from './ChatModeButton.svelte'
  import UncliPendingEdits from './UncliPendingEdits.svelte'

  let {
    title,
    sessions,
    context,
    history,
    onClose,
  }: {
    /** What the chat is about (the card's or project's name). */
    title: string
    sessions: ChatSessions
    /** Told to the model with each turn, with the chat mode. */
    context: () => string | null
    /** The chat's history, where Suggest mode stages its edits for review. */
    history: {
      load: () => Promise<ChatFile>
      apply: (messageId: string, acceptIDs: string[]) => Promise<ChatFile>
    }
    onClose: () => void
  } = $props()

  let src = $state('')
  let token = ''
  let error = $state('')
  // A failure that leaves the chat usable (the mode, the review).
  let notice = $state('')
  let node = $state<HTMLElement | null>(null)
  let confirmingNew = $state(false)
  let mode = $state<AIMode>('edit')
  // Bumped when a turn finishes, so the review banner looks again.
  let turns = $state(0)
  // Set when a link navigates: replace() consumes the sheet's history
  // entry (ChatSheetShell.navigatingAway).
  let navigatedAway = false

  // The element (about 200 KB) loads when a chat first opens.
  onMount(() => {
    const enrolment = readEnrolment()
    if (!enrolment) {
      error = t('uncli.load_failed', { detail: t('uncli.not_paired') })
      return
    }
    token = enrolment.deviceToken
    Promise.all([machineRPC<LLMConfig>('GetLLMConfig'), import('@uncli/chat/element')])
      .then(([cfg]) => {
        mode = aiModeOf(cfg?.ai_mode)
        src = uncliSrc(enrolment.serverURL)
      })
      .catch(e => { error = t('uncli.load_failed', { detail: errorDetail(e) }) })
  })

  // A finished turn may have staged edits for review.
  $effect(() => {
    const el = node
    if (!el) return
    const onIdle = () => turns++
    el.addEventListener(UNCLI_IDLE, onIdle)
    return () => el.removeEventListener(UNCLI_IDLE, onIdle)
  })

  function follow(target: string) {
    navigatedAway = true
    onClose()
    followBruv(target, replace)
  }

  // The host before src: the element connects as soon as it has a src,
  // and its first requests must already carry BRUV's sign-in. Then the
  // chat's session.
  $effect(() => {
    const el = node as UncliChatElement | null
    if (!el || !src) return
    untrack(() => {
      el.host = chatHost({
        token: () => token,
        follow,
        searchCards: q => repoRPC<SearchResult[]>('SearchCards', [q, 8]),
        context: () => [context(), aiModeTurnContext(mode)].filter(Boolean).join('\n'),
        cardLabel: t('uncli.mention_card'),
        enter: 'newline', // as ChatSheet: Enter a new line, Send (or Ctrl+Enter) sends
      })
      el.setAttribute('src', src)
      openChatSession(el, sessions).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
    })
  })

  async function newChat() {
    confirmingNew = false
    const el = node as UncliChatElement | null
    if (!el) return
    try {
      await startNewChat(el, sessions)
    } catch (e) {
      notice = t('uncli.new_chat_failed', { detail: errorDetail(e) })
    }
  }

  // Escape closes the sheet, unless it's for the chat itself (its own
  // menus and dialogs, inside its shadow root) or the confirm dialog.
  function onKeydown(e: KeyboardEvent) {
    if (confirmingNew || e.key !== 'Escape' || (node && e.composedPath().includes(node))) return
    e.preventDefault()
    e.stopPropagation()
    onClose()
  }
</script>

<ChatSheetShell {title} note={t('uncli.engine_badge')} {onClose} {onKeydown} navigatingAway={() => navigatedAway}>
  {#snippet actions()}
    {#if src && !error}
      <button type="button" class="icon-btn" onclick={() => (confirmingNew = true)} aria-label={t('uncli.new_chat')} title={t('uncli.new_chat')}>
        <SquarePen size={16} />
      </button>
    {/if}
  {/snippet}
  {#if notice}
    <div class="notice" role="alert">
      <span>{notice}</span>
      <button type="button" class="notice-dismiss" onclick={() => (notice = '')} aria-label={t('toast.dismiss')}><X size={14} /></button>
    </div>
  {/if}
  {#if error}
    <p class="status error" role="alert">{error}</p>
  {:else if src}
    <!-- src is set by the effect above, after the host. Slotted children
         stay in BRUV's DOM and styles. -->
    <uncli-chat bind:this={node} scheme={theme.current === 'auto' ? undefined : theme.current} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-chat">
      <div slot="header-extra">
        <UncliPendingEdits load={history.load} apply={history.apply} reloadKey={turns} onerror={(message) => (notice = message)} />
      </div>
      <div slot="below-composer" class="mode">
        <ChatModeButton bind:mode onerror={(message) => (notice = message)} />
      </div>
    </uncli-chat>
  {:else}
    <p class="status">{t('chat.loading')}</p>
  {/if}
</ChatSheetShell>

{#if confirmingNew}
  <ConfirmDialog
    title={t('uncli.new_chat')}
    body={t('uncli.new_chat_confirm')}
    confirmLabel={t('uncli.new_chat')}
    onConfirm={newChat}
    onCancel={() => (confirmingNew = false)}
  />
{/if}

<style>
  uncli-chat {
    display: block;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }
  /* The mode button, under the composer, sized like ChatSheet's. */
  .mode {
    display: flex;
    justify-content: flex-end;
    padding: 0 0.85rem 0.5rem;
  }
  .mode > :global(*) {
    width: 80px;
  }
  .notice {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin: 0.5rem 0.85rem 0;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--danger-border);
    border-radius: 8px;
    background: var(--danger-bg);
    color: var(--danger-text);
    font-size: 0.8rem;
  }
  .notice span {
    flex: 1;
  }
  .notice-dismiss {
    display: inline-flex;
    padding: 0.15rem;
    border: none;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .status {
    margin: 1rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .error {
    color: var(--danger-text);
  }
</style>
