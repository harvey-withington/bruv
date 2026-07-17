<script lang="ts">
  import { GetAgentConfig, SaveAgentConfig, TriggerAgent, CancelAgent, DeleteAgent, GetAgentRuns, IsLLMConfigured, ListAgentCardStates, GetLLMAccounts, ListMCPServers, ValidateSchedulePreview } from '@shared/api'
  import type { MCPServerView } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { board } from '../lib/store.svelte'
  import { downloadBlob } from '@shared/download'
  import type { AgentConfig, LLMAccount } from '@shared/types'
  import { Timer, Play, Square, Download, Trash2 } from 'lucide-svelte'
  import LLMAccountSelect from './LLMAccountSelect.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { onEvent } from '../lib/events'

  let { cardId }: { cardId: string } = $props()

  let loading = $state(true)
  let loadError = $state(false)
  let llmConfigured = $state(true)
  let saving = $state(false)
  let triggering = $state(false)
  let removing = $state(false)
  let dirty = $state(false)
  let nextRunAt = $state<string | null>(null)

  // Scheduler bookkeeping loaded with the config and sent back verbatim
  // on save. Hardcoding these to null/0 meant every config touch reset
  // one-shot / min-interval / retry state — a one-shot agent that had
  // already run would fire again after any settings tweak.
  let lastRunAt = $state<string | null>(null)
  let runStartedAt = $state<string | null>(null)
  let retryCount = $state(0)

  // Wizard step
  let agentStep = $state<1 | 2 | 3>(1)

  // Config fields
  let enabled = $state(false)
  let goal = $state('')
  let schedule = $state('')
  let allowedTools = $state<string[]>([])
  let status = $state<'idle' | 'running' | 'failed' | 'disabled'>('disabled')
  let notifyOn = $state<string[]>([])
  let notifyChannels = $state<string[]>([])
  let llmAccountId = $state('')
  let llmModel = $state('')
  let llmAccounts = $state<LLMAccount[]>([])
  let maxTokensBudget = $state(0)
  let minIntervalMins = $state(0)
  let maxRetries = $state(0)
  let retryBackoffMins = $state(0)
  let costBudgetUSD = $state(0)
  let costSpentUSD = $state(0)
  let startDate = $state('')
  let endDate = $state('')
  let activeWindowStart = $state('')
  let activeWindowEnd = $state('')
  let oneShot = $state(false)
  let timezone = $state('')
  const COMMON_TIMEZONES = [
    '', 'UTC',
    'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
    'Europe/London', 'Europe/Paris', 'Europe/Berlin',
    'Asia/Tokyo', 'Asia/Shanghai', 'Australia/Sydney',
  ]

  type ToolDef = { id: string; labelKey: string; descKey: string }
  type ToolGroup = { titleKey: string; tools: ToolDef[] }

  const TOOL_GROUPS: ToolGroup[] = [
    {
      titleKey: 'agent.tool_group_web',
      tools: [
        { id: 'web_fetch', labelKey: 'agent.tool_web_fetch', descKey: 'agent.tool_web_fetch_desc' },
        { id: 'web_search', labelKey: 'agent.tool_web_search', descKey: 'agent.tool_web_search_desc' },
        { id: 'http_request', labelKey: 'agent.tool_http_request', descKey: 'agent.tool_http_request_desc' },
      ],
    },
    {
      titleKey: 'agent.tool_group_card',
      tools: [
        { id: 'update_self', labelKey: 'agent.tool_update_self', descKey: 'agent.tool_update_self_desc' },
        { id: 'read_card', labelKey: 'agent.tool_read_card', descKey: 'agent.tool_read_card_desc' },
        { id: 'create_card', labelKey: 'agent.tool_create_card', descKey: 'agent.tool_create_card_desc' },
      ],
    },
    {
      titleKey: 'agent.tool_group_system',
      tools: [
        { id: 'notify', labelKey: 'agent.tool_notify', descKey: 'agent.tool_notify_desc' },
      ],
    },
  ]


  // MCP servers — one checkbox per server, toggles all its tools.
  let mcpServers = $state<MCPServerView[]>([])

  function mcpServerToolIds(server: MCPServerView): string[] {
    return server.tools.map(t => t.namespace_id)
  }

  function isMcpServerEnabled(server: MCPServerView): boolean {
    const ids = mcpServerToolIds(server)
    return ids.length > 0 && ids.every(id => allowedTools.includes(id))
  }

  function toggleMcpServer(server: MCPServerView) {
    const ids = mcpServerToolIds(server)
    if (isMcpServerEnabled(server)) {
      allowedTools = allowedTools.filter(id => !ids.includes(id))
    } else {
      allowedTools = [...new Set([...allowedTools, ...ids])]
    }
    dirty = true
  }

  const SCHEDULE_PRESETS = [
    { label: '@hourly', value: '@hourly' },
    { label: '@daily', value: '@daily' },
    { label: '@weekly', value: '@weekly' },
    { label: '30m', value: '30m' },
  ]

  // Schedule preview — debounced call to the (previously orphaned)
  // ValidateSchedulePreview RPC. Purely advisory: an RPC failure hides
  // the preview silently rather than surfacing a toast, and a staleness
  // token (mirroring store.svelte.ts's boardLoadSeq) drops responses
  // for a schedule the user has already changed away from.
  let schedulePreviewRuns = $state<string[]>([])
  let schedulePreviewInvalid = $state(false)
  let schedulePreviewSeq = 0
  let schedulePreviewTimer: ReturnType<typeof setTimeout> | undefined

  function toRFC3339(localDateTime: string): string {
    if (!localDateTime) return ''
    const d = new Date(localDateTime)
    return isNaN(d.getTime()) ? '' : d.toISOString()
  }

  function updateSchedulePreview() {
    clearTimeout(schedulePreviewTimer)
    const sched = schedule.trim()
    if (!sched) {
      schedulePreviewRuns = []
      schedulePreviewInvalid = false
      return
    }
    schedulePreviewTimer = setTimeout(async () => {
      const seq = ++schedulePreviewSeq
      try {
        const runs = await ValidateSchedulePreview(sched, toRFC3339(startDate), toRFC3339(endDate), timezone, 3)
        if (seq !== schedulePreviewSeq) return // stale — schedule changed again while this was in flight
        schedulePreviewRuns = runs || []
        // An empty result with no error means the backend couldn't parse
        // the schedule string at all.
        schedulePreviewInvalid = schedulePreviewRuns.length === 0
      } catch {
        if (seq !== schedulePreviewSeq) return
        schedulePreviewRuns = []
        schedulePreviewInvalid = false
      }
    }, 400)
  }

  // Stale-response guard: this tab is reused across cards (mention-link
  // navigation swaps cardId mid-load). Without it a slow GetAgentConfig
  // for the previous card populates the form under the new card — and
  // Save would then write the OLD card's agent config onto the new one.
  let configLoadSeq = 0

  async function loadConfig(silent: boolean = false) {
    const seq = ++configLoadSeq
    if (!silent) loading = true
    try {
      const [af, isConfigured, accounts, servers] = await Promise.all([GetAgentConfig(cardId), IsLLMConfigured(), GetLLMAccounts(), ListMCPServers()])
      if (seq !== configLoadSeq) return
      mcpServers = servers ?? []
      llmAccounts = accounts || []
      llmConfigured = isConfigured
      enabled = af.config.enabled
      goal = af.config.goal
      schedule = af.config.schedule
      allowedTools = [...(af.config.allowed_tools || [])]
      status = af.config.status || 'disabled'
      notifyOn = [...(af.config.notify_on || [])]
      // Parse comma-separated channels, filtering out "in-app" (it's always implicit)
      const rawChannels = (af.config.notify_channel || '').split(',').map((s: string) => s.trim()).filter((s: string) => s && s !== 'in-app')
      notifyChannels = rawChannels
      llmAccountId = af.config.llm_account_id || ''
      llmModel = af.config.llm_model || ''
      maxTokensBudget = af.config.max_tokens_budget || 0
      minIntervalMins = af.config.min_interval_minutes || 0
      maxRetries = af.config.max_retries || 0
      retryBackoffMins = af.config.retry_backoff_minutes || 0
      costBudgetUSD = af.config.cost_budget_usd || 0
      costSpentUSD = af.config.cost_spent_usd || 0
      startDate = af.config.start_date || ''
      endDate = af.config.end_date || ''
      activeWindowStart = af.config.active_window_start || ''
      activeWindowEnd = af.config.active_window_end || ''
      oneShot = af.config.one_shot || false
      timezone = af.config.timezone || ''
      nextRunAt = af.config.next_run_at
      lastRunAt = af.config.last_run_at ?? null
      runStartedAt = af.config.run_started_at ?? null
      retryCount = af.config.retry_count || 0
      loadError = false
      dirty = false
    } catch (e) {
      if (seq !== configLoadSeq) return
      // Render an explicit error state instead of a default form — a
      // transient load failure followed by Save used to overwrite the
      // real on-disk config with blank defaults.
      loadError = true
      console.error('Failed to load agent config:', e)
      showToast(t('agent.load_failed'), 'error')
    } finally {
      if (seq === configLoadSeq) loading = false
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
    if (loadError) return // never overwrite a config we failed to read
    saving = true
    try {
      const config: AgentConfig = {
        enabled,
        goal,
        schedule,
        allowed_tools: allowedTools,
        status: enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled',
        notify_on: notifyOn,
        notify_channel: notifyChannels.length > 0 ? notifyChannels.join(',') : '',
        llm_account_id: llmAccountId,
        llm_model: llmModel,
        // Bookkeeping is preserved verbatim — the backend feeds
        // last_run_at into its schedule math, so nulling it here used
        // to re-fire one-shot agents and bypass min-interval. next_run_at
        // is recomputed server-side on every save regardless of what we
        // send (agentsvc.SaveConfig); passing the loaded value is just
        // the honest round-trip.
        last_run_at: lastRunAt,
        next_run_at: nextRunAt,
        max_tokens_budget: maxTokensBudget,
        run_started_at: runStartedAt,
        min_interval_minutes: minIntervalMins,
        max_retries: maxRetries,
        retry_count: retryCount,
        retry_backoff_minutes: retryBackoffMins,
        cost_budget_usd: costBudgetUSD,
        cost_spent_usd: costSpentUSD,
        start_date: startDate || null,
        end_date: endDate || null,
        active_window_start: activeWindowStart,
        active_window_end: activeWindowEnd,
        one_shot: oneShot,
        timezone,
      }
      await SaveAgentConfig(cardId, config)
      // Refresh board's agent card states so indicators update
      try {
        board.agentCardStates = (await ListAgentCardStates()) || {}
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

  // The card has an agent file on disk (enabled or not) — drives the
  // remove/export affordances. agentCardStates carries every card with
  // a config file; the local fields cover a just-saved config before
  // the states map refreshes.
  const hasAgent = $derived(cardId in board.agentCardStates || enabled || goal !== '' || schedule !== '')

  // Export the run history as a JSON download — offered before Remove
  // so the history isn't lost silently (it's deleted with the agent).
  async function exportHistory() {
    try {
      const runs = (await GetAgentRuns(cardId)) ?? []
      const payload = { card_id: cardId, exported_at: new Date().toISOString(), runs }
      downloadBlob(JSON.stringify(payload, null, 2), `agent-history-${cardId}.json`, 'application/json;charset=utf-8')
      showToast(t('agent.history_exported'), 'success')
    } catch (e) {
      showToast(t('agent.history_export_failed'), 'error')
      console.error('Failed to export agent run history:', e)
    }
  }

  async function removeAgent() {
    if (!await showConfirm(t('agent.remove_confirm'))) return
    removing = true
    try {
      await DeleteAgent(cardId)
      showToast(t('agent.removed'), 'success')
      // Refresh board indicators + reset this tab to the plain-card state.
      try {
        board.agentCardStates = (await ListAgentCardStates()) || {}
      } catch { /* ignore */ }
      await loadConfig(true)
    } catch (e) {
      showToast(t('agent.remove_failed'), 'error')
      console.error('Failed to remove agent:', e)
    } finally {
      removing = false
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

  function toggleNotifyChannel(ch: string) {
    if (notifyChannels.includes(ch)) {
      notifyChannels = notifyChannels.filter(c => c !== ch)
    } else {
      notifyChannels = [...notifyChannels, ch]
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

  async function resetCost() {
    costSpentUSD = 0
    dirty = true
  }

  function markDirty() { dirty = true }

  function statusColor(s: string): string {
    switch (s) {
      case 'idle': return 'var(--success)'
      case 'running': return 'var(--info)'
      case 'failed': return 'var(--danger)'
      default: return 'var(--text-muted)'
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
      onEvent<{ cardID?: string }>('agent:started', (data) => {
        if (data?.cardID === cardId) { status = 'running' }
      }),
      onEvent<{ cardID?: string }>('agent:completed', (data) => {
        if (data?.cardID === cardId && !dirty) { loadConfig(true) }
      }),
      onEvent<{ cardID?: string }>('agent:failed', (data) => {
        if (data?.cardID === cardId && !dirty) { loadConfig(true) }
      }),
      // Re-fetch config when the card is updated externally — this
      // covers configure_agent called from the card chat or project
      // chat, which emits card:updated after saving the new config.
      // Without this the Agent tab shows stale goal/schedule/tools
      // until the user closes and reopens the card.
      // Skip reload when the user has unsaved edits to avoid losing them.
      // Silent reload — no loading placeholder, so the tab doesn't
      // flash every time the running agent writes back to the card.
      onEvent<{ cardID?: string }>('card:updated', (data) => {
        if (data?.cardID === cardId && !dirty) { loadConfig(true) }
      }),
    ]
  })

  onDestroy(() => {
    for (const fn of cleanupFns) { if (typeof fn === 'function') fn() }
    clearTimeout(schedulePreviewTimer)
  })

  $effect(() => {
    void cardId // track cardId so we reload when it changes
    loadConfig()
  })

  $effect(() => {
    // Track every field the preview depends on so a change to any of
    // them (not just the raw cron text) re-triggers the debounce.
    void schedule; void startDate; void endDate; void timezone
    updateSchedulePreview()
  })
</script>

{#if loading}
  <div class="agent-loading">
    <Timer size={24} strokeWidth={1.5} />
    <span>{t('app.loading')}</span>
  </div>
{:else if loadError}
  <!-- Explicit error state: rendering the default form here would let
       Save overwrite the real on-disk config with blanks. -->
  <div class="agent-loading agent-load-error">
    <span class="load-error-text">{t('agent.load_failed')}</span>
    <button class="retry-btn" onclick={() => loadConfig()}>{t('agent.load_retry')}</button>
  </div>
{:else}
  <div class="agent-tab">
    {#if !llmConfigured}
      <div class="agent-banner">
        {@html t('chat.not_configured')}
      </div>
    {/if}

    <!-- Header: Enable + Status + Actions -->
    <div class="agent-header">
      <label class="toggle-row">
        <input type="checkbox" bind:checked={enabled} onchange={() => { markDirty(); save() }} />
        <span class="toggle-label">{t('agent.enable')}</span>
      </label>
      <span class="status-badge" class:status-running={status === 'running'} style={status === 'running' ? '' : `background: ${statusColor(enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled')}`}>
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

    <!-- Step indicators -->
    <div class="step-nav">
      <button class="step-pill" class:active={agentStep === 1} onclick={() => agentStep = 1}>
        <span class="step-num">1</span>
        <span class="step-label">{t('agent.step_setup')}</span>
      </button>
      <button class="step-pill" class:active={agentStep === 2} onclick={() => agentStep = 2}>
        <span class="step-num">2</span>
        <span class="step-label">{t('agent.step_schedule')}</span>
      </button>
      <button class="step-pill" class:active={agentStep === 3} onclick={() => agentStep = 3}>
        <span class="step-num">3</span>
        <span class="step-label">{t('agent.step_permissions')}</span>
      </button>
    </div>

    <!-- Step content -->
    <div class="step-content">
      {#if agentStep === 1}
        <!-- STEP 1: Goal & Model -->
        <div class="config-card">
          <div class="config-label">{t('agent.goal')}</div>
          <textarea
            class="agent-textarea"
            rows="8"
            placeholder={t('agent.goal_placeholder')}
            bind:value={goal}
            oninput={markDirty}
          ></textarea>
        </div>

        {#if llmAccounts.length > 0}
          <div class="config-card">
            <div class="config-label">{t('agent.llm_account')}</div>
            <LLMAccountSelect
              accounts={llmAccounts}
              bind:selectedAccountId={llmAccountId}
              bind:selectedModel={llmModel}
              onchange={() => markDirty()}
            />
          </div>
        {/if}

      {:else if agentStep === 2}
        <!-- STEP 2: Schedule & Notifications -->
        <div class="config-card">
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
          {#if schedule.trim()}
            {#if schedulePreviewInvalid}
              <p class="schedule-preview schedule-preview-invalid">{t('agent.schedule_invalid')}</p>
            {:else if schedulePreviewRuns.length > 0}
              <div class="schedule-preview">
                <span class="schedule-preview-label">{t('agent.schedule_next_runs')}</span>
                <ul class="schedule-preview-list">
                  {#each schedulePreviewRuns as run}
                    <li>{new Date(run).toLocaleString()}</li>
                  {/each}
                </ul>
              </div>
            {/if}
          {/if}
        </div>

        <div class="config-row">
          <div class="config-card">
            <div class="config-label">{t('agent.start_date')}</div>
            <input type="datetime-local" class="agent-input" bind:value={startDate} oninput={markDirty} />
          </div>
          <div class="config-card">
            <div class="config-label">{t('agent.end_date')}</div>
            <input type="datetime-local" class="agent-input" bind:value={endDate} oninput={markDirty} />
          </div>
        </div>

        <div class="config-row">
          <div class="config-card">
            <div class="config-label">{t('agent.active_window_start')}</div>
            <input type="time" class="agent-input" bind:value={activeWindowStart} oninput={markDirty} />
          </div>
          <div class="config-card">
            <div class="config-label">{t('agent.active_window_end')}</div>
            <input type="time" class="agent-input" bind:value={activeWindowEnd} oninput={markDirty} />
          </div>
        </div>

        <div class="config-row">
          <div class="config-card">
            <label class="toggle-row toggle-row-sm">
              <input type="checkbox" bind:checked={oneShot} onchange={markDirty} />
              <span class="schedule-field-label">{t('agent.one_shot')}</span>
            </label>
          </div>
          <div class="config-card">
            <div class="config-label">{t('agent.timezone')}</div>
            <select class="agent-input" bind:value={timezone} onchange={markDirty}>
              {#each COMMON_TIMEZONES as tz}
                <option value={tz}>{tz || t('agent.timezone_local')}</option>
              {/each}
            </select>
          </div>
        </div>

      {:else}
        <!-- STEP 3: Permissions — Tools & Safety side by side, Notifications below -->
        <div class="config-row">
          <div class="config-card perm-column">
            <div class="config-label">{t('agent.tools')}</div>
            <div class="perm-scroll">
              <div class="tools-list">
                {#each TOOL_GROUPS as group}
                  {@const groupIds = group.tools.map((t: ToolDef) => t.id)}
                  {@const allChecked = groupIds.every((id: string) => allowedTools.includes(id))}
                  {@const someChecked = groupIds.some((id: string) => allowedTools.includes(id)) && !allChecked}
                  <div class="tool-group">
                    <label class="tool-group-header">
                      <input
                        type="checkbox"
                        checked={allChecked}
                        indeterminate={someChecked}
                        onchange={() => {
                          if (allChecked) {
                            allowedTools = allowedTools.filter(id => !groupIds.includes(id))
                          } else {
                            allowedTools = [...new Set([...allowedTools, ...groupIds])]
                          }
                          dirty = true
                        }}
                      />
                      <span class="tool-group-title">{t(group.titleKey)}</span>
                    </label>
                    {#each group.tools as tool}
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
                {/each}
                {#if mcpServers.some(s => s.health.status === 'ready' && s.tools.length > 0)}
                  <div class="tool-group mcp-group">
                    <span class="tool-group-title">{t('agent.tool_group_mcp')}</span>
                    {#each mcpServers.filter(s => s.health.status === 'ready' && s.tools.length > 0) as server}
                      <label class="tool-item">
                        <input
                          type="checkbox"
                          checked={isMcpServerEnabled(server)}
                          onchange={() => toggleMcpServer(server)}
                        />
                        <div class="tool-info">
                          <span class="tool-name">{server.spec.name}</span>
                          <span class="tool-desc">{server.tools.length === 1 ? t('agent.mcp_tool_count_one') : t('agent.mcp_tool_count_other', { n: server.tools.length })}</span>
                        </div>
                      </label>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          </div>

          <div class="config-card perm-column">
            <div class="config-label">{t('agent.safety')}</div>
            <div class="perm-scroll">
              <div class="safety-stack">
                <label class="safety-field">
                  <span class="safety-label">{t('agent.token_budget')}</span>
                  <input type="number" class="agent-input" placeholder={t('agent.token_budget_placeholder')} bind:value={maxTokensBudget} oninput={markDirty} min="0" step="1000" />
                  <span class="safety-hint">{t('agent.token_budget_hint')}</span>
                </label>
                <label class="safety-field">
                  <span class="safety-label">{t('agent.min_interval')}</span>
                  <input type="number" class="agent-input" placeholder={t('agent.min_interval_placeholder')} bind:value={minIntervalMins} oninput={markDirty} min="0" />
                  <span class="safety-hint">{t('agent.min_interval_hint')}</span>
                </label>
                <label class="safety-field">
                  <span class="safety-label">{t('agent.max_retries')}</span>
                  <input type="number" class="agent-input" placeholder={t('agent.max_retries_placeholder')} bind:value={maxRetries} oninput={markDirty} min="0" max="10" />
                  <span class="safety-hint">{t('agent.max_retries_hint')}</span>
                </label>
                <label class="safety-field">
                  <span class="safety-label">{t('agent.retry_backoff')}</span>
                  <input type="number" class="agent-input" placeholder={t('agent.retry_backoff_placeholder')} bind:value={retryBackoffMins} oninput={markDirty} min="0" />
                  <span class="safety-hint">{t('agent.retry_backoff_hint')}</span>
                </label>
                <label class="safety-field">
                  <span class="safety-label">{t('agent.cost_budget')}</span>
                  <input type="number" class="agent-input" placeholder={t('agent.cost_budget_placeholder')} bind:value={costBudgetUSD} oninput={markDirty} min="0" step="0.01" />
                  <span class="safety-hint">{t('agent.cost_budget_hint')}</span>
                  {#if costSpentUSD > 0}
                    <span class="cost-spent-row">
                      <span class="cost-spent-label">{t('agent.cost_spent')}: ${costSpentUSD.toFixed(4)}</span>
                      <button type="button" class="cost-reset-btn" onclick={resetCost}>{t('agent.cost_reset')}</button>
                    </span>
                  {/if}
                </label>
              </div>
            </div>
          </div>
        </div>

        <div class="config-card">
          <div class="config-label">{t('agent.notifications')}</div>
          <div class="notify-row">
            <span class="notify-label">{t('agent.notify_channel')}</span>
            <div class="notify-checks">
              <label><input type="checkbox" checked={notifyChannels.includes('system')} onchange={() => toggleNotifyChannel('system')} /> {t('agent.channel_system')}</label>
              <label><input type="checkbox" checked={notifyChannels.includes('email')} onchange={() => toggleNotifyChannel('email')} /> {t('agent.channel_email')}</label>
              <label><input type="checkbox" checked={notifyChannels.includes('webhook')} onchange={() => toggleNotifyChannel('webhook')} /> {t('agent.channel_webhook')}</label>
            </div>
          </div>
          <div class="notify-row">
            <span class="notify-label">{t('agent.notify_on')}</span>
            <div class="notify-checks">
              <label><input type="checkbox" checked={notifyOn.includes('success')} onchange={() => toggleNotifyOn('success')} /> {t('agent.notify_success')}</label>
              <label><input type="checkbox" checked={notifyOn.includes('failure')} onchange={() => toggleNotifyOn('failure')} /> {t('agent.notify_failure')}</label>
            </div>
          </div>
        </div>
      {/if}
    </div>

    {#if hasAgent}
      <!-- Remove agent: the confirm warns that run history is deleted
           and points at Export history, which sits right next to it. -->
      <div class="remove-row">
        <button class="export-btn" onclick={exportHistory}>
          <Download size={13} />
          {t('agent.export_history')}
        </button>
        <button
          class="remove-btn"
          onclick={removeAgent}
          disabled={removing || status === 'running'}
          title={status === 'running' ? t('agent.remove_while_running') : t('agent.remove_tooltip')}
        >
          <Trash2 size={13} />
          {removing ? '…' : t('common.remove')}
        </button>
      </div>
    {/if}
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
  .agent-load-error {
    flex-direction: column;
  }
  .load-error-text {
    color: var(--danger);
  }
  .retry-btn {
    padding: 0.3rem 0.75rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    color: var(--text-body);
    font-size: 0.75rem;
    cursor: pointer;
    transition: border-color var(--duration-normal), color var(--duration-normal);
  }
  .retry-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  .agent-tab {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
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
  .toggle-row-sm {
    font-size: 0.75rem;
  }
  .toggle-row-sm input[type="checkbox"] {
    width: 13px;
    height: 13px;
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
    transition: filter var(--duration-fast);
  }
  .run-now-btn:hover { filter: brightness(1.15); }
  .run-now-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .cancel-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    background: var(--agent-running-gradient);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    color: white;
    box-shadow: var(--agent-running-glow);
    border: none;
    border-radius: 5px;
    font-size: 0.73rem;
    font-weight: 500;
    cursor: pointer;
  }
  .cancel-btn:hover { filter: brightness(1.15); }
  .next-run-text {
    font-size: 0.73rem;
    color: var(--text-muted);
  }

  /* Neon gradient animation for running agents */
  .status-badge.status-running {
    background: var(--agent-running-gradient);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    /* Same token as the sibling badges in AgentDashboard/AgentsPage —
       the hand-tuned rgba pair here had already drifted from them. */
    box-shadow: var(--agent-running-glow-sm);
  }
  @keyframes agent-neon {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
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
    transition: filter var(--duration-fast);
  }
  .save-btn:hover { filter: brightness(1.15); }
  .save-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  /* ── Step nav ── */
  .step-nav {
    display: flex;
    gap: 0.25rem;
    padding: 0;
  }
  .step-pill {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.75rem;
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    background: none;
    color: var(--text-muted);
    font-size: 0.75rem;
    cursor: pointer;
    transition: all var(--duration-normal);
    flex: 1;
    justify-content: center;
  }
  .step-pill:hover { border-color: var(--accent); color: var(--text-primary); }
  .step-pill.active {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    border-color: var(--accent);
    color: var(--accent);
  }
  .step-num {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--border-muted);
    color: var(--text-muted);
    font-size: 0.65rem;
    font-weight: 700;
    flex-shrink: 0;
  }
  .step-pill.active .step-num {
    background: var(--accent);
    color: white;
  }
  .step-label { white-space: nowrap; }

  /* ── Step content ── */
  .step-content {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  /* ── Config cards ── */
  .config-card {
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    padding: 0.6rem 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    transition: border-color var(--duration-normal);
  }
  .config-card:focus-within {
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
  }

  .config-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
  }

  .config-label {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    margin-bottom: 0.15rem;
  }

  /* ── Inputs ── */
  .agent-textarea, .agent-input {
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: inherit;
    resize: vertical;
    transition: border-color var(--duration-normal);
    color-scheme: dark light;
  }
  :global([data-theme="dark"]) .agent-input { color-scheme: dark; }
  :global([data-theme="light"]) .agent-input { color-scheme: light; }
  .agent-textarea:focus, .agent-input:focus {
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
    transition: border-color var(--duration-fast), color var(--duration-fast), background var(--duration-fast);
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

  /* ── Schedule preview ── */
  .schedule-preview {
    margin: 0.3rem 0 0;
    font-size: 0.7rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .schedule-preview-label {
    font-size: 0.62rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .schedule-preview-list {
    margin: 0;
    padding-left: 1.1rem;
    color: var(--text-muted);
    font-family: monospace;
  }
  .schedule-preview-invalid {
    color: var(--danger);
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

  /* ── Tools ── */
  .perm-column {
    min-width: 0;
  }
  .perm-scroll {
    max-height: 30vh;
    overflow-y: auto;
  }

  .tools-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .tool-group {
    margin-bottom: 0.4rem;
  }
  .tool-group-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.4rem;
    cursor: pointer;
  }
  .tool-group-title {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
  }
  .mcp-group {
    border-top: 1px solid var(--border-muted);
    padding-top: 0.5rem;
    margin-top: 0.3rem;
  }
  .tool-item {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin-left: 0.6rem;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    cursor: pointer;
    transition: background var(--duration-fast);
    flex-shrink: 0;
  }
  .tool-item:hover { background: var(--bg-hover); }
  .tool-item input { margin-top: 0.15rem; }
  .tool-info { display: flex; flex-direction: column; }
  .tool-name { font-size: 0.8rem; font-weight: 500; color: var(--text-body); }
  .tool-desc { font-size: 0.7rem; color: var(--text-muted); }

  /* ── Safety stack (vertical) ── */
  .safety-stack {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .safety-field {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .safety-label {
    font-size: 0.7rem;
    font-weight: 500;
    color: var(--text-body);
  }
  .safety-hint {
    font-size: 0.6rem;
    color: var(--text-muted);
    line-height: 1.3;
  }
  .cost-spent-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.2rem;
  }
  .cost-spent-label {
    font-size: 0.68rem;
    color: var(--text-body);
    font-family: monospace;
  }
  .cost-reset-btn {
    padding: 0.1rem 0.4rem;
    font-size: 0.62rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    color: var(--text-body);
    cursor: pointer;
    transition: border-color var(--duration-normal);
  }
  .cost-reset-btn:hover {
    border-color: var(--accent);
  }

  /* ── Remove agent row ── */
  .remove-row {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--border-muted);
  }
  .export-btn,
  .remove-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.6rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    color: var(--text-body);
    font-size: 0.73rem;
    cursor: pointer;
    transition: border-color var(--duration-normal), color var(--duration-normal);
  }
  .export-btn:hover { border-color: var(--accent); color: var(--accent); }
  .remove-btn { color: var(--danger); }
  .remove-btn:hover { border-color: var(--danger); }
  .remove-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
