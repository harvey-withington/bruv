<script lang="ts">
  // A chat on the UNCLI engine (opted into in the desktop's Settings → AI),
  // a card's or a project's: UNCLI's <uncli-chat> in the chat sheet,
  // showing the chat's session (shared/uncliChat), in its compact layout
  // by its width. The element shows UNCLI's own download and sign-in steps
  // until its CLI is ready, approvals and its own header (New chat,
  // delete, options). BRUV supplies its sign-in, @mentions of cards, bruv:
  // links, what the chat is about as each turn's context, the Chat /
  // Suggest / Edit control in the message box, and Suggest's review: one
  // card when a turn ends for the changes it staged (reviewStagedChanges).
  // The sheet's header names the chat, so it's clear which one you're
  // typing into.
  import { onMount, untrack } from 'svelte'
  import { X } from 'lucide-svelte'
  import type { UncliChat } from '@uncli/chat/element'
  import { aiModeOf, aiModeTurnContext, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import { chatHost, chatMeta, followChatSessions, modeControl, nextTurnNote, openChatSession, reviewStagedChanges, runsOnFor, uncliSrc, UNCLI_LINK_SCHEMES, type ChatRef, type ChatSessions, type StagedHistory } from '@shared/uncliChat'
  import type { LLMConfig, SearchResult } from '@shared/types'
  import { machineRPC, readActiveRepoID, readEnrolment, repoRPC } from '../../lib/auth'
  import { saveAIMode } from '../../lib/aiModeSave'
  import { replace } from '../../lib/router.svelte'
  import { followBruv } from '../../lib/mentions.svelte'
  import { theme } from '../../lib/theme.svelte'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheetShell from './ChatSheetShell.svelte'

  let {
    title,
    chat,
    sessions,
    context,
    history,
    onClose,
  }: {
    /** What the chat is about (the card's or project's name). */
    title: string
    /** Which chat this is. */
    chat: ChatRef
    /** Where the chat's session is recorded (Get/Set…UncliSession). */
    sessions: Omit<ChatSessions, 'meta'>
    /** Told to the model with each turn, with the chat mode. */
    context: () => string | null
    /** The chat's history, where Suggest mode stages its changes, and
     *  Apply(Project)PendingEdits for the turn's review. */
    history: StagedHistory
    onClose: () => void
  } = $props()

  let src = $state('')
  let token = ''
  let error = $state('')
  // A failure that leaves the chat usable (the mode, a session).
  let notice = $state('')
  let el = $state<UncliChat | null>(null)
  let mode = $state<AIMode>('edit')
  // The settings' CLI and model choice for new chats (runsOnFor).
  let config: LLMConfig | null = null
  // How the last review went, told to the model with its next turn.
  const reviewed = nextTurnNote()
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
        config = cfg
        mode = aiModeOf(cfg?.ai_mode)
        src = uncliSrc(enrolment.serverURL)
      })
      .catch(e => { error = t('uncli.load_failed', { detail: errorDetail(e) }) })
  })

  function follow(target: string) {
    navigatedAway = true
    onClose()
    followBruv(target, replace)
  }

  async function changeMode(next: AIMode) {
    const previous = mode
    mode = next
    try {
      await saveAIMode(next)
    } catch (e) {
      mode = previous
      notice = t('chat.mode_save_failed', { detail: errorDetail(e) })
    }
  }

  // The host, again whenever the mode changes (UNCLI shows a control's new
  // value when the host is set again).
  $effect(() => {
    const target = el
    const current = mode
    if (!target) return
    untrack(() => {
      target.host = chatHost({
        token: () => token,
        follow,
        searchCards: q => repoRPC<SearchResult[]>('SearchCards', [q, 8]),
        context: () => [context(), aiModeTurnContext(current), reviewed.take()].filter(Boolean).join('\n'),
        cardLabel: t('uncli.mention_card'),
        enter: 'newline', // as ChatSheet: Enter a new line, Send (or Ctrl+Enter) sends
        controls: [modeControl(current, t, m => void changeMode(m))],
      })
    })
  })

  // The chat's session, kept right as the element starts, deletes or
  // loses sessions.
  $effect(() => {
    const target = el
    if (!target) return
    return untrack(() => {
      const s: ChatSessions = { ...sessions, meta: chatMeta(readActiveRepoID() ?? '', chat), runsOn: runsOnFor(config, chat) }
      openChatSession(target, s).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
      return followChatSessions(target, s, e => {
        notice = t('uncli.session_failed', { detail: errorDetail(e) })
      })
    })
  })

  // Suggest mode: one review card when a turn ends, for what it staged.
  $effect(() => {
    const target = el
    if (!target) return
    return untrack(() =>
      reviewStagedChanges(
        target,
        history,
        { title: n => t('uncli.review_title', { count: n }), detail: t('uncli.review_detail') },
        {
          done: ({ failed, note }) => {
            reviewed.set(note)
            if (failed) notice = t('uncli.review_some_failed', { count: failed })
          },
          error: e => {
            notice = t('uncli.review_failed', { detail: errorDetail(e) })
          },
        },
      ),
    )
  })

  // Escape closes the sheet, unless it's for the chat itself (its own
  // menus and dialogs, inside its shadow root).
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape' || (el && e.composedPath().includes(el))) return
    e.preventDefault()
    e.stopPropagation()
    onClose()
  }
</script>

<ChatSheetShell {title} note={t('uncli.engine_badge')} {onClose} {onKeydown} navigatingAway={() => navigatedAway}>
  {#if notice}
    <div class="notice" role="alert">
      <span>{notice}</span>
      <button type="button" class="notice-dismiss" onclick={() => (notice = '')} aria-label={t('toast.dismiss')}><X size={14} /></button>
    </div>
  {/if}
  {#if error}
    <p class="status error" role="alert">{error}</p>
  {:else if src}
    <!-- The host is set as it mounts; the element connects a moment after
         it has a src, so its first request carries BRUV's sign-in. Slotted
         children stay in BRUV's DOM and styles. -->
    <uncli-chat bind:this={el} {src} scheme={theme.current === 'auto' ? undefined : theme.current} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-chat"></uncli-chat>
  {:else}
    <p class="status">{t('chat.loading')}</p>
  {/if}
</ChatSheetShell>

<style>
  uncli-chat {
    display: block;
    flex: 1;
    min-height: 0;
    min-width: 0;
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
