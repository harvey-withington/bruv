<script lang="ts">
  import { BotMessageSquare } from 'lucide-svelte'
  import { cardTurnContext, projectTurnContext, readChatEngine, type ChatEngineChoice, type ChatRef, type ChatSessions, type StagedHistory } from '@shared/uncliChat'
  import type { LLMConfig, UncliStatus } from '@shared/types'
  import { machineRPC, repoRPC } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheet from './ChatSheet.svelte'
  import UncliChatSheet from './UncliChatSheet.svelte'
  import type { ChatScope } from './scope'
  import type { ChatFile } from './types'

  // Topbar icon button that opens the AI chat sheet (replaces the old
  // floating action button). BotMessageSquare matches desktop's chat
  // iconography. Scope determines which RPCs the sheet calls — see
  // ChatScope.
  //
  // `open` is bindable so pages with their own overlay layering (e.g.
  // CardPage's Back/Escape guards) can see whether the sheet is up.
  //
  // The chat runs on the engine the user chose (desktop Settings → AI),
  // read as it opens: UNCLI when opted in and the server can run it, else
  // the built-in chat, saying why if they opted in.

  let {
    scope,
    open = $bindable(false),
    card = () => null,
    projectName = '',
  }: {
    scope: ChatScope
    open?: boolean
    /** The open card (card scope), for the UNCLI chat's title and context. */
    card?: () => { id: string; title: string; type: string } | null
    /** The project's name (project scope); its slug until loaded. */
    projectName?: string
  } = $props()

  let engine = $state<ChatEngineChoice>({ engine: 'builtin', fallback: null })

  async function openChat() {
    engine = await readChatEngine(
      () => machineRPC<LLMConfig>('GetLLMConfig'),
      () => machineRPC<UncliStatus>('GetUncliStatus'),
    ).catch((): ChatEngineChoice => ({ engine: 'builtin', fallback: null }))
    open = true
  }

  // The UNCLI chat's title, recorded session, turn context and history
  // (where Suggest mode stages its changes), per scope.
  type UncliChatProps = {
    title: string
    chat: ChatRef
    sessions: Omit<ChatSessions, 'meta'>
    context: () => string | null
    history: StagedHistory
  }
  const uncliChat = $derived.by((): UncliChatProps => {
    const s = scope
    if (s.kind === 'card') {
      return {
        title: card()?.title || t('card.untitled'),
        chat: { kind: 'card', cardId: s.cardID },
        sessions: {
          get: () => repoRPC<string>('GetCardUncliSession', [s.cardID]),
          set: session => repoRPC<void>('SetCardUncliSession', [s.cardID, session]),
          count: () => repoRPC<number>('GetCardUncliSessionCount', [s.cardID]),
          name: () => card()?.title || t('card.untitled'),
        },
        context: () => {
          const c = card()
          return c ? cardTurnContext(c) : null
        },
        history: {
          load: () => repoRPC<ChatFile>('LoadChatHistory', [s.cardID]),
          apply: (messageId, acceptIDs) => repoRPC<ChatFile>('ApplyPendingEdits', [s.cardID, messageId, acceptIDs]),
        },
      }
    }
    const name = projectName || s.project
    return {
      title: name,
      chat: { kind: 'project', brand: s.brand, stream: s.stream, project: s.project },
      sessions: {
        get: () => repoRPC<string>('GetProjectUncliSession', [s.brand, s.stream, s.project]),
        set: session => repoRPC<void>('SetProjectUncliSession', [s.brand, s.stream, s.project, session]),
        count: () => repoRPC<number>('GetProjectUncliSessionCount', [s.brand, s.stream, s.project]),
        name: () => name,
      },
      context: () => projectTurnContext({ brand: s.brand, stream: s.stream, project: name }),
      history: {
        load: () => repoRPC<ChatFile>('LoadProjectChatHistory', [s.brand, s.stream, s.project]),
        apply: (messageId, acceptIDs) => repoRPC<ChatFile>('ApplyProjectPendingEdits', [s.brand, s.stream, s.project, messageId, acceptIDs]),
      },
    }
  })
</script>

<button
  type="button"
  class="icon-btn"
  onclick={openChat}
  aria-label={t('chat.button_label')}
  title={t('chat.button_label')}
>
  <BotMessageSquare size={18} />
</button>

{#if open}
  {#if engine.engine === 'uncli'}
    <UncliChatSheet {...uncliChat} onClose={() => (open = false)} />
  {:else}
    <ChatSheet
      {scope}
      onClose={() => (open = false)}
      engineFallback={engine.engine === 'builtin' && engine.fallback ? t(engine.fallback.key, engine.fallback.params) : ''}
    />
  {/if}
{/if}

<style>
  /* Matches the topbar icon-button pattern (Search / Bell / Settings). */
  .icon-btn {
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

  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
</style>
