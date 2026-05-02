<script lang="ts">
  import { MessageCircle } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import ChatSheet from './chat/ChatSheet.svelte'
  import type { ChatScope } from './chat/scope'

  // Floating action button that opens the AI chat sheet. Anchored to
  // the same safe-area corner as the capture FAB but offset to the
  // left so the two coexist. Scope determines which RPCs the sheet
  // calls — see ChatScope.

  let {
    scope,
    /** When true, the FAB sits in the safe-area corner (no capture FAB
     *  alongside). When false, it offsets to the left of the capture
     *  FAB's footprint. */
    solo = false,
  }: { scope: ChatScope; solo?: boolean } = $props()

  let open = $state(false)
</script>

<button
  type="button"
  class="fab"
  class:solo
  onclick={() => (open = true)}
  aria-label={t('chat.fab_label')}
  title={t('chat.fab_label')}
>
  <MessageCircle size={24} />
</button>

{#if open}
  <ChatSheet {scope} onClose={() => (open = false)} />
{/if}

<style>
  .fab {
    position: fixed;
    /* Sit immediately to the left of the capture FAB. The capture FAB
       is 56px wide + 1.25rem right inset; sit ~72px (56 + 16 gutter)
       to its left so the two touch-targets don't blur into one. */
    right: calc(1.25rem + 56px + 1rem + env(safe-area-inset-right));
    bottom: calc(1.25rem + env(safe-area-inset-bottom));
    width: 52px;
    height: 52px;
    border-radius: 50%;
    background: var(--bg-elev-1);
    color: var(--accent);
    border: 1px solid var(--border);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 6px 16px rgba(0, 0, 0, 0.3), 0 2px 4px rgba(0, 0, 0, 0.2);
    transition: transform 120ms ease, border-color 120ms ease, color 120ms ease;
    z-index: 50;
  }

  /* On Card page the capture FAB is hidden — chat sits at the corner. */
  .fab.solo {
    right: calc(1.25rem + env(safe-area-inset-right));
  }

  .fab:hover,
  .fab:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .fab:active {
    transform: scale(0.94);
  }
</style>
