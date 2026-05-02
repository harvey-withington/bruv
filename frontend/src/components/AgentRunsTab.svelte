<script lang="ts">
  import { GetAgentConfig, ClearAgentRuns } from '@shared/api'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { AgentRun } from '@shared/types'
  import { Clock, CircleCheck, CircleX, TriangleAlert, Square, Trash2, Timer } from 'lucide-svelte'
  import { onMount, onDestroy } from 'svelte'
  import { onEvent } from '../lib/events'

  let { cardId }: { cardId: string } = $props()

  let loading = $state(true)
  let runs = $state<AgentRun[]>([])
  let expandedRun = $state<string | null>(null)

  async function loadRuns(silent: boolean = false) {
    if (!silent) loading = true
    try {
      const af = await GetAgentConfig(cardId)
      runs = af.runs || []
    } catch (e) {
      console.error('Failed to load agent runs:', e)
    } finally {
      loading = false
    }
  }

  async function clearRuns() {
    try {
      await ClearAgentRuns(cardId)
      runs = []
      expandedRun = null
      showToast(t('agent.runs_cleared'), 'success')
    } catch {
      showToast(t('error.delete_failed'), 'error')
    }
  }

  function statusIcon(s: string) {
    switch (s) {
      case 'success': return CircleCheck
      case 'failure': return CircleX
      case 'timeout': return TriangleAlert
      case 'cancelled': return Square
      default: return Clock
    }
  }

  function statusColor(s: string): string {
    switch (s) {
      case 'success': return 'var(--color-success, #22c55e)'
      case 'failure': return 'var(--color-error, #ef4444)'
      default: return 'var(--color-muted, #94a3b8)'
    }
  }

  function formatTime(iso: string): string {
    const d = new Date(iso)
    const diff = Date.now() - d.getTime()
    const mins = Math.floor(diff / 60000)
    if (mins < 1) return 'just now'
    if (mins < 60) return `${mins}m ago`
    const hours = Math.floor(mins / 60)
    if (hours < 24) return `${hours}h ago`
    const days = Math.floor(hours / 24)
    if (days < 7) return `${days}d ago`
    return d.toLocaleDateString()
  }

  function formatDuration(started: string, finished: string | null): string {
    if (!finished) return '—'
    const secs = Math.round((new Date(finished).getTime() - new Date(started).getTime()) / 1000)
    if (secs < 60) return `${secs}s`
    const mins = Math.floor(secs / 60)
    return `${mins}m ${secs % 60}s`
  }

  function parseError(err: string): string {
    if (!err) return ''
    if (err.startsWith('{')) {
      try {
        const parsed = JSON.parse(err) as Record<string, unknown>
        return (parsed.message || parsed.error || parsed.msg || err) as string
      } catch { /* not JSON, use as-is */ }
    }
    return err
  }

  let cleanupFns: (() => void)[] = []
  onMount(() => {
    cleanupFns = [
      // Silent reload — don't flash the loading placeholder every time
      // an agent run completes while the Runs tab is open.
      onEvent<{ cardID?: string }>('agent:completed', (data) => { if (data?.cardID === cardId) loadRuns(true) }),
      onEvent<{ cardID?: string }>('agent:failed', (data) => { if (data?.cardID === cardId) loadRuns(true) }),
    ]
  })
  onDestroy(() => { for (const fn of cleanupFns) { if (typeof fn === 'function') fn() } })

  $effect(() => {
    void cardId
    loadRuns()
  })
</script>

{#if loading}
  <div class="runs-loading">
    <Timer size={20} strokeWidth={1.5} />
    <span>{t('app.loading')}</span>
  </div>
{:else}
  <div class="runs-tab">
    {#if runs.length > 0}
      <div class="runs-header">
        <span class="runs-summary">
          {runs.length} {t('agents_page.stat_runs').toLowerCase()}
          — <span class="stat-ok">{runs.filter(r => r.status === 'success').length}</span> {t('agent.notify_success').toLowerCase()}
          / <span class="stat-fail">{runs.filter(r => r.status === 'failure' || r.status === 'cancelled').length}</span> {t('agent.notify_failure').toLowerCase()}
        </span>
        <button class="clear-btn" onclick={clearRuns} title={t('agent.clear_runs')}>
          <Trash2 size={12} />
          <span>{t('agent.clear_runs')}</span>
        </button>
      </div>
    {/if}

    {#if runs.length === 0}
      <p class="runs-empty">{t('agent.runs_empty')}</p>
    {:else}
      <div class="runs-list">
        {#each runs as run}
          <button
            class="run-row"
            class:expanded={expandedRun === run.id}
            onclick={() => expandedRun = expandedRun === run.id ? null : run.id}
          >
            <span class="run-icon" style:color={statusColor(run.status)}>
              {#each [statusIcon(run.status)] as RunIcon}
                <RunIcon size={14} />
              {/each}
            </span>
            <span class="run-time">{formatTime(run.started_at)}</span>
            <span class="run-duration">{formatDuration(run.started_at, run.finished_at ?? null)}</span>
            <span class="run-badge">{run.tool_calls?.length ?? 0} {t('agents_page.col_tools').toLowerCase()}</span>
            {#if run.tokens_used}
              <span class="run-badge accent">{run.tokens_used.toLocaleString()} tok</span>
            {/if}
            <span class="run-preview">{parseError(run.error) || run.summary || t('agent.run_no_summary')}</span>
          </button>
          {#if expandedRun === run.id}
            <div class="run-detail">
              {#if run.summary}
                <div class="detail-field">
                  <span class="detail-label">{t('agent.run_summary')}</span>
                  <p class="detail-value">{run.summary}</p>
                </div>
              {/if}
              {#if run.error}
                <div class="detail-field detail-error">
                  <span class="detail-label">{t('agent.run_error')}</span>
                  <p class="detail-value">{parseError(run.error)}</p>
                </div>
              {/if}
              {#if run.tokens_used}
                <div class="detail-field">
                  <span class="detail-label">{t('agent.run_tokens')}</span>
                  <span class="detail-value">{run.tokens_used.toLocaleString()}</span>
                </div>
              {/if}
              {#if run.tool_calls?.length}
                <div class="detail-field">
                  <span class="detail-label">{t('agent.run_tools_used')}</span>
                  <span class="detail-value tools-list">{run.tool_calls.map(tc => tc.tool).join(', ')}</span>
                </div>
              {/if}
            </div>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .runs-loading {
    display: flex; align-items: center; gap: 0.5rem;
    padding: 2rem; color: var(--text-muted); justify-content: center;
  }
  .runs-tab {
    padding: 1rem 1.25rem;
    display: flex; flex-direction: column; gap: 0.5rem;
  }
  .runs-header {
    display: flex; align-items: center; justify-content: space-between;
    padding-bottom: 0.5rem; border-bottom: 1px solid var(--border-muted);
  }
  .runs-summary { font-size: 0.8rem; color: var(--text-muted); }
  .stat-ok { color: #22c55e; font-weight: 500; }
  .stat-fail { color: #ef4444; font-weight: 500; }
  .clear-btn {
    display: flex; align-items: center; gap: 0.3rem;
    padding: 0.2rem 0.5rem; border-radius: 4px;
    border: 1px solid var(--border-muted); background: none;
    color: var(--text-muted); font-size: 0.7rem; cursor: pointer;
  }
  .clear-btn:hover { color: var(--danger, #ef4444); border-color: var(--danger, #ef4444); }
  .runs-empty {
    padding: 2rem; text-align: center;
    color: var(--text-muted); font-size: 0.85rem; font-style: italic;
  }
  .runs-list { display: flex; flex-direction: column; }
  .run-row {
    display: flex; align-items: center; gap: 0.5rem;
    padding: 0.45rem 0.6rem; width: 100%;
    background: none; border: none; border-bottom: 1px solid var(--border-muted);
    color: var(--text-body); cursor: pointer; font-size: 0.8rem; text-align: left;
    transition: background var(--duration-fast);
  }
  .run-row:hover { background: var(--bg-hover); }
  .run-row:first-child { border-top: 1px solid var(--border-muted); }
  .run-icon { display: flex; flex-shrink: 0; }
  .run-time { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; min-width: 4rem; }
  .run-duration { font-size: 0.7rem; color: var(--text-faint); flex-shrink: 0; min-width: 3rem; }
  .run-badge {
    font-size: 0.65rem; padding: 0.05rem 0.35rem; border-radius: 3px;
    background: var(--bg-elevated); border: 1px solid var(--border-muted);
    color: var(--text-muted); flex-shrink: 0; white-space: nowrap;
  }
  .run-badge.accent {
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-elevated));
  }
  .run-preview {
    flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis;
    white-space: nowrap; font-size: 0.75rem; color: var(--text-secondary);
  }
  .run-detail {
    padding: 0.6rem 1rem; background: var(--bg-elevated);
    border-bottom: 1px solid var(--border-muted);
    display: flex; flex-direction: column; gap: 0.4rem;
  }
  .detail-field { display: flex; flex-direction: column; gap: 0.1rem; }
  .detail-label {
    font-size: 0.65rem; font-weight: 600; text-transform: uppercase;
    letter-spacing: 0.04em; color: var(--text-faint);
  }
  .detail-value { font-size: 0.8rem; color: var(--text-body); margin: 0; line-height: 1.4; }
  .detail-error .detail-value { color: var(--danger, #ef4444); }
  .tools-list { font-family: monospace; font-size: 0.75rem; }
</style>
