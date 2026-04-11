<script lang="ts">
  import { GetAllAgents, TriggerAgent, CancelAgent, PauseAllAgents, ResumeAllAgents, GetAgentSchedulerStatus } from '../lib/api'
  import type { AgentSummary } from '../lib/types'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { Timer, Play, Square, Pause, CirclePlay, CircleCheck, CircleX, TriangleAlert, Clock, X } from 'lucide-svelte'
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn } from '../../wailsjs/runtime/runtime'

  let { onClose, onOpenCard }: {
    onClose: () => void
    onOpenCard: (cardId: string) => void
  } = $props()

  let agents = $state<AgentSummary[]>([])
  let loading = $state(true)
  let schedulerPaused = $state(false)

  async function load() {
    try {
      const [all, status] = await Promise.all([GetAllAgents(), GetAgentSchedulerStatus()])
      agents = (all || []).sort((a, b) => {
        // Running first, then enabled, then disabled
        if (a.is_running !== b.is_running) return a.is_running ? -1 : 1
        if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
        return a.card_title.localeCompare(b.card_title)
      })
      schedulerPaused = status.paused
    } catch (e) {
      console.error('Failed to load agents:', e)
    } finally {
      loading = false
    }
  }

  async function triggerAgent(cardId: string) {
    try {
      await TriggerAgent(cardId)
      showToast(t('agent.triggered'), 'success')
      setTimeout(load, 1500)
    } catch {
      showToast(t('agent.trigger_failed'), 'error')
    }
  }

  async function cancelAgent(cardId: string) {
    try {
      await CancelAgent(cardId)
      showToast(t('agent.cancelled'), 'info')
      setTimeout(load, 1000)
    } catch {
      showToast(t('agent.cancel_failed'), 'error')
    }
  }

  async function togglePause() {
    try {
      if (schedulerPaused) {
        await ResumeAllAgents()
        schedulerPaused = false
        showToast(t('dashboard.resumed'), 'success')
      } else {
        await PauseAllAgents()
        schedulerPaused = true
        showToast(t('dashboard.paused'), 'info')
      }
    } catch {
      showToast(t('dashboard.toggle_failed'), 'error')
    }
  }

  function statusColor(s: string): string {
    switch (s) {
      case 'idle': return 'var(--color-success, #22c55e)'
      case 'running': return 'var(--color-info, #3b82f6)'
      case 'failed': return 'var(--color-error, #ef4444)'
      default: return 'var(--color-muted, #94a3b8)'
    }
  }

  function runStatusIcon(s: string | null) {
    switch (s) {
      case 'success': return CircleCheck
      case 'failure': return CircleX
      case 'timeout': return TriangleAlert
      default: return Clock
    }
  }

  function formatRelative(iso: string | null): string {
    if (!iso) return '-'
    const d = new Date(iso)
    const diff = Date.now() - d.getTime()
    const mins = Math.floor(Math.abs(diff) / 60000)
    const future = diff < 0
    if (mins < 1) return future ? 'soon' : 'just now'
    if (mins < 60) return future ? `in ${mins}m` : `${mins}m ago`
    const hours = Math.floor(mins / 60)
    if (hours < 24) return future ? `in ${hours}h` : `${hours}h ago`
    const days = Math.floor(hours / 24)
    return future ? `in ${days}d` : `${days}d ago`
  }

  function truncate(s: string | null, len: number): string {
    if (!s) return ''
    return s.length > len ? s.slice(0, len) + '...' : s
  }

  // Stats
  let enabledCount = $derived(agents.filter(a => a.enabled).length)
  let runningCount = $derived(agents.filter(a => a.is_running).length)
  let failedCount = $derived(agents.filter(a => a.status === 'failed').length)

  // Event listeners for live updates
  let cleanupFns: (() => void)[] = []

  onMount(() => {
    load()
    cleanupFns = [
      EventsOn('agent:started', () => load()),
      EventsOn('agent:completed', () => load()),
      EventsOn('agent:failed', () => load()),
      EventsOn('scheduler:paused', (data: { paused: boolean }) => { schedulerPaused = data.paused }),
    ]
  })

  onDestroy(() => {
    for (const fn of cleanupFns) { if (typeof fn === 'function') fn() }
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="dashboard-overlay" onclick={onClose} onkeydown={(e) => e.key === 'Escape' && onClose()}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="dashboard-panel" onclick={(e) => e.stopPropagation()}>
    <div class="dashboard-header">
      <div class="header-left">
        <Timer size={18} />
        <h2>{t('dashboard.title')}</h2>
      </div>
      <div class="header-stats">
        <span class="stat">{enabledCount} {t('dashboard.enabled')}</span>
        {#if runningCount > 0}
          <span class="stat stat-running">{runningCount} {t('dashboard.running')}</span>
        {/if}
        {#if failedCount > 0}
          <span class="stat stat-failed">{failedCount} {t('dashboard.failed')}</span>
        {/if}
      </div>
      <div class="header-actions">
        <button class="header-btn" onclick={togglePause} title={schedulerPaused ? t('dashboard.resume') : t('dashboard.pause')}>
          {#if schedulerPaused}
            <CirclePlay size={14} />
            {t('dashboard.resume')}
          {:else}
            <Pause size={14} />
            {t('dashboard.pause')}
          {/if}
        </button>
        <button class="close-btn" onclick={onClose}>
          <X size={16} />
        </button>
      </div>
    </div>

    {#if loading}
      <div class="dashboard-loading">{t('app.loading')}</div>
    {:else if agents.length === 0}
      <div class="dashboard-empty">
        <Timer size={32} strokeWidth={1} />
        <p>{t('dashboard.empty')}</p>
      </div>
    {:else}
      <div class="agent-table">
        <div class="table-header">
          <span class="col-status">{t('agent.status')}</span>
          <span class="col-title">{t('dashboard.col_card')}</span>
          <span class="col-schedule">{t('agent.schedule')}</span>
          <span class="col-last-run">{t('dashboard.col_last_run')}</span>
          <span class="col-next-run">{t('dashboard.col_next_run')}</span>
          <span class="col-tokens">{t('agent.run_tokens')}</span>
          <span class="col-actions"></span>
        </div>
        {#each agents as agent}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            class="table-row"
            class:disabled={!agent.enabled}
            class:running={agent.is_running}
            onclick={() => onOpenCard(agent.card_id)}
          >
            <span class="col-status">
              <span class="status-dot" style="background: {statusColor(agent.enabled ? agent.status : 'disabled')}"></span>
              <span class="status-text">{t(`agent.status_${agent.enabled ? agent.status : 'disabled'}`)}</span>
            </span>
            <span class="col-title">
              <span class="agent-title">{agent.card_title}</span>
              {#if agent.goal}
                <span class="agent-goal">{truncate(agent.goal, 60)}</span>
              {/if}
            </span>
            <span class="col-schedule">{agent.schedule || '-'}</span>
            <span class="col-last-run">
              {#if agent.last_run_status}
                <span class="run-status-icon" style="color: {statusColor(agent.last_run_status === 'success' ? 'idle' : agent.last_run_status === 'failure' ? 'failed' : 'disabled')}">
                  {#each [runStatusIcon(agent.last_run_status)] as RunIcon}
                    <RunIcon size={12} />
                  {/each}
                </span>
              {/if}
              <span>{formatRelative(agent.last_run_at)}</span>
            </span>
            <span class="col-next-run">{formatRelative(agent.next_run_at)}</span>
            <span class="col-tokens">{agent.last_run_tokens ? agent.last_run_tokens.toLocaleString() : '-'}</span>
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <span class="col-actions" onclick={(e) => e.stopPropagation()}>
              {#if agent.is_running}
                <button class="action-btn action-cancel" onclick={() => cancelAgent(agent.card_id)} title={t('agent.cancel')}>
                  <Square size={12} />
                </button>
              {:else if agent.enabled}
                <button class="action-btn action-run" onclick={() => triggerAgent(agent.card_id)} title={t('agent.run_now')}>
                  <Play size={12} />
                </button>
              {/if}
            </span>
          </div>
          {#if agent.last_run_error}
            <div class="row-error">{truncate(agent.last_run_error, 120)}</div>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .dashboard-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 900;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .dashboard-panel {
    background: var(--bg-base);
    border: 1px solid var(--border-muted);
    border-radius: 10px;
    width: min(900px, 90vw);
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
  }

  .dashboard-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .header-left {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--text-strong);
  }
  .header-left h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
  }
  .header-stats {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-left: auto;
  }
  .stat {
    font-size: 0.7rem;
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    color: var(--text-muted);
  }
  .stat-running {
    background: color-mix(in srgb, var(--color-info, #3b82f6) 15%, var(--bg-elevated));
    color: var(--color-info, #3b82f6);
    border-color: var(--color-info, #3b82f6);
  }
  .stat-failed {
    background: color-mix(in srgb, var(--color-error, #ef4444) 15%, var(--bg-elevated));
    color: var(--color-error, #ef4444);
    border-color: var(--color-error, #ef4444);
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .header-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    color: var(--text-body);
    font-size: 0.72rem;
    cursor: pointer;
    transition: border-color 0.15s;
  }
  .header-btn:hover {
    border-color: var(--accent);
  }
  .close-btn {
    display: flex;
    align-items: center;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    transition: color 0.15s, background 0.15s;
  }
  .close-btn:hover {
    color: var(--text-strong);
    background: var(--bg-subtle-hover);
  }

  .dashboard-loading, .dashboard-empty {
    padding: 3rem;
    text-align: center;
    color: var(--text-muted);
  }
  .dashboard-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
  }

  /* Table */
  .agent-table {
    overflow-y: auto;
    flex: 1;
  }
  .table-header {
    display: grid;
    grid-template-columns: 100px 1fr 90px 100px 90px 80px 50px;
    gap: 0.5rem;
    padding: 0.4rem 1rem;
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    border-bottom: 1px solid var(--border-muted);
    position: sticky;
    top: 0;
    background: var(--bg-base);
    z-index: 1;
  }

  .table-row {
    display: grid;
    grid-template-columns: 100px 1fr 90px 100px 90px 80px 50px;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    background: none;
    color: var(--text-body);
    font-size: 0.78rem;
    text-align: left;
    cursor: pointer;
    transition: background 0.1s;
    width: 100%;
    align-items: center;
  }
  .table-row:hover {
    background: var(--bg-subtle-hover);
  }
  .table-row.disabled {
    opacity: 0.5;
  }

  .col-status {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .status-text {
    font-size: 0.7rem;
    text-transform: capitalize;
  }

  .col-title {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }
  .agent-title {
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .agent-goal {
    font-size: 0.68rem;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .col-schedule {
    font-size: 0.72rem;
    color: var(--text-muted);
    font-family: monospace;
  }

  .col-last-run, .col-next-run {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.72rem;
    color: var(--text-muted);
  }

  .run-status-icon {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .col-tokens {
    font-size: 0.72rem;
    color: var(--text-muted);
    font-family: monospace;
  }

  .col-actions {
    display: flex;
    justify-content: flex-end;
  }

  .action-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 4px;
    cursor: pointer;
    transition: filter 0.1s;
  }
  .action-run {
    background: var(--accent);
    color: white;
  }
  .action-cancel {
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    color: white;
    box-shadow: 0 0 6px rgba(99, 102, 241, 0.5), 0 0 12px rgba(168, 85, 247, 0.3);
  }
  .action-btn:hover {
    filter: brightness(1.15);
  }

  /* Neon glow on running row status dot */
  .table-row.running .status-dot {
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1) !important;
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    box-shadow: 0 0 4px rgba(99, 102, 241, 0.6), 0 0 8px rgba(168, 85, 247, 0.3);
  }
  @keyframes agent-neon {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
  }

  .row-error {
    padding: 0.15rem 1rem 0.4rem 1rem;
    font-size: 0.68rem;
    color: var(--color-error, #ef4444);
    border-bottom: 1px solid var(--border-muted);
    padding-left: calc(1rem + 100px + 0.5rem);
  }
</style>
