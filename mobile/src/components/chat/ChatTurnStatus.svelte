<script lang="ts">
  // The reply-in-progress bubble: elapsed time, output so far, a warning
  // once the turn passes a warn budget, and Stop. Mirrors the desktop's
  // ChatTurnStatus; state comes from the shared ChatTurn.
  import { Square, TriangleAlert } from 'lucide-svelte'
  import { formatElapsed, type ChatTurn } from '@shared/chatTurn.svelte'
  import { t } from '../../lib/i18n.svelte'

  let { turn, onStop }: { turn: ChatTurn; onStop: () => void } = $props()
</script>

<div class="turn-status" role="status" aria-label={t('chat.thinking')}>
  <div class="turn-line">
    <span class="dots" aria-hidden="true"><span class="dot"></span><span class="dot"></span><span class="dot"></span></span>
    <span class="turn-meta">
      {formatElapsed(turn.elapsedMs)}
      {#if turn.used > 0}· {t('chat.turn_tokens', { count: turn.used.toLocaleString() })}{/if}
    </span>
    <button type="button" class="turn-stop" onclick={onStop} disabled={turn.stopping} aria-label={t('chat.stop_hint')}>
      <Square size={11} />
      {turn.stopping ? t('chat.stopping') : t('chat.stop')}
    </button>
  </div>
  {#if turn.overBudget}
    <div class="turn-over">
      <TriangleAlert size={13} />
      {t('chat.turn_over_budget', { budget: turn.budget.toLocaleString() })}
    </div>
  {/if}
</div>

<style>
  .turn-status {
    align-self: flex-start;
    max-width: 92%;
    padding: 8px 12px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
    border-bottom-left-radius: 4px;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .turn-line {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .dots {
    display: inline-flex;
    gap: 4px;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: bounce 1.2s infinite;
  }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }

  @keyframes bounce {
    0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
    30% { transform: translateY(-4px); opacity: 1; }
  }

  .turn-meta {
    font-variant-numeric: tabular-nums;
  }
  /* A comfortable touch target that doesn't dominate the bubble. */
  .turn-stop {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    min-height: 36px;
    margin-left: auto;
    padding: 0 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .turn-stop:active:not(:disabled) {
    border-color: var(--danger-border);
  }
  .turn-stop:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .turn-over {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    margin-top: 6px;
    color: var(--warn-border);
  }
  .turn-over :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
  }
</style>
