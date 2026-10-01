<script lang="ts">
  // A chat turn's notice (cut off, refused, stopped, over budget): the
  // whole of a system message, or a footer under a reply's text. Mirrors
  // the desktop's ChatNoticeLine.
  import { Info, TriangleAlert } from 'lucide-svelte'
  import { describeNotice, isWarningNotice } from '@shared/chatTurn.svelte'
  import type { ChatNotice } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'

  let { notice, footer = false }: { notice: ChatNotice; footer?: boolean } = $props()
  const warning = $derived(isWarningNotice(notice))
</script>

<div class="notice" class:footer class:warning>
  {#if warning}<TriangleAlert size={13} />{:else}<Info size={13} />{/if}
  <span>{describeNotice(notice, t)}</span>
</div>

<style>
  .notice {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 0.8rem;
    font-style: normal;
    color: var(--text-muted);
  }
  .notice :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
  }
  .warning {
    color: var(--warn-border);
  }
  .footer {
    margin-top: 6px;
    padding-top: 6px;
    border-top: 1px solid var(--border);
  }
</style>
