<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { Plus, Pencil, Trash2, X, RefreshCw, Check, CircleAlert, Power, PowerOff } from 'lucide-svelte'
  import {
    ListMCPServers, AddMCPServer, UpdateMCPServer, DeleteMCPServer,
    SetMCPServerSecret, GetMCPServerSecretStatus, RestartMCPServer,
  } from '@shared/api'
  import type { MCPServerView, MCPServerSpec, MCPHealthStatus } from '@shared/types'
  import { draggable } from '../lib/draggable'
  import { focusTrap, portal } from '../lib/actions'

  let { onClose }: { onClose: () => void } = $props()

  // Source of truth — refreshed on every mutation. The list view
  // shows spec + health in one row so users see live status without
  // having to check another surface.
  let servers = $state<MCPServerView[]>([])
  let loading = $state(true)

  // Edit form state. null = not editing; a spec = editing that
  // server (or a brand new one if spec.name is empty). The form is
  // modal-inside-the-modal.
  let editingSpec = $state<MCPServerSpec | null>(null)
  // Track whether we're adding a NEW server (name field editable)
  // vs editing an existing one (name locked — rename is not
  // supported because keychain secrets are keyed by name, and
  // silently orphaning secrets would be worse than no rename).
  let editingIsNew = $state(false)
  // Secret status for the server being edited: map of env var
  // name → is currently set in keychain. We never display values,
  // only "(set)" vs "(not set)" badges.
  let secretStatus = $state<Record<string, boolean>>({})
  // Draft secret values — typed into the form but only committed
  // on Save. Separate from secretStatus so we can distinguish "user
  // typed a new value" from "keychain has something stored".
  let secretDrafts = $state<Record<string, string>>({})

  async function refresh() {
    loading = true
    try {
      servers = await ListMCPServers() ?? []
    } catch (e) {
      showToast(t('mcp.load_failed') + ': ' + String(e), 'error')
      servers = []
    } finally {
      loading = false
    }
  }

  $effect(() => { refresh() })

  function startAdd() {
    editingSpec = {
      name: '',
      description: '',
      command: '',
      args: [],
      env_names: [],
      enabled: true,
    }
    editingIsNew = true
    secretStatus = {}
    secretDrafts = {}
  }

  async function startEdit(view: MCPServerView) {
    // Defensive copy so form edits don't mutate the list rendering.
    editingSpec = {
      name: view.spec.name,
      description: view.spec.description ?? '',
      command: view.spec.command,
      args: [...(view.spec.args ?? [])],
      env_names: [...(view.spec.env_names ?? [])],
      enabled: view.spec.enabled,
    }
    editingIsNew = false
    secretDrafts = {}
    try {
      secretStatus = await GetMCPServerSecretStatus(view.spec.name) ?? {}
    } catch {
      secretStatus = {}
    }
  }

  function cancelEdit() {
    editingSpec = null
    secretStatus = {}
    secretDrafts = {}
  }

  async function saveEdit() {
    if (!editingSpec) return
    const spec = editingSpec
    if (!spec.name.trim()) {
      showToast(t('mcp.error_name_required'), 'error')
      return
    }
    if (!spec.command.trim()) {
      showToast(t('mcp.error_command_required'), 'error')
      return
    }
    try {
      if (editingIsNew) {
        await AddMCPServer(spec)
      } else {
        await UpdateMCPServer(spec)
      }
      // Persist any draft secrets the user typed. We write every
      // draft whose value is non-empty; empty drafts are left alone
      // so the user doesn't have to re-enter unchanged secrets.
      for (const [name, value] of Object.entries(secretDrafts)) {
        if (value !== '') {
          await SetMCPServerSecret(spec.name, name, value)
        }
      }
      // If the user added new env_names that weren't present on
      // the existing server, we also need to restart so the server
      // picks them up on the next spawn. AddMCPServer and
      // UpdateMCPServer already reload the whole registry, so this
      // is automatic — but we call RestartMCPServer explicitly for
      // the case where only secret values changed without a spec
      // change, which wouldn't otherwise trigger a reload.
      if (!editingIsNew && Object.keys(secretDrafts).some(k => secretDrafts[k] !== '')) {
        await RestartMCPServer(spec.name)
      }
      showToast(t('mcp.save_success'), 'success')
      editingSpec = null
      secretStatus = {}
      secretDrafts = {}
      await refresh()
    } catch (e) {
      showToast(t('mcp.save_failed') + ': ' + String(e), 'error')
    }
  }

  async function deleteServer(view: MCPServerView) {
    const ok = await showConfirm(t('mcp.delete_confirm', { name: view.spec.name }))
    if (!ok) return
    try {
      await DeleteMCPServer(view.spec.name)
      showToast(t('mcp.delete_success'), 'success')
      await refresh()
    } catch (e) {
      showToast(t('mcp.delete_failed') + ': ' + String(e), 'error')
    }
  }

  async function toggleEnabled(view: MCPServerView) {
    try {
      const next = { ...view.spec, enabled: !view.spec.enabled }
      await UpdateMCPServer(next)
      await refresh()
    } catch (e) {
      showToast(t('mcp.save_failed') + ': ' + String(e), 'error')
    }
  }

  async function restartServer(view: MCPServerView) {
    try {
      await RestartMCPServer(view.spec.name)
      showToast(t('mcp.restart_success'), 'success')
      await refresh()
    } catch (e) {
      showToast(t('mcp.restart_failed') + ': ' + String(e), 'error')
    }
  }

  // --- Args list editing helpers ---
  // Args are rendered as one-input-per-arg so users don't have to
  // think about shell quoting. Add/remove buttons keep the list
  // simple.
  function addArg() {
    if (!editingSpec) return
    editingSpec.args = [...(editingSpec.args ?? []), '']
  }
  function removeArg(index: number) {
    if (!editingSpec) return
    editingSpec.args = (editingSpec.args ?? []).filter((_, i) => i !== index)
  }
  function updateArg(index: number, value: string) {
    if (!editingSpec) return
    const args = [...(editingSpec.args ?? [])]
    args[index] = value
    editingSpec.args = args
  }

  // --- Env var list editing helpers ---
  // env_names is a list of strings (names only). Values go through
  // secretDrafts and then SetMCPServerSecret on save.
  let newEnvName = $state('')
  function addEnvName() {
    if (!editingSpec || !newEnvName.trim()) return
    const trimmed = newEnvName.trim()
    if ((editingSpec.env_names ?? []).includes(trimmed)) {
      showToast(t('mcp.error_duplicate_env'), 'error')
      return
    }
    editingSpec.env_names = [...(editingSpec.env_names ?? []), trimmed]
    newEnvName = ''
  }
  function removeEnvName(name: string) {
    if (!editingSpec) return
    editingSpec.env_names = (editingSpec.env_names ?? []).filter(n => n !== name)
    delete secretStatus[name]
    delete secretDrafts[name]
  }

  function healthLabel(status: MCPHealthStatus): string {
    return t('mcp.health_' + status)
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (editingSpec) {
        cancelEdit()
      } else {
        onClose()
      }
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" role="dialog" tabindex="-1" aria-label={t('mcp.title')} use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('mcp.title')}</h2>
      <div class="header-actions">
        <button class="icon-btn" onclick={startAdd} title={t('mcp.add')} aria-label={t('mcp.add')}>
          <Plus size={16} />
        </button>
        <button class="close-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}>
          <X size={18} />
        </button>
      </div>
    </div>

    <div class="dialog-body">
      <p class="intro">{t('mcp.intro')}</p>

      {#if loading}
        <p class="muted">{t('common.loading')}</p>
      {:else if servers.length === 0}
        <div class="empty-state">
          <p>{t('mcp.empty_title')}</p>
          <p class="muted">{t('mcp.empty_subtitle')}</p>
          <button class="btn btn-primary" onclick={startAdd}>
            <Plus size={14} /> {t('mcp.add_first')}
          </button>
        </div>
      {:else}
        <ul class="server-list">
          {#each servers as view (view.spec.name)}
            <li class="server-row" class:disabled={!view.spec.enabled}>
              <div class="server-main">
                <div class="server-name-row">
                  <span class="server-name">{view.spec.name}</span>
                  <span class="health-badge health-{view.health.status}">
                    {#if view.health.status === 'ready'}
                      <Check size={10} />
                    {:else if view.health.status === 'failed'}
                      <CircleAlert size={10} />
                    {/if}
                    {healthLabel(view.health.status)}
                  </span>
                  {#if view.health.tool_count > 0}
                    <span class="tool-count">{view.health.tool_count} {t('mcp.tools_label')}</span>
                  {/if}
                </div>
                {#if view.spec.description}
                  <p class="server-description">{view.spec.description}</p>
                {/if}
                {#if view.health.last_error}
                  <p class="server-error">{view.health.last_error}</p>
                {/if}
                {#if view.tools.length > 0}
                  <details class="tools-disclosure">
                    <summary>{t('mcp.show_tools')}</summary>
                    <ul class="tool-list">
                      {#each view.tools as tool}
                        <li>
                          <code>{tool.name}</code>
                          {#if tool.description}
                            <span class="tool-description">— {tool.description}</span>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                  </details>
                {/if}
              </div>
              <div class="server-actions">
                <button
                  class="icon-btn"
                  onclick={() => toggleEnabled(view)}
                  title={view.spec.enabled ? t('mcp.disable') : t('mcp.enable')}
                  aria-label={view.spec.enabled ? t('mcp.disable') : t('mcp.enable')}
                >
                  {#if view.spec.enabled}
                    <Power size={13} />
                  {:else}
                    <PowerOff size={13} />
                  {/if}
                </button>
                <button
                  class="icon-btn"
                  onclick={() => restartServer(view)}
                  title={t('mcp.restart')}
                  aria-label={t('mcp.restart')}
                >
                  <RefreshCw size={13} />
                </button>
                <button
                  class="icon-btn"
                  onclick={() => startEdit(view)}
                  title={t('mcp.edit')}
                  aria-label={t('mcp.edit')}
                >
                  <Pencil size={13} />
                </button>
                <button
                  class="icon-btn danger"
                  onclick={() => deleteServer(view)}
                  title={t('mcp.delete')}
                  aria-label={t('mcp.delete')}
                >
                  <Trash2 size={13} />
                </button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  {#if editingSpec}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="overlay edit-overlay" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) cancelEdit() }}>
      <div class="dialog edit-dialog" role="dialog" tabindex="-1" aria-label={editingIsNew ? t('mcp.add_title') : t('mcp.edit_title')} use:focusTrap>
        <div class="dialog-header">
          <h2>{editingIsNew ? t('mcp.add_title') : t('mcp.edit_title')}</h2>
          <button class="close-btn" onclick={cancelEdit} title={t('common.close')} aria-label={t('common.close')}>
            <X size={18} />
          </button>
        </div>
        <div class="dialog-body">
          <div class="field-row">
            <span class="field-label">{t('mcp.field_name')}</span>
            <input
              class="field-input"
              bind:value={editingSpec.name}
              placeholder={t('mcp.field_name_placeholder')}
              disabled={!editingIsNew}
            />
            {#if !editingIsNew}
              <span class="field-hint">{t('mcp.field_name_locked')}</span>
            {/if}
          </div>

          <div class="field-row">
            <span class="field-label">{t('mcp.field_description')}</span>
            <input
              class="field-input"
              bind:value={editingSpec.description}
              placeholder={t('mcp.field_description_placeholder')}
            />
          </div>

          <div class="field-row">
            <span class="field-label">{t('mcp.field_command')}</span>
            <input
              class="field-input mono"
              bind:value={editingSpec.command}
              placeholder="npx"
            />
            <span class="field-hint">{t('mcp.field_command_hint')}</span>
          </div>

          <div class="field-row">
            <span class="field-label">{t('mcp.field_args')}</span>
            <div class="args-list">
              {#each editingSpec.args ?? [] as arg, i}
                <div class="arg-row">
                  <input
                    class="field-input mono"
                    value={arg}
                    oninput={(e) => updateArg(i, (e.target as HTMLInputElement).value)}
                    placeholder={t('mcp.field_arg_placeholder')}
                  />
                  <button class="icon-btn danger" onclick={() => removeArg(i)} aria-label={t('mcp.remove_arg')}>
                    <X size={12} />
                  </button>
                </div>
              {/each}
              <button class="btn btn-ghost btn-sm" onclick={addArg}>
                <Plus size={12} /> {t('mcp.add_arg')}
              </button>
            </div>
          </div>

          <div class="field-row">
            <span class="field-label">{t('mcp.field_env_names')}</span>
            <div class="env-list">
              {#each editingSpec.env_names ?? [] as name}
                <div class="env-row">
                  <div class="env-name-group">
                    <code class="env-name">{name}</code>
                    {#if secretStatus[name] && !secretDrafts[name]}
                      <span class="secret-status set">{t('mcp.secret_set')}</span>
                    {:else if secretDrafts[name]}
                      <span class="secret-status pending">{t('mcp.secret_pending')}</span>
                    {:else}
                      <span class="secret-status unset">{t('mcp.secret_unset')}</span>
                    {/if}
                    <button class="icon-btn danger small" onclick={() => removeEnvName(name)} aria-label={t('mcp.remove_env')}>
                      <X size={11} />
                    </button>
                  </div>
                  <input
                    type="password"
                    class="field-input mono secret-input"
                    placeholder={secretStatus[name] ? t('mcp.secret_placeholder_set') : t('mcp.secret_placeholder_empty')}
                    bind:value={secretDrafts[name]}
                  />
                </div>
              {/each}
              <div class="arg-row">
                <input
                  class="field-input mono"
                  bind:value={newEnvName}
                  placeholder={t('mcp.env_name_placeholder')}
                  onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addEnvName() } }}
                />
                <button class="btn btn-ghost btn-sm" onclick={addEnvName}>
                  <Plus size={12} /> {t('mcp.add_env')}
                </button>
              </div>
            </div>
            <span class="field-hint">{t('mcp.field_env_hint')}</span>
          </div>

          <div class="field-row">
            <label class="toggle-row">
              <input type="checkbox" bind:checked={editingSpec.enabled} />
              <span>{t('mcp.field_enabled')}</span>
            </label>
          </div>
        </div>
        <div class="dialog-footer">
          <button class="btn btn-ghost" onclick={cancelEdit}>{t('common.cancel')}</button>
          <button class="btn btn-primary" onclick={saveEdit}>{t('common.save')}</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .edit-overlay {
    z-index: 210;
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 560px;
    max-width: 92vw;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .edit-dialog { width: 520px; }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border-muted);
    cursor: move;
  }
  .dialog-header h2 { margin: 0; font-size: 0.95rem; font-weight: 600; }
  .header-actions { display: flex; gap: 4px; align-items: center; }

  .dialog-body {
    padding: 1rem 1.1rem;
    overflow-y: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-height: 0;
  }
  .dialog-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 0.75rem 1.1rem;
    border-top: 1px solid var(--border-muted);
  }

  .intro { font-size: 12px; color: var(--text-muted); margin: 0 0 8px; line-height: 1.5; }
  .muted { color: var(--text-muted); font-size: 12px; }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 24px 0;
    text-align: center;
  }
  .empty-state p { margin: 0; font-size: 13px; }

  .server-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }

  .server-row {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    padding: 10px 12px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .server-row.disabled { opacity: 0.6; }
  .server-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .server-name-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .server-name { font-size: 13px; font-weight: 600; }
  .server-description { font-size: 11px; color: var(--text-muted); margin: 0; line-height: 1.4; }
  .server-error {
    font-size: 11px;
    color: var(--danger);
    margin: 2px 0 0;
    font-family: ui-monospace, Consolas, monospace;
    word-break: break-word;
  }
  .tool-count {
    font-size: 10px;
    color: var(--text-muted);
    padding: 1px 6px;
    background: var(--bg-elevated);
    border-radius: 4px;
  }

  .health-badge {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 1px 6px;
    border-radius: 4px;
  }
  .health-ready       { color: var(--text-primary); background: color-mix(in srgb, var(--accent) 20%, transparent); }
  .health-starting,
  .health-restarting  { color: var(--text-muted); background: var(--bg-elevated); }
  .health-failed      { color: #fff; background: var(--danger); }
  .health-disabled    { color: var(--text-muted); background: var(--bg-elevated); }

  .tools-disclosure {
    margin-top: 6px;
    font-size: 11px;
    color: var(--text-muted);
  }
  .tools-disclosure summary {
    cursor: pointer;
    user-select: none;
    padding: 2px 0;
  }
  .tool-list {
    list-style: none;
    margin: 4px 0 0 8px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 160px;
    overflow-y: auto;
  }
  .tool-list li {
    font-size: 11px;
    line-height: 1.4;
  }
  .tool-list code {
    font-family: ui-monospace, Consolas, monospace;
    color: var(--text-primary);
  }
  .tool-description { color: var(--text-muted); }

  .server-actions { display: flex; gap: 3px; flex-shrink: 0; }
  .icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 4px;
    border-radius: 4px;
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon-btn:hover { color: var(--text-primary); background: var(--bg-hover); }
  .icon-btn.danger:hover { color: var(--danger); }
  .icon-btn.small { padding: 2px; }
  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 2px;
    line-height: 1;
  }
  .close-btn:hover { color: var(--text-primary); }

  /* --- edit form --- */
  .field-row { display: flex; flex-direction: column; gap: 4px; }
  .field-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }
  .field-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    box-sizing: border-box;
  }
  .field-input:focus { outline: none; border-color: var(--accent); }
  .field-input.mono { font-family: ui-monospace, Consolas, monospace; font-size: 12px; }
  .field-input:disabled { opacity: 0.6; cursor: not-allowed; }
  .field-hint { font-size: 10px; color: var(--text-muted); font-style: italic; }

  .args-list, .env-list { display: flex; flex-direction: column; gap: 6px; }
  .arg-row { display: flex; gap: 6px; align-items: center; }
  .arg-row .field-input { flex: 1; }

  .env-row {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 6px 8px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .env-name-group {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .env-name {
    font-family: ui-monospace, Consolas, monospace;
    font-size: 11px;
    color: var(--text-primary);
  }
  .secret-status {
    font-size: 9px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 1px 5px;
    border-radius: 3px;
    margin-left: auto;
    margin-right: 2px;
  }
  .secret-status.set { color: var(--text-primary); background: color-mix(in srgb, var(--accent) 20%, transparent); }
  .secret-status.unset { color: var(--text-muted); background: var(--bg-elevated); }
  .secret-status.pending { color: #fff; background: #f59e0b; }
  .secret-input { width: 100%; }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    cursor: pointer;
  }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 14px;
    font-size: 12px;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn:hover { background: var(--bg-hover); }
  .btn-primary { background: var(--accent); color: white; border-color: var(--accent); }
  .btn-primary:hover { filter: brightness(1.1); }
  .btn-ghost { background: transparent; }
  .btn-sm { padding: 4px 10px; font-size: 11px; }
</style>
