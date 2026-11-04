<script lang="ts">
  import { BotMessageSquare } from 'lucide-svelte'
  import { readCardChatEngine, type CardChatEngine } from '@shared/uncliChat'
  import type { LLMConfig, UncliStatus } from '@shared/types'
  import { machineRPC } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheet from './ChatSheet.svelte'
  import UncliChatSheet from './UncliChatSheet.svelte'
  import type { ChatScope } from './scope'

  // Topbar icon button that opens the AI chat sheet (replaces the old
  // floating action button). BotMessageSquare matches desktop's chat
  // iconography. Scope determines which RPCs the sheet calls — see
  // ChatScope.
  //
  // `open` is bindable so pages with their own overlay layering (e.g.
  // CardPage's Back/Escape guards) can see whether the sheet is up.
  //
  // A card's chat runs on the engine the user chose (desktop Settings →
  // AI), read as it opens: UNCLI when opted in and the server can run it,
  // else the built-in chat, saying why if they opted in.

  let {
    scope,
    open = $bindable(false),
    card = () => null,
  }: {
    scope: ChatScope
    open?: boolean
    /** The open card, for the UNCLI chat's turn context. */
    card?: () => { id: string; title: string; type: string } | null
  } = $props()

  let engine = $state<CardChatEngine>({ engine: 'builtin', fallback: null })

  async function openChat() {
    if (scope.kind === 'card') {
      engine = await readCardChatEngine(
        () => machineRPC<LLMConfig>('GetLLMConfig'),
        () => machineRPC<UncliStatus>('GetUncliStatus'),
      ).catch((): CardChatEngine => ({ engine: 'builtin', fallback: null }))
    }
    open = true
  }
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
  {#if scope.kind === 'card' && engine.engine === 'uncli'}
    <UncliChatSheet cardID={scope.cardID} {card} onClose={() => (open = false)} />
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
