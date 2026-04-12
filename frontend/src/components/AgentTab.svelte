<script lang="ts">
  import { GetAgentConfig, SaveAgentConfig, TriggerAgent, CancelAgent, IsLLMConfigured, ListAgentCardIDs, GetLLMAccounts, ListMCPServers } from '../lib/api'
  import type { MCPServerView } from '../lib/types'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { board } from '../lib/store.svelte'
  import type { AgentConfig, LLMAccount } from '../lib/types'
  import { Timer, Play, Square } from 'lucide-svelte'
  import LLMAccountSelect from './LLMAccountSelect.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { EventsOn } from '../../wailsjs/runtime/runtime'

  let { cardId }: { cardId: string } = $props()

  let loading = $state(true)
  let llmConfigured = $state(true)
  let saving = $state(false)
  let triggering = $state(false)
  let dirty = $state(false)
  let nextRunAt = $state<string | null>(null)

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

  async function loadConfig() {
    loading = true
    try {
      const [af, isConfigured, accounts, servers] = await Promise.all([GetAgentConfig(cardId), IsLLMConfigured(), GetLLMAccounts(), ListMCPServers()])
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
        notify_channel: notifyChannels.length > 0 ? notifyChannels.join(',') : '',
        llm_account_id: llmAccountId,
        llm_model: llmModel,
        last_run_at: null,
        next_run_at: null,
        max_tokens_budget: maxTokensBudget,
        run_started_at: null,
        min_interval_minutes: minIntervalMins,
        max_retries: maxRetries,
        retry_count: 0,
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
      case 'idle': return 'var(--color-success, #22c55e)'
      case 'running': return 'var(--color-info, #3b82f6)'
      case 'failed': return 'var(--color-error, #ef4444)'
      default: return 'var(--color-muted, #94a3b8)'
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
      // Re-fetch config when the card is updated externally — this
      // covers configure_agent called from the card chat or project
      // chat, which emits card:updated after saving the new config.
      // Without this the Agent tab shows stale goal/schedule/tools
      // until the user closes and reopens the card.
      EventsOn('card:updated', (data: any) => {
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
    <Timer size={24} strokeWidth={1.5} />
    <span>{t('app.loading')}</span>
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
                          <span class="tool-desc">{server.tools.length} {server.tools.length === 1 ? 'tool' : 'tools'}</span>
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
    transition: filter 0.1s;
  }
  .run-now-btn:hover { filter: brightness(1.15); }
  .run-now-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .cancel-btn {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    color: white;
    box-shadow: 0 0 6px rgba(99, 102, 241, 0.5), 0 0 12px rgba(168, 85, 247, 0.3);
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
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    box-shadow: 0 0 4px rgba(99, 102, 241, 0.4), 0 0 8px rgba(168, 85, 247, 0.2);
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
    transition: filter 0.1s;
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
    transition: all 0.15s;
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
    transition: border-color 0.15s;
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
    transition: background 0.1s;
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
    transition: border-color 0.15s;
  }
  .cost-reset-btn:hover {
    border-color: var(--accent);
  }
</style>
