<script lang="ts">
  import { BotMessageSquare } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheet from './ChatSheet.svelte'
  import type { ChatScope } from './scope'

  // Topbar icon button that opens the AI chat sheet (replaces the old
  // floating action button). BotMessageSquare matches desktop's chat
  // iconography. Scope determines which RPCs the sheet calls — see
  // ChatScope.
  //
  // `open` is bindable so pages with their own overlay layering (e.g.
  // CardPage's Back/Escape guards) can see whether the sheet is up.

  let {
    scope,
    open = $bindable(false),
  }: { scope: ChatScope; open?: boolean } = $props()
</script>

<button
  type="button"
  class="icon-btn"
  onclick={() => (open = true)}
  aria-label={t('chat.button_label')}
  title={t('chat.button_label')}
>
  <BotMessageSquare size={18} />
</button>

{#if open}
  <ChatSheet {scope} onClose={() => (open = false)} />
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
