<script lang="ts">
  import { GetAgentConfig, SaveAgentConfig } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { AgentConfig, AgentRun } from '../lib/types'
  import { Bot, Play, Clock, Bell, ChevronDown, ChevronRight, CheckCircle, XCircle, AlertTriangle } from 'lucide-svelte'

  let { cardId }: { cardId: string } = $props()

  let loading = $state(true)
  let saving = $state(false)
  let dirty = $state(false)
  let runs = $state<AgentRun[]>([])
  let expandedRun = $state<string | null>(null)

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
      const af = await GetAgentConfig(cardId)
      enabled = af.config.enabled
      goal = af.config.goal
      schedule = af.config.schedule
      allowedTools = [...(af.config.allowed_tools || [])]
      status = af.config.status || 'disabled'
      notifyOn = [...(af.config.notify_on || [])]
      notifyChannel = af.config.notify_channel || ''
      runs = af.runs || []
      dirty = false
    } catch (e) {
      console.error('Failed to load agent config:', e)
    } finally {
      loading = false
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
      }
      await SaveAgentConfig(cardId, config)
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
      case 'success': return CheckCircle
      case 'failure': return XCircle
      case 'timeout': return AlertTriangle
      default: return Clock
    }
  }

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
    <!-- Enable toggle + status -->
    <div class="agent-section agent-enable">
      <label class="toggle-row">
        <input type="checkbox" bind:checked={enabled} onchange={() => { markDirty(); save() }} />
        <span class="toggle-label">{t('agent.enable')}</span>
        <span class="status-badge" style="background: {statusColor(enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled')}">
          {t(`agent.status_${enabled ? (status === 'disabled' ? 'idle' : status) : 'disabled'}`)}
        </span>
      </label>
    </div>

    <!-- Goal -->
    <div class="agent-section">
      <label class="section-label">{t('agent.goal')}</label>
      <textarea
        class="agent-textarea"
        rows="3"
        placeholder={t('agent.goal_placeholder')}
        bind:value={goal}
        oninput={markDirty}
      ></textarea>
    </div>

    <!-- Schedule -->
    <div class="agent-section">
      <label class="section-label">{t('agent.schedule')}</label>
      <div class="schedule-row">
        <input
          type="text"
          class="agent-input"
          placeholder={t('agent.schedule_placeholder')}
          bind:value={schedule}
          oninput={markDirty}
        />
      </div>
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

    <!-- Allowed Tools -->
    <div class="agent-section">
      <label class="section-label">{t('agent.tools')}</label>
      <p class="section-desc">{t('agent.tools_desc')}</p>
      <div class="tools-list">
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

    <!-- Notifications -->
    <div class="agent-section">
      <label class="section-label">{t('agent.notifications')}</label>
      <div class="notify-row">
        <label class="notify-label">{t('agent.notify_channel')}</label>
        <select class="agent-select" bind:value={notifyChannel} onchange={markDirty}>
          <option value="">&mdash;</option>
          <option value="in-app">{t('agent.channel_inapp')}</option>
          <option value="system">{t('agent.channel_system')}</option>
          <option value="email">{t('agent.channel_email')}</option>
          <option value="webhook">{t('agent.channel_webhook')}</option>
        </select>
      </div>
      <div class="notify-row">
        <label class="notify-label">{t('agent.notify_on')}</label>
        <div class="notify-checks">
          <label><input type="checkbox" checked={notifyOn.includes('success')} onchange={() => toggleNotifyOn('success')} /> {t('agent.notify_success')}</label>
          <label><input type="checkbox" checked={notifyOn.includes('failure')} onchange={() => toggleNotifyOn('failure')} /> {t('agent.notify_failure')}</label>
        </div>
      </div>
    </div>

    <!-- Save button -->
    {#if dirty}
      <div class="agent-section save-section">
        <button class="save-btn" onclick={save} disabled={saving}>
          {saving ? '…' : t('agent.save')}
        </button>
      </div>
    {/if}

    <!-- Run History -->
    <div class="agent-section">
      <label class="section-label">{t('agent.runs')}</label>
      {#if runs.length === 0}
        <p class="runs-empty">{t('agent.runs_empty')}</p>
      {:else}
        <div class="runs-list">
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
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .agent-loading {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--color-text-secondary);
    justify-content: center;
  }

  .agent-tab {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    padding: 1rem;
    overflow-y: auto;
    max-height: 100%;
  }

  .agent-section {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .section-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-secondary);
  }

  .section-desc {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
    margin: 0;
  }

  /* Enable toggle */
  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }
  .toggle-label { font-weight: 500; }
  .status-badge {
    font-size: 0.7rem;
    padding: 0.125rem 0.5rem;
    border-radius: 999px;
    color: white;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    margin-left: auto;
  }

  /* Textarea & inputs */
  .agent-textarea, .agent-input, .agent-select {
    width: 100%;
    padding: 0.5rem;
    border: 1px solid var(--color-border);
    border-radius: 6px;
    background: var(--color-bg-secondary);
    color: var(--color-text);
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
  }
  .agent-textarea:focus, .agent-input:focus, .agent-select:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  /* Schedule presets */
  .preset-chips {
    display: flex;
    gap: 0.375rem;
    flex-wrap: wrap;
    margin-top: 0.25rem;
  }
  .preset-chip {
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    background: var(--color-bg-secondary);
    color: var(--color-text-secondary);
    font-size: 0.75rem;
    cursor: pointer;
    font-family: monospace;
  }
  .preset-chip:hover { border-color: var(--color-accent); }
  .preset-chip.active {
    background: var(--color-accent);
    color: white;
    border-color: var(--color-accent);
  }

  /* Tools checklist */
  .tools-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .tool-item {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.375rem 0.5rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .tool-item:hover { background: var(--color-bg-hover); }
  .tool-item input { margin-top: 0.2rem; }
  .tool-info { display: flex; flex-direction: column; }
  .tool-name { font-size: 0.85rem; font-weight: 500; }
  .tool-desc { font-size: 0.75rem; color: var(--color-text-tertiary); }

  /* Notifications */
  .notify-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  .notify-label {
    font-size: 0.8rem;
    color: var(--color-text-secondary);
    min-width: 5rem;
  }
  .notify-checks {
    display: flex;
    gap: 0.75rem;
  }
  .notify-checks label {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.8rem;
    cursor: pointer;
  }

  /* Save */
  .save-section { align-items: flex-start; }
  .save-btn {
    padding: 0.4rem 1.25rem;
    background: var(--color-accent);
    color: white;
    border: none;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .save-btn:hover { filter: brightness(1.1); }
  .save-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Run history */
  .runs-empty {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
    font-style: italic;
    margin: 0;
  }
  .runs-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .run-item {
    border: 1px solid var(--color-border);
    border-radius: 6px;
    overflow: hidden;
  }
  .run-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.375rem 0.5rem;
    width: 100%;
    background: none;
    border: none;
    color: var(--color-text);
    cursor: pointer;
    font-size: 0.8rem;
    text-align: left;
  }
  .run-header:hover { background: var(--color-bg-hover); }
  .run-time { flex: 1; }
  .run-status-text {
    font-size: 0.7rem;
    text-transform: uppercase;
    font-weight: 600;
    color: var(--color-text-secondary);
  }
  .run-detail {
    padding: 0.5rem 0.75rem;
    border-top: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    font-size: 0.8rem;
  }
  .run-field {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }
  .run-field-label {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }
  .run-error { color: var(--color-error, #ef4444); }
  .tool-calls {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .tool-call {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
  }
  .tool-call code {
    font-size: 0.75rem;
    background: var(--color-bg-secondary);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
  }
  .tool-result {
    font-size: 0.75rem;
    color: var(--color-text-tertiary);
  }
</style>
