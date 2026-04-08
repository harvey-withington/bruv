<script lang="ts">
  import { GetAgentConfig, SaveAgentConfig, TriggerAgent, CancelAgent, IsLLMConfigured, ListAgentCardIDs } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { board } from '../lib/store.svelte'
  import type { AgentConfig, AgentRun } from '../lib/types'
  import { Bot, Clock, ChevronDown, ChevronRight, CircleCheck, CircleX, TriangleAlert, Play, Square } from 'lucide-svelte'
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn } from '../../wailsjs/runtime/runtime'

  let { cardId }: { cardId: string } = $props()

  let loading = $state(true)
  let llmConfigured = $state(true)
  let saving = $state(false)
  let triggering = $state(false)
  let dirty = $state(false)
  let runs = $state<AgentRun[]>([])
  let expandedRun = $state<string | null>(null)
  let nextRunAt = $state<string | null>(null)

  // Config fields
  let enabled = $state(false)
  let goal = $state('')
  let schedule = $state('')
  let allowedTools = $state<string[]>([])
  let status = $state<'idle' | 'running' | 'failed' | 'disabled'>('disabled')
  let notifyOn = $state<string[]>([])
  let notifyChannel = $state('')

  const AVAILABLE_TOOLS = [
    { id: 'web_fetch', labelKey: 'agent.tool_web_fetch', descKey: 'agent.tool_web_fetch_desc' },
    { id: 'web_search', labelKey: 'agent.tool_web_search', descKey: 'agent.tool_web_search_desc' },
    { id: 'notify', labelKey: 'agent.tool_notify', descKey: 'agent.tool_notify_desc' },
    { id: 'update_self', labelKey: 'agent.tool_update_self', descKey: 'agent.tool_update_self_desc' },
    { id: 'create_card', labelKey: 'agent.tool_create_card', descKey: 'agent.tool_create_card_desc' },
    { id: 'read_card', labelKey: 'agent.tool_read_card', descKey: 'agent.tool_read_card_desc' },
    { id: 'http_request', labelKey: 'agent.tool_http_request', descKey: 'agent.tool_http_request_desc' },
  ] as const

  const SCHEDULE_PRESETS = [
    { label: '@hourly', value: '@hourly' },
    { label: '@daily', value: '@daily' },
    { label: '@weekly', value: '@weekly' },
    { label: '30m', value: '30m' },
  ]

  async function loadConfig() {
    loading = true
    try {
      const [af, isConfigured] = await Promise.all([GetAgentConfig(cardId), IsLLMConfigured()])
      llmConfigured = isConfigured
      enabled = af.config.enabled
      goal = af.config.goal
      schedule = af.config.schedule
      allowedTools = [...(af.config.allowed_tools || [])]
      status = af.config.status || 'disabled'
      notifyOn = [...(af.config.notify_on || [])]
      notifyChannel = af.config.notify_channel || ''
      runs = af.runs || []
      nextRunAt = af.config.next_run_at
      dirty = false
    } catch (e) {
      console.error('Failed to load agent config:', e)
    } finally {
      loading = false
    }
  }

  async function triggerNow() {
    triggering = true
    try {
      await TriggerAgent(cardId)
      showToast(t('agent.triggered'), 'success')
      setTimeout(() => loadConfig(), 1500)
    } catch (e) {
      showToast(t('agent.trigger_failed'), 'error')
    } finally {
      triggering = false
    }
  }

  async function cancelNow() {
    try {
      await CancelAgent(cardId)
      showToast(t('agent.cancelled'), 'info')
      setTimeout(() => loadConfig(), 1000)
    } catch (e) {
      showToast(t('agent.cancel_failed'), 'error')
    }
  }

  async function save() {
    saving = true
    try {
      const config: AgentConfig = {
        enabled,
        goal,
        schedule,
        allowed_tools: allowedTools,
        status: enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled',
        notify_on: notifyOn,
        notify_channel: notifyChannel,
        last_run_at: null,
        next_run_at: null,
        max_tokens_budget: 0,
      }
      await SaveAgentConfig(cardId, config)
      // Refresh board's agent card IDs so indicators update
      try {
        const ids = await ListAgentCardIDs() || []
        const map: Record<string, boolean> = {}
        for (const id of ids) map[id] = true
        board.agentCardIds = map
      } catch { /* ignore */ }
      showToast(t('agent.saved'), 'success')
      dirty = false
    } catch (e) {
      showToast(t('agent.save_failed'), 'error')
      console.error('Failed to save agent config:', e)
    } finally {
      saving = false
    }
  }

  function toggleTool(toolId: string) {
    if (allowedTools.includes(toolId)) {
      allowedTools = allowedTools.filter(t => t !== toolId)
    } else {
      allowedTools = [...allowedTools, toolId]
    }
    dirty = true
  }

  function toggleNotifyOn(value: string) {
    if (notifyOn.includes(value)) {
      notifyOn = notifyOn.filter(n => n !== value)
    } else {
      notifyOn = [...notifyOn, value]
    }
    dirty = true
  }

  function markDirty() { dirty = true }

  function statusColor(s: string): string {
    switch (s) {
      case 'idle': return 'var(--color-success, #22c55e)'
      case 'running': return 'var(--color-info, #3b82f6)'
      case 'failed': return 'var(--color-error, #ef4444)'
      default: return 'var(--color-muted, #94a3b8)'
    }
  }

  function runStatusIcon(s: string) {
    switch (s) {
      case 'success': return CircleCheck
      case 'failure': return CircleX
      case 'timeout': return TriangleAlert
      case 'cancelled': return Square
      default: return Clock
    }
  }

  function formatNextRun(iso: string | null): string {
    if (!iso) return ''
    const d = new Date(iso)
    const diff = d.getTime() - Date.now()
    if (diff < 0) return t('agent.run_overdue')
    const mins = Math.floor(diff / 60000)
    if (mins < 60) return t('agent.next_run', { time: `${mins}m` })
    const hours = Math.floor(mins / 60)
    if (hours < 24) return t('agent.next_run', { time: `${hours}h ${mins % 60}m` })
    return t('agent.next_run', { time: d.toLocaleDateString() })
  }

  // Wails event listeners for live status updates
  let cleanupFns: (() => void)[] = []

  onMount(() => {
    cleanupFns = [
      EventsOn('agent:started', (data: any) => {
        if (data?.cardID === cardId) { status = 'running' }
      }),
      EventsOn('agent:completed', (data: any) => {
        if (data?.cardID === cardId) { loadConfig() }
      }),
      EventsOn('agent:failed', (data: any) => {
        if (data?.cardID === cardId) { loadConfig() }
      }),
    ]
  })

  onDestroy(() => {
    for (const fn of cleanupFns) { if (typeof fn === 'function') fn() }
  })

  $effect(() => {
    void cardId // track cardId so we reload when it changes
    loadConfig()
  })
</script>

{#if loading}
  <div class="agent-loading">
    <Bot size={24} strokeWidth={1.5} />
    <span>{t('app.loading')}</span>
  </div>
{:else}
  <div class="agent-tab">
    {#if !llmConfigured}
      <div class="agent-banner">
        {@html t('chat.not_configured')}
      </div>
    {/if}

    <!-- Header bar -->
    <div class="agent-header">
      <label class="toggle-row">
        <input type="checkbox" bind:checked={enabled} onchange={() => { markDirty(); save() }} />
        <span class="toggle-label">{t('agent.enable')}</span>
      </label>
      <span class="status-badge" style="background: {statusColor(enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled')}">
        {t(`agent.status_${enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled'}`)}
      </span>
      <div class="agent-actions-row">
        {#if enabled && status !== 'running'}
          <button class="run-now-btn" onclick={triggerNow} disabled={triggering}>
            <Play size={14} />
            {triggering ? '...' : t('agent.run_now')}
          </button>
        {/if}
        {#if status === 'running'}
          <button class="cancel-btn" onclick={cancelNow}>
            <Square size={12} />
            {t('agent.cancel')}
          </button>
          <span class="running-indicator">{t('agent.running')}</span>
        {/if}
        {#if enabled && nextRunAt && status !== 'running'}
          <span class="next-run-text">{formatNextRun(nextRunAt)}</span>
        {/if}
        {#if dirty}
          <button class="save-btn" onclick={save} disabled={saving}>
            {saving ? '…' : t('agent.save')}
          </button>
        {/if}
      </div>
    </div>

    <!-- Full-width config sections -->
    <div class="agent-config">
      <!-- Goal -->
      <div class="config-card">
        <div class="config-label">{t('agent.goal')}</div>
        <textarea
          class="agent-textarea"
          rows="3"
          placeholder={t('agent.goal_placeholder')}
          bind:value={goal}
          oninput={markDirty}
        ></textarea>
      </div>

      <!-- Schedule + Notifications side by side -->
      <div class="config-row">
        <div class="config-card config-card-flex">
          <div class="config-label">{t('agent.schedule')}</div>
          <input
            type="text"
            class="agent-input"
            placeholder={t('agent.schedule_placeholder')}
            bind:value={schedule}
            oninput={markDirty}
          />
          <div class="preset-chips">
            {#each SCHEDULE_PRESETS as preset}
              <button
                class="preset-chip"
                class:active={schedule === preset.value}
                onclick={() => { schedule = preset.value; dirty = true }}
              >{preset.label}</button>
            {/each}
          </div>
        </div>

        <div class="config-card config-card-flex">
          <div class="config-label">{t('agent.notifications')}</div>
          <div class="notify-row">
            <span class="notify-label">{t('agent.notify_channel')}</span>
            <select class="agent-select" bind:value={notifyChannel} onchange={markDirty}>
              <option value="">&mdash;</option>
              <option value="in-app">{t('agent.channel_inapp')}</option>
              <option value="system">{t('agent.channel_system')}</option>
              <option value="email">{t('agent.channel_email')}</option>
              <option value="webhook">{t('agent.channel_webhook')}</option>
            </select>
          </div>
          <div class="notify-row">
            <span class="notify-label">{t('agent.notify_on')}</span>
            <div class="notify-checks">
              <label><input type="checkbox" checked={notifyOn.includes('success')} onchange={() => toggleNotifyOn('success')} /> {t('agent.notify_success')}</label>
              <label><input type="checkbox" checked={notifyOn.includes('failure')} onchange={() => toggleNotifyOn('failure')} /> {t('agent.notify_failure')}</label>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Two-column panels: Tools + Run History -->
    <div class="agent-panels">
      <div class="panel">
        <div class="panel-header">
          {t('agent.tools')}
          {#if allowedTools.length > 0}
            <span class="header-badge">{allowedTools.length}</span>
          {/if}
        </div>
        <div class="panel-body panel-scroll">
          {#each AVAILABLE_TOOLS as tool}
            <label class="tool-item">
              <input
                type="checkbox"
                checked={allowedTools.includes(tool.id)}
                onchange={() => toggleTool(tool.id)}
              />
              <div class="tool-info">
                <span class="tool-name">{t(tool.labelKey)}</span>
                <span class="tool-desc">{t(tool.descKey)}</span>
              </div>
            </label>
          {/each}
        </div>
      </div>

      <div class="panel">
        <div class="panel-header">
          {t('agent.runs')}
          {#if runs.length > 0}
            <span class="header-stats">
              <span class="stat-success">{runs.filter(r => r.status === 'success').length}</span> / <span class="stat-fail">{runs.filter(r => r.status === 'failure' || r.status === 'cancelled').length}</span>
            </span>
          {/if}
        </div>
        <div class="panel-body panel-scroll">
          {#if runs.length === 0}
            <p class="runs-empty">{t('agent.runs_empty')}</p>
          {:else}
            {#each runs as run}
              <div class="run-item" class:expanded={expandedRun === run.id}>
                <button class="run-header" onclick={() => expandedRun = expandedRun === run.id ? null : run.id}>
                  <span class="run-toggle">
                    {#if expandedRun === run.id}
                      <ChevronDown size={14} />
                    {:else}
                      <ChevronRight size={14} />
                    {/if}
                  </span>
                  <span class="run-status-icon" style="color: {statusColor(run.status === 'success' ? 'idle' : run.status === 'failure' ? 'failed' : 'disabled')}">
                    <svelte:component this={runStatusIcon(run.status)} size={14} />
                  </span>
                  <span class="run-time">{new Date(run.started_at).toLocaleString()}</span>
                  <span class="run-status-text">{run.status}</span>
                </button>
                {#if expandedRun === run.id}
                  <div class="run-detail">
                    {#if run.summary}
                      <div class="run-field">
                        <span class="run-field-label">{t('agent.run_summary')}</span>
                        <span>{run.summary}</span>
                      </div>
                    {/if}
                    {#if run.error}
                      <div class="run-field run-error">
                        <span class="run-field-label">{t('agent.run_error')}</span>
                        <span>{run.error}</span>
                      </div>
                    {/if}
                    {#if run.tokens_used}
                      <div class="run-field">
                        <span class="run-field-label">{t('agent.run_tokens')}</span>
                        <span>{run.tokens_used}</span>
                      </div>
                    {/if}
                    {#if run.tool_calls?.length}
                      <div class="run-field">
                        <span class="run-field-label">Tool calls</span>
                        <div class="tool-calls">
                          {#each run.tool_calls as tc}
                            <div class="tool-call">
                              <code>{tc.tool}</code>
                              {#if tc.result}<span class="tool-result">{tc.result}</span>{/if}
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* ── Layout ── */
  .agent-loading {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--text-muted);
    justify-content: center;
  }

  .agent-tab {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    overflow: hidden;
    height: 100%;
  }

  /* ── AI not configured banner ── */
  .agent-banner {
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    background: color-mix(in srgb, var(--accent) 8%, var(--bg-elevated));
    font-size: 0.8rem;
    color: var(--text-body);
    line-height: 1.4;
  }
  .agent-banner :global(a) {
    color: var(--accent);
    text-decoration: underline;
  }

  /* ── Header bar ── */
  .agent-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    background: var(--bg-elevated);
  }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }
  .toggle-label {
    font-weight: 500;
    font-size: 0.85rem;
  }
  .status-badge {
    font-size: 0.65rem;
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    color: white;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .agent-actions-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-left: auto;
  }
  .run-now-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    background: var(--accent);
    color: white;
    border: none;
    border-radius: 5px;
    font-size: 0.73rem;
    font-weight: 500;
    cursor: pointer;
    transition: filter 0.1s;
  }
  .run-now-btn:hover { filter: brightness(1.15); }
  .run-now-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .cancel-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    background: #eb5a46;
    color: white;
    border: none;
    border-radius: 5px;
    font-size: 0.73rem;
    font-weight: 500;
    cursor: pointer;
    transition: filter 0.1s;
  }
  .cancel-btn:hover { filter: brightness(1.15); }
  .next-run-text {
    font-size: 0.73rem;
    color: var(--text-muted);
  }
  .running-indicator {
    font-size: 0.73rem;
    color: var(--accent);
    font-weight: 500;
  }
  .save-btn {
    padding: 0.2rem 0.75rem;
    background: var(--accent);
    color: white;
    border: none;
    border-radius: 5px;
    font-size: 0.73rem;
    font-weight: 500;
    cursor: pointer;
    transition: filter 0.1s;
  }
  .save-btn:hover { filter: brightness(1.15); }
  .save-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  /* ── Config area (full-width cards) ── */
  .agent-config {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .config-card {
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    padding: 0.6rem 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    transition: border-color 0.15s;
  }
  .config-card:focus-within {
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
  }

  .config-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
  }

  .config-card-flex { min-width: 0; }

  .config-label {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    margin-bottom: 0.15rem;
  }

  /* ── Inputs ── */
  .agent-textarea, .agent-input, .agent-select {
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: inherit;
    resize: vertical;
    transition: border-color 0.15s;
  }
  .agent-textarea:focus, .agent-input:focus, .agent-select:focus {
    outline: none;
    border-color: var(--accent);
  }

  /* ── Schedule presets ── */
  .preset-chips {
    display: flex;
    gap: 0.3rem;
    flex-wrap: wrap;
    margin-top: 0.2rem;
  }
  .preset-chip {
    padding: 0.15rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 999px;
    background: var(--bg-surface);
    color: var(--text-muted);
    font-size: 0.7rem;
    cursor: pointer;
    font-family: monospace;
    transition: border-color 0.1s, color 0.1s, background 0.1s;
  }
  .preset-chip:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
  .preset-chip.active {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }

  /* ── Notifications ── */
  .notify-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .notify-label {
    font-size: 0.75rem;
    color: var(--text-muted);
    min-width: 4.5rem;
    flex-shrink: 0;
  }
  .notify-checks {
    display: flex;
    gap: 0.6rem;
  }
  .notify-checks label {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    font-size: 0.75rem;
    cursor: pointer;
    color: var(--text-body);
  }

  /* ── Two-column panels ── */
  .agent-panels {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
    min-height: 0;
    flex: 1;
    overflow: hidden;
  }

  .panel {
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;
  }

  .panel-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    flex-shrink: 0;
  }
  .header-badge {
    font-size: 0.6rem;
    font-weight: 700;
    background: var(--accent);
    color: white;
    padding: 0 0.35rem;
    border-radius: 999px;
    line-height: 1.4;
  }
  .header-stats {
    font-size: 0.65rem;
    font-weight: 500;
    margin-left: auto;
    color: var(--text-muted);
    text-transform: none;
    letter-spacing: 0;
  }
  .stat-success { color: #22c55e; }
  .stat-fail { color: #ef4444; }

  .panel-body {
    padding: 0.35rem;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .panel-scroll {
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  /* ── Tools ── */
  .tool-item {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    cursor: pointer;
    transition: background 0.1s;
    flex-shrink: 0;
  }
  .tool-item:hover { background: var(--bg-hover); }
  .tool-item input { margin-top: 0.15rem; }
  .tool-info { display: flex; flex-direction: column; }
  .tool-name { font-size: 0.8rem; font-weight: 500; color: var(--text-body); }
  .tool-desc { font-size: 0.7rem; color: var(--text-muted); }

  /* ── Run history ── */
  .runs-empty {
    font-size: 0.75rem;
    color: var(--text-muted);
    font-style: italic;
    padding: 0.5rem;
  }
  .run-item {
    border-radius: 4px;
    overflow: hidden;
    transition: background 0.1s;
    flex-shrink: 0;
  }
  .run-item + .run-item {
    border-top: 1px solid var(--border-muted);
  }
  .run-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.5rem;
    width: 100%;
    background: none;
    border: none;
    color: var(--text-body);
    cursor: pointer;
    font-size: 0.75rem;
    text-align: left;
    transition: background 0.1s;
  }
  .run-header:hover { background: var(--bg-hover); }
  .run-time {
    flex: 1;
    color: var(--text-body);
  }
  .run-status-text {
    font-size: 0.65rem;
    text-transform: uppercase;
    font-weight: 600;
    color: var(--text-muted);
  }
  .run-detail {
    padding: 0.4rem 0.75rem 0.5rem;
    border-top: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.75rem;
  }
  .run-field {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }
  .run-field-label {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--text-secondary);
  }
  .run-error { color: #eb5a46; }
  .tool-calls {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .tool-call {
    display: flex;
    gap: 0.4rem;
    align-items: baseline;
  }
  .tool-call code {
    font-size: 0.7rem;
    background: var(--bg-surface);
    padding: 0.05rem 0.25rem;
    border-radius: 3px;
    border: 1px solid var(--border-muted);
  }
  .tool-result {
    font-size: 0.7rem;
    color: var(--text-muted);
  }
</style>
