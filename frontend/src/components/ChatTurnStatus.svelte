<script lang="ts">
  // The reply-in-progress bubble: elapsed time, output so far, a warning
  // once the turn passes a warn budget, and Stop.
  import { Square, TriangleAlert } from 'lucide-svelte'
  import { formatElapsed, type ChatTurn } from '@shared/chatTurn.svelte'
  import { t } from '../lib/i18n.svelte'

  let { turn, onStop }: { turn: ChatTurn; onStop: () => void } = $props()
</script>

<div class="turn-status" role="status" aria-label={t('chat.thinking')}>
  <div class="turn-line">
    <span class="dots" aria-hidden="true"><span class="dot"></span><span class="dot"></span><span class="dot"></span></span>
    <span class="turn-meta">
      {formatElapsed(turn.elapsedMs)}
      {#if turn.used > 0}· {t('chat.turn_tokens', { count: turn.used.toLocaleString() })}{/if}
    </span>
    <button class="turn-stop" onclick={onStop} disabled={turn.stopping} title={t('chat.stop_hint')}>
      <Square size={9} />
      {turn.stopping ? t('chat.stopping') : t('chat.stop')}
    </button>
  </div>
  {#if turn.overBudget}
    <div class="turn-over">
      <TriangleAlert size={11} />
      {t('chat.turn_over_budget', { budget: turn.budget.toLocaleString() })}
    </div>
  {/if}
</div>

<style>
  .turn-status {
    align-self: flex-start;
    max-width: 85%;
    padding: 8px 12px;
    border-radius: 10px;
    background: var(--bg-elevated);
    font-size: 0.75rem;
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
  .turn-stop {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: auto;
    padding: 2px 8px;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: none;
    color: var(--text-secondary);
    font-size: 0.7rem;
    cursor: pointer;
  }
  .turn-stop:hover:not(:disabled) {
    border-color: var(--danger-border);
    color: var(--danger-light);
  }
  .turn-stop:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .turn-over {
    display: flex;
    align-items: center;
    gap: 5px;
    margin-top: 6px;
    color: var(--warning);
  }
</style>
