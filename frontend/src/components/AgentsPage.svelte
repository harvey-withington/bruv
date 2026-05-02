<script lang="ts">
  import { GetAllAgents, GetAllAgentRuns, GetAgentAnalytics, TriggerAgent, CancelAgent, PauseAllAgents, ResumeAllAgents, GetAgentSchedulerStatus } from '@shared/api'
  import type { AgentSummary, AgentRunEntry, AgentAnalytics } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { Timer, Play, Square, Pause, CirclePlay, CircleCheck, CircleX, TriangleAlert, Clock } from 'lucide-svelte'
  import { onMount, onDestroy } from 'svelte'
  import { onEvent } from '../lib/events'

  let { onCardClick }: { onCardClick: (cardId: string) => void } = $props()

  let agents = $state<AgentSummary[]>([])
  let runs = $state<AgentRunEntry[]>([])
  let analytics = $state<AgentAnalytics>({ total_agents: 0, enabled_agents: 0, total_runs: 0, success_runs: 0, failed_runs: 0, total_tokens: 0, total_cost: 0, cost_today: 0, cost_7d: 0, cost_by_model: {} })
  let schedulerPaused = $state(false)
  let loading = $state(true)
  let activeSection = $state<'overview' | 'history'>('overview')

  // Splitter: top pane (overview table) vs bottom pane (analytics)
  const ANALYTICS_HEIGHT_KEY = 'bruv:analyticsHeight'
  const MIN_PANE = 120
  let analyticsHeight = $state(Number(localStorage.getItem(ANALYTICS_HEIGHT_KEY)) || 260)
  let splitterDragging = $state(false)

  // Typed as MouseEvent because Svelte 5's `onpointerdown` on HTMLElement
  // is typed as MouseEventHandler; PointerEvent extends MouseEvent so
  // we can narrow for pointer-specific fields below. Pointer events give
  // us touch/stylus support for free on mobile/tablet viewports.
  function onAnalyticsSplitterDown(e: MouseEvent) {
    e.preventDefault()
    splitterDragging = true
    const startY = e.clientY
    const startH = analyticsHeight
    const pe = e as PointerEvent
    const target = e.currentTarget as HTMLElement | null
    const pointerId = pe.pointerId
    target?.setPointerCapture?.(pointerId)

    function onMove(ev: PointerEvent) {
      const delta = startY - ev.clientY
      analyticsHeight = Math.max(MIN_PANE, startH + delta)
    }
    function onUp() {
      splitterDragging = false
      localStorage.setItem(ANALYTICS_HEIGHT_KEY, String(analyticsHeight))
      try { target?.releasePointerCapture?.(pointerId) } catch { /* already released */ }
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }

  async function load() {
    try {
      const [allAgents, allRuns, stats, status] = await Promise.all([
        GetAllAgents(),
        GetAllAgentRuns(100),
        GetAgentAnalytics(),
        GetAgentSchedulerStatus(),
      ])
      agents = (allAgents || []).sort((a, b) => {
        if (a.is_running !== b.is_running) return a.is_running ? -1 : 1
        if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
        return a.card_title.localeCompare(b.card_title)
      })
      runs = allRuns || []
      analytics = stats
      schedulerPaused = status.paused
    } catch (e) {
      console.error('Failed to load agents page:', e)
    } finally {
      loading = false
    }
  }

  async function triggerAgent(cardId: string) {
    try {
      await TriggerAgent(cardId)
      showToast(t('agent.triggered'), 'success')
      setTimeout(load, 1500)
    } catch { showToast(t('agent.trigger_failed'), 'error') }
  }

  async function cancelAgent(cardId: string) {
    try {
      await CancelAgent(cardId)
      showToast(t('agent.cancelled'), 'info')
      setTimeout(load, 1000)
    } catch { showToast(t('agent.cancel_failed'), 'error') }
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
    } catch { showToast(t('dashboard.toggle_failed'), 'error') }
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

  function formatTokens(n: number): string {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
    if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
    return String(n)
  }

  // Per-agent token breakdown
  let tokensByAgent = $derived(() => {
    const map = new Map<string, { title: string; tokens: number; runs: number }>()
    for (const r of runs) {
      const existing = map.get(r.card_id) || { title: r.card_title, tokens: 0, runs: 0 }
      existing.tokens += r.tokens_used || 0
      existing.runs++
      map.set(r.card_id, existing)
    }
    return [...map.entries()]
      .map(([id, data]) => ({ id, ...data }))
      .sort((a, b) => b.tokens - a.tokens)
  })

  // Daily run counts (last 14 days)
  let dailyRuns = $derived(() => {
    const days: { date: string; success: number; failed: number; tokens: number }[] = []
    const now = new Date()
    for (let i = 13; i >= 0; i--) {
      const d = new Date(now)
      d.setDate(d.getDate() - i)
      const dateStr = d.toISOString().slice(0, 10)
      days.push({ date: dateStr, success: 0, failed: 0, tokens: 0 })
    }
    for (const r of runs) {
      const dateStr = r.started_at.slice(0, 10)
      const day = days.find(d => d.date === dateStr)
      if (day) {
        if (r.status === 'success') day.success++
        else if (r.status === 'failure') day.failed++
        day.tokens += r.tokens_used || 0
      }
    }
    return days
  })

  let maxDailyRuns = $derived(Math.max(1, ...dailyRuns().map(d => d.success + d.failed)))
  let maxDailyTokens = $derived(Math.max(1, ...dailyRuns().map(d => d.tokens)))

  let runningCount = $derived(agents.filter(a => a.is_running).length)
  let failedCount = $derived(agents.filter(a => a.status === 'failed').length)
  let successRate = $derived(analytics.total_runs > 0 ? Math.round((analytics.success_runs / analytics.total_runs) * 100) : 0)

  let cleanupFns: (() => void)[] = []

  onMount(() => {
    load()
    cleanupFns = [
      onEvent('agent:started', () => load()),
      onEvent('agent:completed', () => load()),
      onEvent('agent:failed', () => load()),
      onEvent<{ paused: boolean }>('scheduler:paused', (data) => { schedulerPaused = data.paused }),
    ]
  })

  onDestroy(() => {
    for (const fn of cleanupFns) { if (typeof fn === 'function') fn() }
  })
</script>

<div class="agents-page">
  {#if loading}
    <div class="page-loading">
      <Timer size={24} strokeWidth={1.5} />
      <span>{t('app.loading')}</span>
    </div>
  {:else}
    <!-- Stats bar -->
    <div class="stats-bar">
      <div class="stat-card">
        <div class="stat-value">{analytics.enabled_agents}<span class="stat-secondary">/ {analytics.total_agents}</span></div>
        <div class="stat-label">{t('agents_page.stat_active')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{analytics.total_runs}</div>
        <div class="stat-label">{t('agents_page.stat_runs')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{successRate}<span class="stat-unit">%</span></div>
        <div class="stat-label">{t('agents_page.stat_success_rate')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{formatTokens(analytics.total_tokens)}</div>
        <div class="stat-label">{t('agents_page.stat_tokens')}</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">${analytics.total_cost.toFixed(2)}</div>
        <div class="stat-label">{t('agents_page.stat_cost')}</div>
        <div class="stat-sub">{t('agents_page.cost_today')}: ${analytics.cost_today.toFixed(2)} | {t('agents_page.cost_7d')}: ${analytics.cost_7d.toFixed(2)}</div>
      </div>
    </div>

    <!-- Section tabs + controls -->
    <div class="section-tabs">
      <button class="section-tab" class:active={activeSection === 'overview'} onclick={() => activeSection = 'overview'}>
        <Timer size={14} /> {t('agents_page.tab_overview')}
      </button>
      <button class="section-tab" class:active={activeSection === 'history'} onclick={() => activeSection = 'history'}>
        <Clock size={14} /> {t('agents_page.tab_history')}
      </button>
      <div class="tab-bar-right">
        {#if runningCount > 0}
          <span class="live-badge">{runningCount} {t('dashboard.running')}</span>
        {/if}
        {#if failedCount > 0}
          <span class="failed-badge">{failedCount} {t('dashboard.failed')}</span>
        {/if}
        <button class="pause-btn" onclick={togglePause}>
          {#if schedulerPaused}
            <CirclePlay size={13} /> {t('dashboard.resume')}
          {:else}
            <Pause size={13} /> {t('dashboard.pause')}
          {/if}
        </button>
      </div>
    </div>

    <!-- Content -->
    {#if activeSection === 'overview'}
      <div class="split-layout" class:dragging={splitterDragging}>
        <!-- Top pane: Agent fleet table -->
        <div class="split-top">
          {#if agents.length === 0}
            <div class="empty-state">
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
                  onclick={() => onCardClick(agent.card_id)}
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
                  <span class="col-schedule">
                  {agent.schedule || '-'}
                  {#if agent.one_shot}<span class="schedule-badge badge-oneshot">1x</span>{/if}
                  {#if agent.start_date || agent.end_date}<span class="schedule-badge badge-dated">&#128197;</span>{/if}
                </span>
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
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
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

        <!-- Horizontal splitter -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div class="h-splitter" role="separator" tabindex="-1" onpointerdown={onAnalyticsSplitterDown}></div>

        <!-- Bottom pane: Analytics -->
        <div class="split-bottom" style="height: {analyticsHeight}px;">
          <div class="analytics-grid">
            <!-- Daily runs chart -->
            <div class="chart-card">
              <h3 class="chart-title">{t('agents_page.chart_daily_runs')}</h3>
              <div class="bar-chart">
                {#each dailyRuns() as day}
                  <div class="bar-col" title="{day.date}: {day.success} ok, {day.failed} failed">
                    <div class="bar-stack">
                      {#if day.failed > 0}
                        <div class="bar bar-failed" style="height: {(day.failed / maxDailyRuns) * 100}%"></div>
                      {/if}
                      {#if day.success > 0}
                        <div class="bar bar-success" style="height: {(day.success / maxDailyRuns) * 100}%"></div>
                      {/if}
                    </div>
                    <span class="bar-label">{day.date.slice(8)}</span>
                  </div>
                {/each}
              </div>
              <div class="chart-legend">
                <span class="legend-item"><span class="legend-dot legend-success"></span> {t('agent.notify_success')}</span>
                <span class="legend-item"><span class="legend-dot legend-failed"></span> {t('agent.notify_failure')}</span>
              </div>
            </div>

            <!-- Daily token usage chart -->
            <div class="chart-card">
              <h3 class="chart-title">{t('agents_page.chart_daily_tokens')}</h3>
              <div class="bar-chart">
                {#each dailyRuns() as day}
                  <div class="bar-col" title="{day.date}: {day.tokens.toLocaleString()} tokens">
                    <div class="bar-stack">
                      {#if day.tokens > 0}
                        <div class="bar bar-tokens" style="height: {(day.tokens / maxDailyTokens) * 100}%"></div>
                      {/if}
                    </div>
                    <span class="bar-label">{day.date.slice(8)}</span>
                  </div>
                {/each}
              </div>
            </div>

            <!-- Per-agent token breakdown -->
            <div class="chart-card chart-card-wide">
              <h3 class="chart-title">{t('agents_page.chart_token_breakdown')}</h3>
              {#if tokensByAgent().length === 0}
                <p class="chart-empty">{t('agents_page.no_token_data')}</p>
              {:else}
                {@const maxTokens = Math.max(1, ...tokensByAgent().map(a => a.tokens))}
                <div class="breakdown-list">
                  {#each tokensByAgent() as agent}
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <div class="breakdown-row" onclick={() => onCardClick(agent.id)}>
                      <span class="breakdown-title">{agent.title}</span>
                      <span class="breakdown-runs">{agent.runs} runs</span>
                      <div class="breakdown-bar-wrap">
                        <div class="breakdown-bar" style="width: {(agent.tokens / maxTokens) * 100}%"></div>
                      </div>
                      <span class="breakdown-tokens">{formatTokens(agent.tokens)}</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>

            <!-- Cost by model breakdown -->
            <div class="chart-card chart-card-wide">
              <h3 class="chart-title">{t('agents_page.cost_by_model')}</h3>
              {#if Object.keys(analytics.cost_by_model).length === 0}
                <p class="chart-empty">{t('agents_page.no_token_data')}</p>
              {:else}
                {@const costEntries = Object.entries(analytics.cost_by_model).sort((a, b) => b[1] - a[1])}
                {@const maxCost = Math.max(1, ...costEntries.map(e => e[1]))}
                <div class="breakdown-list">
                  {#each costEntries as [model, cost]}
                    <div class="breakdown-row">
                      <span class="breakdown-title">{model}</span>
                      <div class="breakdown-bar-wrap">
                        <div class="breakdown-bar breakdown-bar-cost" style="width: {(cost / maxCost) * 100}%"></div>
                      </div>
                      <span class="breakdown-tokens">${cost.toFixed(4)}</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          </div>
        </div>
      </div>

    {:else if activeSection === 'history'}
      <div class="page-content">
        {#if runs.length === 0}
          <div class="empty-state">
            <Clock size={32} strokeWidth={1} />
            <p>{t('agents_page.no_runs')}</p>
          </div>
        {:else}
          <div class="run-history">
            <div class="run-header">
              <span class="rh-status"></span>
              <span class="rh-card">{t('dashboard.col_card')}</span>
              <span class="rh-time">{t('agents_page.col_time')}</span>
              <span class="rh-duration">{t('agents_page.col_duration')}</span>
              <span class="rh-tools">{t('agents_page.col_tools')}</span>
              <span class="rh-tokens">{t('agents_page.col_tokens')}</span>
              <span class="rh-cost">{t('agents_page.col_cost')}</span>
              <span class="rh-summary">{t('agent.run_summary')}</span>
            </div>
            {#each runs as run}
              <!-- svelte-ignore a11y_no_static_element_interactions -->
                <!-- svelte-ignore a11y_click_events_have_key_events -->
              <div class="run-row" onclick={() => onCardClick(run.card_id)}>
                <span class="rh-status run-status-icon" style="color: {statusColor(run.status === 'success' ? 'idle' : run.status === 'failure' ? 'failed' : 'disabled')}">
                  {#each [runStatusIcon(run.status)] as RunIcon}
                    <RunIcon size={13} />
                  {/each}
                </span>
                <span class="rh-card run-card-title">{run.card_title}</span>
                <span class="rh-time">{formatRelative(run.started_at)}</span>
                <span class="rh-duration">{run.duration_secs != null ? `${run.duration_secs}s` : '-'}</span>
                <span class="rh-tools">{run.tool_count}</span>
                <span class="rh-tokens">{run.tokens_used ? run.tokens_used.toLocaleString() : '-'}</span>
                <span class="rh-cost">${run.estimated_cost?.toFixed(4) || '0.0000'}{#if run.model_used} <span class="rh-model">{run.model_used}</span>{/if}</span>
                <span class="rh-summary">{run.error || run.summary || ''}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .agents-page {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    gap: 0.75rem;
  }

  .page-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    height: 100%;
    color: var(--text-muted);
  }

  /* Stats bar */
  .stats-bar {
    display: flex;
    gap: 0.6rem;
    flex-shrink: 0;
  }
  .stat-card {
    flex: 1;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--border-muted);
    border-radius: 8px;
    background: var(--bg-elevated);
  }
  .stat-value {
    font-size: 1.4rem;
    font-weight: 700;
    color: var(--text-strong);
    line-height: 1.2;
  }
  .stat-secondary {
    font-size: 0.85rem;
    font-weight: 400;
    color: var(--text-muted);
  }
  .stat-unit {
    font-size: 0.85rem;
    font-weight: 400;
    color: var(--text-muted);
  }
  .stat-label {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    margin-top: 0.15rem;
  }
  .stat-sub {
    font-size: 0.62rem;
    color: var(--text-muted);
    margin-top: 0.1rem;
  }
  .tab-bar-right {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-left: auto;
    padding-right: 0.25rem;
  }
  .live-badge {
    font-size: 0.68rem;
    padding: 0.1rem 0.4rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--color-info, #3b82f6) 15%, var(--bg-elevated));
    color: var(--color-info, #3b82f6);
    border: 1px solid var(--color-info, #3b82f6);
  }
  .failed-badge {
    font-size: 0.68rem;
    padding: 0.1rem 0.4rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--color-error, #ef4444) 15%, var(--bg-elevated));
    color: var(--color-error, #ef4444);
    border: 1px solid var(--color-error, #ef4444);
  }
  .pause-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    color: var(--text-body);
    font-size: 0.7rem;
    cursor: pointer;
    transition: border-color 0.15s;
  }
  .pause-btn:hover { border-color: var(--accent); }

  /* Section tabs */
  .section-tabs {
    display: flex;
    gap: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }
  .section-tab {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.5rem 0.75rem;
    border: none;
    background: none;
    color: var(--text-muted);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    transition: color 0.15s, border-color 0.15s;
  }
  .section-tab:hover { color: var(--text-body); }
  .section-tab.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  /* Split layout (overview + analytics) */
  .split-layout {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .split-layout.dragging { user-select: none; }
  .split-top {
    flex: 1;
    overflow-y: auto;
    min-height: 120px;
  }
  .h-splitter {
    height: 5px;
    flex-shrink: 0;
    cursor: row-resize;
    background: transparent;
    transition: background 0.15s;
    position: relative;
  }
  .h-splitter:hover, .split-layout.dragging .h-splitter {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent);
  }
  .split-bottom {
    flex-shrink: 0;
    overflow-y: auto;
    border-top: 1px solid var(--border-muted);
  }

  /* Content area (history tab) */
  .page-content {
    flex: 1;
    overflow-y: auto;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 3rem;
    color: var(--text-muted);
    text-align: center;
  }

  /* Agent table (same grid as dashboard) */
  .agent-table {
    overflow-y: auto;
  }
  .table-header {
    display: grid;
    grid-template-columns: 100px 1fr 90px 100px 90px 80px 50px;
    gap: 0.5rem;
    padding: 0.4rem 0.5rem;
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
    padding: 0.5rem 0.5rem;
    border-bottom: 1px solid var(--border-muted);
    font-size: 0.78rem;
    color: var(--text-body);
    cursor: pointer;
    transition: background 0.1s;
    align-items: center;
  }
  .table-row:hover { background: var(--bg-subtle-hover); }
  .table-row.disabled { opacity: 0.5; }

  .col-status { display: flex; align-items: center; gap: 0.35rem; }
  .status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .status-text { font-size: 0.7rem; text-transform: capitalize; }

  .table-row.running .status-dot {
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1) !important;
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    box-shadow: 0 0 4px rgba(99, 102, 241, 0.6), 0 0 8px rgba(168, 85, 247, 0.3);
  }

  .col-title { display: flex; flex-direction: column; gap: 0.1rem; min-width: 0; }
  .agent-title { font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .agent-goal { font-size: 0.68rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .col-schedule { font-size: 0.72rem; color: var(--text-muted); font-family: monospace; display: flex; align-items: center; gap: 0.25rem; flex-wrap: wrap; }
  .schedule-badge {
    font-size: 0.55rem;
    font-weight: 700;
    padding: 0.05rem 0.3rem;
    border-radius: 999px;
    line-height: 1.3;
    font-family: sans-serif;
  }
  .badge-oneshot {
    background: color-mix(in srgb, var(--accent) 15%, var(--bg-elevated));
    color: var(--accent);
    border: 1px solid var(--accent);
  }
  .badge-dated {
    background: color-mix(in srgb, var(--color-success, #22c55e) 12%, var(--bg-elevated));
    color: var(--color-success, #22c55e);
    border: 1px solid var(--color-success, #22c55e);
    font-size: 0.6rem;
  }
  .col-last-run, .col-next-run { display: flex; align-items: center; gap: 0.25rem; font-size: 0.72rem; color: var(--text-muted); }
  .run-status-icon { display: flex; align-items: center; flex-shrink: 0; }
  .col-tokens { font-size: 0.72rem; color: var(--text-muted); font-family: monospace; }
  .col-actions { display: flex; justify-content: flex-end; }

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
  .action-run { background: var(--accent); color: white; }
  .action-cancel {
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    color: white;
    box-shadow: 0 0 6px rgba(99, 102, 241, 0.5), 0 0 12px rgba(168, 85, 247, 0.3);
  }
  .action-btn:hover { filter: brightness(1.15); }

  .row-error {
    padding: 0.15rem 0.5rem 0.4rem 0.5rem;
    font-size: 0.68rem;
    color: var(--color-error, #ef4444);
    border-bottom: 1px solid var(--border-muted);
    padding-left: calc(0.5rem + 100px + 0.5rem);
  }

  /* Run history — fixed-width columns */
  .run-history {
    display: flex;
    flex-direction: column;
  }
  .run-header, .run-row {
    display: grid;
    grid-template-columns: 24px 1fr 90px 65px 55px 85px 90px 2fr;
    gap: 0.5rem;
    padding: 0.4rem 0.5rem;
    align-items: center;
  }
  .run-header {
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
  .run-row {
    font-size: 0.78rem;
    color: var(--text-body);
    border-bottom: 1px solid var(--border-muted);
    cursor: pointer;
    transition: background 0.1s;
  }
  .run-row:hover { background: var(--bg-subtle-hover); }

  .rh-status { display: flex; align-items: center; justify-content: center; }
  .rh-card { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .run-card-title { font-weight: 500; }
  .rh-time { font-size: 0.72rem; color: var(--text-muted); white-space: nowrap; }
  .rh-duration { font-size: 0.72rem; color: var(--text-faint); font-family: monospace; white-space: nowrap; }
  .rh-tools { font-size: 0.72rem; color: var(--text-faint); white-space: nowrap; }
  .rh-tokens { font-size: 0.72rem; color: var(--text-muted); font-family: monospace; white-space: nowrap; }
  .rh-cost { font-size: 0.72rem; color: var(--text-muted); font-family: monospace; white-space: nowrap; min-width: 5.5rem; flex-shrink: 0; overflow: hidden; text-overflow: ellipsis; }
  .rh-model { font-size: 0.6rem; color: var(--text-faint); font-family: sans-serif; }
  .rh-summary { font-size: 0.72rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }

  /* Analytics */
  .analytics-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }
  .chart-card {
    border: 1px solid var(--border-muted);
    border-radius: 8px;
    padding: 0.75rem;
    background: var(--bg-elevated);
  }
  .chart-card-wide {
    grid-column: 1 / -1;
  }
  .chart-title {
    margin: 0 0 0.6rem 0;
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
  }
  .chart-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-align: center;
    padding: 1rem;
  }

  /* Bar chart */
  .bar-chart {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    height: 120px;
    padding-bottom: 1.2rem;
    position: relative;
  }
  .bar-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    height: 100%;
    position: relative;
  }
  .bar-stack {
    flex: 1;
    width: 100%;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    gap: 1px;
  }
  .bar {
    width: 100%;
    border-radius: 2px 2px 0 0;
    min-height: 1px;
    transition: height 0.3s ease;
  }
  .bar-success { background: var(--color-success, #22c55e); }
  .bar-failed { background: var(--color-error, #ef4444); }
  .bar-tokens { background: var(--accent); opacity: 0.7; }
  .bar-label {
    position: absolute;
    bottom: 0;
    font-size: 0.55rem;
    color: var(--text-faint);
  }

  .chart-legend {
    display: flex;
    gap: 0.75rem;
    margin-top: 0.4rem;
    justify-content: center;
  }
  .legend-item {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.65rem;
    color: var(--text-muted);
  }
  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 2px;
  }
  .legend-success { background: var(--color-success, #22c55e); }
  .legend-failed { background: var(--color-error, #ef4444); }

  /* Token breakdown */
  .breakdown-list {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .breakdown-row {
    display: grid;
    grid-template-columns: 1fr 70px 1fr 70px;
    gap: 0.5rem;
    align-items: center;
    padding: 0.3rem 0.25rem;
    border-radius: 4px;
    cursor: pointer;
    transition: background 0.1s;
  }
  .breakdown-row:hover { background: var(--bg-subtle-hover); }
  .breakdown-title {
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--text-body);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .breakdown-runs {
    font-size: 0.68rem;
    color: var(--text-faint);
    text-align: right;
  }
  .breakdown-bar-wrap {
    height: 6px;
    background: var(--bg-surface);
    border-radius: 3px;
    overflow: hidden;
  }
  .breakdown-bar {
    height: 100%;
    background: var(--accent);
    border-radius: 3px;
    transition: width 0.3s ease;
  }
  .breakdown-bar-cost {
    background: var(--color-success, #22c55e);
  }
  .breakdown-tokens {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-body);
    text-align: right;
    font-family: monospace;
  }

  @keyframes agent-neon {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
  }
</style>
