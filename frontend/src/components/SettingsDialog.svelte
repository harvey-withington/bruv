<script lang="ts">
  import { X, Eye, EyeOff, Search, Bell } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { GetPreferences, SetPreferences, GetLLMConfig, SetLLMConfig, GetNotifyConfig, SetNotifyConfig, GetLLMAccounts, SaveLLMAccounts, TestSystemNotification } from '../lib/api'
  import LLMAccountsManager from './LLMAccountsManager.svelte'
  import type { LLMAccount } from '../lib/types'
  import { theme, setTheme } from '../lib/theme.svelte'
  import { setLocale, availableLocales } from '../lib/i18n.svelte'
  import { nav, prefs as prefsStore } from '../lib/store.svelte'
  import { draggable } from '../lib/draggable'
  import { focusTrap } from '../lib/actions'
  type TabId = 'general' | 'ai' | 'notifications'

  let { onClose, initialTab }: { onClose: () => void; initialTab?: TabId } = $props()

  const startTab: TabId = initialTab ?? 'general'
  let activeTab = $state<TabId>(startTab)
  let searchQuery = $state('')
  let searchInputEl = $state<HTMLInputElement | null>(null)

  // --- General prefs ---
  let prefs = $state({
    reopen_last_repo: false,
    theme: 'dark',
    locale: 'en',
    confirm_before_delete: true,
    sidebar_width: 260,
    type_badge_display: 'color' as 'text' | 'color' | 'hidden',
    default_category_name: 'Ideas',
    inbox_recent_cards_limit: 21,
    inbox_activity_limit: 25,
    sidebar_collapse_default: false,
  })

  // --- AI / LLM ---
  let llm = $state({
    context: '',
    provider: '',
    model: '',
    api_key: '',
    base_url: '',
    auto_pin: 'off',
    ai_mode: 'edit',
    min_confidence: '',
  })
  let notifCfg = $state({
    system_enabled: false,
    smtp_host: '',
    smtp_port: 587,
    smtp_username: '',
    smtp_password: '',
    smtp_from_addr: '',
    smtp_to_addr: '',
    smtp_tls: true,
    webhook_url: '',
    webhook_auth_header: '',
  })
  let llmAccounts = $state<LLMAccount[]>([])
  let showSmtpPassword = $state(false)
  let testingSystem = $state(false)

  let loaded = $state(false)

  $effect(() => { loadAll() })

  async function loadAll() {
    try {
      const [p, c, nc, accts] = await Promise.all([
        GetPreferences(),
        GetLLMConfig(),
        GetNotifyConfig(),
        GetLLMAccounts(),
      ])
      llmAccounts = accts || []
      if (p) {
        prefs.reopen_last_repo = p.reopen_last_repo ?? false
        prefs.theme = p.theme || 'dark'
        prefs.locale = p.locale || 'en'
        prefs.confirm_before_delete = p.confirm_before_delete ?? true
        prefs.sidebar_width = nav.sidebarWidth
        prefs.type_badge_display = (p.type_badge_display || 'color') as 'text' | 'color' | 'hidden'
        prefs.default_category_name = p.default_category_name || 'Ideas'
        prefs.inbox_recent_cards_limit = p.inbox_recent_cards_limit || 21
        prefs.inbox_activity_limit = p.inbox_activity_limit || 25
        prefs.sidebar_collapse_default = p.sidebar_collapse_default ?? false
      }
      if (c) {
        llm.context = c.context || ''
        llm.provider = c.provider || ''
        llm.model = c.model || ''
        llm.api_key = c.api_key || ''
        llm.base_url = c.base_url || ''
        llm.auto_pin = c.auto_pin || 'off'
        llm.ai_mode = c.ai_mode || 'edit'
        llm.min_confidence = c.min_confidence || ''
      }
      if (nc) {
        notifCfg.system_enabled = nc.system_enabled ?? false
        notifCfg.smtp_host = nc.smtp_host || ''
        notifCfg.smtp_port = nc.smtp_port || 587
        notifCfg.smtp_username = nc.smtp_username || ''
        notifCfg.smtp_password = nc.smtp_password || ''
        notifCfg.smtp_from_addr = nc.smtp_from_addr || ''
        notifCfg.smtp_to_addr = nc.smtp_to_addr || ''
        notifCfg.smtp_tls = nc.smtp_tls ?? true
        notifCfg.webhook_url = nc.webhook_url || ''
        notifCfg.webhook_auth_header = nc.webhook_auth_header || ''
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await Promise.all([
        SetPreferences(prefs),
        SetLLMConfig(llm),
        SetNotifyConfig(notifCfg),
        SaveLLMAccounts(llmAccounts),
      ])
      setTheme(prefs.theme as 'dark' | 'light')
      setLocale(prefs.locale)
      nav.sidebarWidth = prefs.sidebar_width
      localStorage.setItem('bruv-sidebar-width', String(prefs.sidebar_width))
      prefsStore.typeBadgeDisplay = prefs.type_badge_display
      onClose()
    } catch (e) { console.error('Settings save error:', e) }
  }

  async function testSystemNotif() {
    testingSystem = true
    try {
      await TestSystemNotification()
      showToast(t('notifications.test_system_ok'), 'success')
    } catch {
      showToast(t('notifications.test_system_fail'), 'error')
    }
    testingSystem = false
  }

  // --- Search filtering ---
  interface SettingsField {
    tab: TabId
    key: string
    label: string
  }

  const allFields: SettingsField[] = [
    { tab: 'general', key: 'reopen_last_repo', label: 'reopen last repository launch' },
    { tab: 'general', key: 'theme', label: 'theme dark light' },
    { tab: 'general', key: 'locale', label: 'language locale' },
    { tab: 'general', key: 'confirm_before_delete', label: 'confirm before deleting' },
    { tab: 'general', key: 'sidebar_width', label: 'sidebar width' },
    { tab: 'general', key: 'type_badge_display', label: 'category type badges' },
    { tab: 'general', key: 'default_category_name', label: 'default category name' },
    { tab: 'general', key: 'inbox_recent_cards_limit', label: 'inbox recently updated card limit' },
    { tab: 'general', key: 'sidebar_collapse_default', label: 'sidebar collapsed collapse tree startup' },
    { tab: 'ai', key: 'accounts', label: 'ai accounts provider openai anthropic ollama api key model' },
    { tab: 'ai', key: 'ai_mode', label: 'ai mode chat edit card fields' },
    { tab: 'ai', key: 'auto_pin', label: 'auto pin behavior' },
    { tab: 'ai', key: 'min_confidence', label: 'minimum confidence ai suggestion pin threshold' },
    { tab: 'ai', key: 'context', label: 'ai context additional' },
    { tab: 'notifications', key: 'system_enabled', label: 'desktop system notifications' },
    { tab: 'notifications', key: 'smtp_host', label: 'email smtp host server' },
    { tab: 'notifications', key: 'smtp_to', label: 'email smtp recipient to address' },
    { tab: 'notifications', key: 'webhook_url', label: 'webhook url post' },
  ]

  let matchingKeys = $derived.by(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return null
    return new Set(
      allFields.filter(f => f.label.includes(q) || f.key.includes(q)).map(f => f.key)
    )
  })

  let matchingTabs = $derived.by(() => {
    if (!matchingKeys) return null
    return new Set(allFields.filter(f => matchingKeys!.has(f.key)).map(f => f.tab))
  })

  function fieldVisible(key: string): boolean {
    if (!matchingKeys) return true
    return matchingKeys.has(key)
  }

  function tabHasResults(tab: TabId): boolean {
    if (!matchingTabs) return true
    return matchingTabs.has(tab)
  }

  // Auto-switch to first matching tab when searching
  $effect(() => {
    if (matchingTabs && !matchingTabs.has(activeTab)) {
      const first = (['general', 'ai', 'notifications'] as TabId[]).find(t => matchingTabs!.has(t))
      if (first) activeTab = first
    }
  })

  const tabs: { id: TabId; labelKey: string }[] = [
    { id: 'general', labelKey: 'prefs.tab_general' },
    { id: 'ai', labelKey: 'prefs.tab_ai' },
    { id: 'notifications', labelKey: 'prefs.tab_notifications' },
  ]

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" onclick={handleOverlayClick}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('prefs.title')}</h2>
      <div class="search-box">
        <Search size={14} />
        <input
          bind:this={searchInputEl}
          bind:value={searchQuery}
          type="text"
          placeholder={t('prefs.search_placeholder')}
          class="search-input"
        />
        {#if searchQuery}
          <button class="search-clear" onclick={() => { searchQuery = ''; searchInputEl?.focus() }}><X size={12} /></button>
        {/if}
      </div>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="tab-bar">
      {#each tabs as tab}
        <button
          class="tab-btn"
          class:active={activeTab === tab.id}
          class:dimmed={!tabHasResults(tab.id)}
          onclick={() => activeTab = tab.id}
        >{t(tab.labelKey)}</button>
      {/each}
    </div>

    {#if loaded}
      <div class="dialog-body">
        <!-- GENERAL TAB -->
        {#if activeTab === 'general'}
          {#if fieldVisible('reopen_last_repo')}
            <label class="field toggle-field">
              <span class="field-label">{t('prefs.reopen_last_repo')}</span>
              <input type="checkbox" bind:checked={prefs.reopen_last_repo} />
            </label>
          {/if}

          {#if fieldVisible('theme')}
            <label class="field">
              <span class="field-label">{t('prefs.theme')}</span>
              <select bind:value={prefs.theme}>
                <option value="dark">{t('prefs.theme_dark')}</option>
                <option value="light">{t('prefs.theme_light')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('locale')}
            <label class="field">
              <span class="field-label">{t('prefs.locale')}</span>
              <select bind:value={prefs.locale}>
                {#each availableLocales() as loc}
                  <option value={loc}>{loc.toUpperCase()}</option>
                {/each}
              </select>
            </label>
          {/if}

          {#if fieldVisible('confirm_before_delete')}
            <label class="field toggle-field">
              <span class="field-label">{t('prefs.confirm_delete')}</span>
              <input type="checkbox" bind:checked={prefs.confirm_before_delete} />
            </label>
          {/if}

          {#if fieldVisible('sidebar_width')}
            <label class="field">
              <span class="field-label">{t('prefs.sidebar_width')}</span>
              <div class="range-row">
                <input type="range" min="160" max="500" step="10" bind:value={prefs.sidebar_width} />
                <span class="range-value">{prefs.sidebar_width}px</span>
              </div>
            </label>
          {/if}

          {#if fieldVisible('type_badge_display')}
            <label class="field">
              <span class="field-label">{t('prefs.type_badges')}</span>
              <select bind:value={prefs.type_badge_display}>
                <option value="text">{t('prefs.type_badge_text')}</option>
                <option value="color">{t('prefs.type_badge_color')}</option>
                <option value="hidden">{t('prefs.type_badge_hidden')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('default_category_name')}
            <label class="field">
              <span class="field-label">{t('prefs.default_category')}</span>
              <input
                type="text"
                bind:value={prefs.default_category_name}
                placeholder={t('prefs.default_category_placeholder')}
                class="text-input"
              />
              <span class="field-hint">{t('prefs.default_category_hint')}</span>
            </label>
          {/if}

          {#if fieldVisible('inbox_recent_cards_limit')}
            <label class="field">
              <span class="field-label">{t('prefs.inbox_recent_limit')}</span>
              <input
                type="number"
                min="1"
                max="100"
                bind:value={prefs.inbox_recent_cards_limit}
                class="text-input number-input"
              />
              <span class="field-hint">{t('prefs.inbox_recent_limit_hint')}</span>
            </label>
          {/if}

          {#if fieldVisible('inbox_activity_limit')}
            <label class="field">
              <span class="field-label">{t('prefs.inbox_activity_limit')}</span>
              <input
                type="number"
                min="1"
                max="500"
                bind:value={prefs.inbox_activity_limit}
                class="text-input number-input"
              />
              <span class="field-hint">{t('prefs.inbox_activity_limit_hint')}</span>
            </label>
          {/if}

          {#if fieldVisible('sidebar_collapse_default')}
            <label class="field toggle-field">
              <span class="field-label">{t('prefs.sidebar_collapse_default')}</span>
              <input type="checkbox" bind:checked={prefs.sidebar_collapse_default} />
            </label>
          {/if}
        {/if}

        <!-- AI TAB -->
        {#if activeTab === 'ai'}
          <!-- Accounts section -->
          <div class="field-section-label">{t('llm.accounts_title')}</div>
          <LLMAccountsManager bind:accounts={llmAccounts} />

          <!-- Behavior section -->
          <div class="field-section-label">{t('llm.behavior_title')}</div>

          {#if fieldVisible('ai_mode')}
            <label class="field">
              <span class="field-label">{t('llm.ai_mode')}</span>
              <select bind:value={llm.ai_mode}>
                <option value="edit">{t('llm.ai_mode_edit')}</option>
                <option value="suggest">{t('llm.ai_mode_suggest')}</option>
                <option value="chat">{t('llm.ai_mode_chat')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('auto_pin')}
            <label class="field">
              <span class="field-label">{t('llm.auto_pin')}</span>
              <select bind:value={llm.auto_pin}>
                <option value="off">{t('llm.auto_pin_off')}</option>
                <option value="suggest">{t('llm.auto_pin_suggest')}</option>
                <option value="auto">{t('llm.auto_pin_auto')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('min_confidence')}
            <label class="field">
              <span class="field-label">{t('llm.min_confidence')}</span>
              <select bind:value={llm.min_confidence}>
                <option value="">{t('llm.min_confidence_any')}</option>
                <option value="low">{t('llm.min_confidence_low')}</option>
                <option value="medium">{t('llm.min_confidence_medium')}</option>
                <option value="high">{t('llm.min_confidence_high')}</option>
              </select>
              <span class="field-hint">{t('llm.min_confidence_hint')}</span>
            </label>
          {/if}

          {#if fieldVisible('context')}
            <label class="field">
              <span class="field-label">{t('llm.context')}</span>
              <textarea rows="4" bind:value={llm.context} placeholder={t('llm.context_placeholder')}></textarea>
            </label>
          {/if}
        {/if}

        {#if activeTab === 'notifications'}
          <!-- System notifications -->
          <div class="field-row">
            <label class="field-label">{t('notifications.system_enabled')}</label>
            <div class="field-value">
              <label class="toggle"><input type="checkbox" bind:checked={notifCfg.system_enabled} /> {t('notifications.system_desc')}</label>
            </div>
          </div>
          <div class="field-row">
            <span class="field-label"></span>
            <div class="field-value">
              <button class="btn btn-outline btn-sm" onclick={testSystemNotif} disabled={testingSystem}>
                <Bell size={14} />
                {testingSystem ? '...' : t('notifications.test_system')}
              </button>
            </div>
          </div>

          <!-- Email SMTP -->
          <div class="field-section-label">{t('notifications.email_title')}</div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_host')}</label>
            <div class="field-value"><input type="text" class="field-input" bind:value={notifCfg.smtp_host} placeholder="smtp.gmail.com" /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_port')}</label>
            <div class="field-value"><input type="number" class="field-input field-input-short" bind:value={notifCfg.smtp_port} /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_username')}</label>
            <div class="field-value"><input type="text" class="field-input" bind:value={notifCfg.smtp_username} /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_password')}</label>
            <div class="field-value">
              <div class="key-row">
                <input type={showSmtpPassword ? 'text' : 'password'} class="field-input" bind:value={notifCfg.smtp_password} />
                <button class="btn-icon" onclick={() => showSmtpPassword = !showSmtpPassword}>
                  {#if showSmtpPassword}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                </button>
              </div>
            </div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_from')}</label>
            <div class="field-value"><input type="email" class="field-input" bind:value={notifCfg.smtp_from_addr} placeholder="bruv@example.com" /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_to')}</label>
            <div class="field-value"><input type="email" class="field-input" bind:value={notifCfg.smtp_to_addr} placeholder="you@example.com" /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.smtp_tls')}</label>
            <div class="field-value"><input type="checkbox" bind:checked={notifCfg.smtp_tls} /></div>
          </div>

          <!-- Webhook -->
          <div class="field-section-label">{t('notifications.webhook_title')}</div>
          <div class="field-row">
            <label class="field-label">{t('notifications.webhook_url')}</label>
            <div class="field-value"><input type="url" class="field-input" bind:value={notifCfg.webhook_url} placeholder="https://example.com/webhook" /></div>
          </div>
          <div class="field-row">
            <label class="field-label">{t('notifications.webhook_auth')}</label>
            <div class="field-value"><input type="text" class="field-input" bind:value={notifCfg.webhook_auth_header} placeholder="Bearer token..." /></div>
          </div>
        {/if}

      </div>

      <div class="dialog-footer">
        <button class="btn btn-ghost" onclick={onClose}>{t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={save}>{t('common.save')}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 540px;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
    white-space: nowrap;
  }

  .search-box {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-muted);
  }
  .search-box:focus-within {
    border-color: var(--accent);
  }

  .search-input {
    flex: 1;
    border: none !important;
    background: transparent;
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    min-width: 0;
  }
  .search-input::placeholder {
    color: var(--text-muted);
  }

  .search-clear {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0;
    display: flex;
    align-items: center;
  }
  .search-clear:hover { color: var(--text-primary); }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }
  .close-btn:hover { color: var(--text-primary); }

  .tab-bar {
    display: flex;
    gap: 0;
    padding: 0 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .tab-btn {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-secondary);
    font-size: 0.82rem;
    font-weight: 500;
    padding: 0.6rem 1rem;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .tab-btn:hover { color: var(--text-primary); }
  .tab-btn.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }
  .tab-btn.dimmed {
    opacity: 0.35;
  }

  .dialog-body {
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .toggle-field {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }

  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .field-section-label {
    grid-column: 1 / -1;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    margin-top: 0.5rem;
    padding-bottom: 0.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .field-input-short {
    max-width: 100px;
  }

  select, input[type="text"], input[type="password"], input[type="number"], input[type="email"], input[type="url"], .text-input, textarea {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
    box-sizing: border-box;
  }
  select:focus, input[type="text"]:focus, input[type="password"]:focus, input[type="number"]:focus, .text-input:focus, textarea:focus {
    border-color: var(--accent);
  }
  select:disabled, input:disabled, textarea:disabled {
    opacity: 0.5;
  }

  textarea {
    resize: vertical;
  }

  select {
    cursor: pointer;
  }

  .field-hint {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .number-input {
    width: 6rem;
  }

  input[type="checkbox"] {
    accent-color: var(--accent);
    width: 16px;
    height: 16px;
    cursor: pointer;
  }

  input[type="range"] {
    flex: 1;
    accent-color: var(--accent);
    cursor: pointer;
  }

  .range-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .range-value {
    font-size: 0.8rem;
    color: var(--text-muted);
    min-width: 48px;
    text-align: right;
  }

  .key-row {
    display: flex;
    gap: 4px;
  }
  .key-row input {
    flex: 1;
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .btn {
    padding: 0.45rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover { background: var(--accent-hover); }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
  }
  .btn-ghost:hover { color: var(--text-primary); }

  .btn-outline {
    background: transparent;
    color: var(--text-secondary);
    border: 1px solid var(--border);
  }
  .btn-outline:hover { border-color: var(--accent); color: var(--accent); }
  .btn-outline:disabled { opacity: 0.5; cursor: default; }
  .btn-sm { padding: 0.3rem 0.65rem; font-size: 0.8rem; }
</style>
