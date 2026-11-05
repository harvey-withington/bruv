<script lang="ts">
  // A chat on the UNCLI engine (opt-in, Settings → AI): UNCLI's
  // <uncli-chat> in a card's chat panel or the project side panel,
  // showing the chat's session (shared/uncliChat). The element shows
  // UNCLI's own download and sign-in steps until its CLI is ready,
  // approvals and its own header (New chat, delete, options). BRUV
  // supplies its sign-in, @mentions of cards, bruv: links, what the chat
  // is about as each turn's context, the Chat / Suggest / Edit control in
  // the message box, and Suggest's review: one card when a turn ends for
  // the changes it staged (reviewStagedChanges).
  import { untrack } from 'svelte'
  import type { UncliChat } from '@uncli/chat/element'
  import { GetLLMConfig, SearchCards } from '@shared/api'
  import { httpBase, resolveTransportInfo } from '@shared/adapters/cloud'
  import { aiModeOf, aiModeTurnContext, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import { chatHost, chatMeta, followChatSessions, modeControl, nextTurnNote, openChatSession, reviewStagedChanges, uncliSrc, UNCLI_LINK_SCHEMES, type ChatRef, type ChatSessions, type StagedHistory } from '@shared/uncliChat'
  import ChatPanelShell from './ChatPanelShell.svelte'
  import { saveAIMode } from '../lib/aiModeSave'
  import { followBruv } from '../lib/bruvLinks'
  import { showToast } from '../lib/toast.svelte'
  import { theme } from '../lib/theme.svelte'
  import { t } from '../lib/i18n.svelte'

  let {
    chat,
    sessions,
    context,
    history,
    onApplied,
    visible,
    hosted = false,
    mainWidth = $bindable(720),
    splitterDragging = $bindable(false),
  }: {
    /** Which chat this is; a different one reopens. */
    chat: ChatRef
    /** Where the chat's session is recorded (Get/Set…UncliSession). */
    sessions: Omit<ChatSessions, 'meta'>
    /** Told to the model with each turn, with the chat mode. */
    context: () => string | null
    /** The chat's history, where Suggest mode stages its changes, and
     *  Apply(Project)PendingEdits for the turn's review. */
    history: StagedHistory
    /** Reviewed changes were applied: the card or board may need reloading. */
    onApplied?: () => void
    visible: boolean
    /** In the project SidePanel, which owns geometry; a card's chat sits
     *  under CardDetail's close button instead. */
    hosted?: boolean
    mainWidth?: number
    splitterDragging?: boolean
  } = $props()

  let src = $state('')
  let token = ''
  let repoId = ''
  let error = $state('')
  let el = $state<UncliChat | null>(null)
  let mode = $state<AIMode>('edit')
  // How the last review went, told to the model with its next turn.
  const reviewed = nextTurnNote()

  const chatKey = $derived(JSON.stringify(chat))

  // The element (about 200 KB) and the server's address load when a
  // chat first opens, not with the app.
  async function prepare() {
    try {
      const [info, cfg] = await Promise.all([resolveTransportInfo(), GetLLMConfig(), import('@uncli/chat/element')])
      token = info.token
      repoId = info.repoID ?? ''
      mode = aiModeOf(cfg?.ai_mode)
      src = uncliSrc(httpBase(info))
    } catch (e) {
      error = t('uncli.load_failed', { detail: errorDetail(e) })
    }
  }
  $effect(() => {
    if (visible && !src && !error) untrack(() => void prepare())
  })

  function sessionsWithMeta(): ChatSessions {
    return { ...sessions, meta: chatMeta(repoId, chat) }
  }

  async function changeMode(next: AIMode) {
    const previous = mode
    mode = next
    try {
      await saveAIMode(next)
    } catch (e) {
      mode = previous
      showToast(t('chat.mode_save_failed', { detail: errorDetail(e) }), 'error')
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
        follow: followBruv,
        searchCards: q => SearchCards(q, 8),
        context: () => [context(), aiModeTurnContext(current), reviewed.take()].filter(Boolean).join('\n'),
        cardLabel: t('uncli.mention_card'),
        enter: 'send', // as ChatSection: Enter sends, Shift+Enter a new line
        controls: [modeControl(current, t, m => void changeMode(m))],
      })
    })
  })

  // The chat's session, again whenever the chat changes, and kept right
  // as the element starts, deletes or loses sessions.
  $effect(() => {
    const target = el
    void chatKey
    if (!target) return
    return untrack(() => {
      const s = sessionsWithMeta()
      const onError = (e: unknown) => showToast(t('uncli.session_failed', { detail: errorDetail(e) }), 'error')
      openChatSession(target, s).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
      return followChatSessions(target, s, onError)
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
          done: ({ applied, failed, note }) => {
            reviewed.set(note)
            if (applied) onApplied?.()
            if (failed) showToast(t('error.edit_apply_some_failed', { count: failed }), 'error')
          },
          error: e => showToast(t('uncli.review_failed', { detail: errorDetail(e) }), 'error'),
        },
      ),
    )
  })
</script>

<ChatPanelShell {visible} {hosted} bind:mainWidth bind:splitterDragging>
  {#if error}
    <p class="uncli-error" role="alert">{error}</p>
  {:else if src}
    <!-- The host is set as it mounts; the element connects a moment after
         it has a src, so its first request carries BRUV's sign-in. Slotted
         children stay in BRUV's DOM and styles. -->
    <uncli-chat bind:this={el} {src} scheme={theme.mode} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-chat">
      {#if !hosted}
        <!-- Room for CardDetail's close button, which sits over this corner,
             so the chat's header is the panel's only one. -->
        <span slot="header-end" class="close-room" aria-hidden="true"></span>
      {/if}
    </uncli-chat>
  {:else}
    <p class="uncli-loading">{t('chat.loading')}</p>
  {/if}
</ChatPanelShell>

<style>
  .close-room {
    display: inline-block;
    width: 2.25rem;
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
