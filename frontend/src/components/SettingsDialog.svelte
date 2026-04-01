<script lang="ts">
  import { X, Eye, EyeOff, Zap, Search } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { GetPreferences, SetPreferences, GetLLMConfig, SetLLMConfig, TestLLMConnection } from '../lib/api'
  import { theme, setTheme } from '../lib/theme.svelte'
  import { setLocale, availableLocales } from '../lib/i18n.svelte'
  import { nav, prefs as prefsStore } from '../lib/store.svelte'
  import { draggable } from '../lib/draggable'
  import { focusTrap } from '../lib/actions'

  type TabId = 'general' | 'ai'

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
    type_badge_display: 'text' as 'text' | 'color' | 'hidden',
    default_category_name: 'Ideas',
  })

  // --- AI / LLM ---
  let llm = $state({
    context: '',
    provider: '',
    model: '',
    api_key: '',
    base_url: '',
    auto_pin: 'off',
  })
  let showKey = $state(false)
  let testing = $state(false)
  let testResult = $state<{ ok: boolean; message: string } | null>(null)

  let loaded = $state(false)

  $effect(() => { loadAll() })

  async function loadAll() {
    try {
      const [p, c] = await Promise.all([
        GetPreferences(),
        GetLLMConfig(),
      ])
      if (p) {
        prefs.reopen_last_repo = p.reopen_last_repo ?? false
        prefs.theme = p.theme || 'dark'
        prefs.locale = p.locale || 'en'
        prefs.confirm_before_delete = p.confirm_before_delete ?? true
        prefs.sidebar_width = nav.sidebarWidth
        prefs.type_badge_display = (p.type_badge_display || 'text') as 'text' | 'color' | 'hidden'
        prefs.default_category_name = p.default_category_name || 'Ideas'
      }
      if (c) {
        llm.context = c.context || ''
        llm.provider = c.provider || ''
        llm.model = c.model || ''
        llm.api_key = c.api_key || ''
        llm.base_url = c.base_url || ''
        llm.auto_pin = c.auto_pin || 'off'
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await Promise.all([
        SetPreferences(prefs),
        SetLLMConfig(llm),
      ])
      setTheme(prefs.theme as 'dark' | 'light')
      setLocale(prefs.locale)
      nav.sidebarWidth = prefs.sidebar_width
      localStorage.setItem('bruv-sidebar-width', String(prefs.sidebar_width))
      prefsStore.typeBadgeDisplay = prefs.type_badge_display
      onClose()
    } catch (e) { console.error('Settings save error:', e) }
  }

  async function testConnection() {
    testing = true
    testResult = null
    try {
      await SetLLMConfig(llm)
      const model = await TestLLMConnection()
      testResult = { ok: true, message: `${t('llm.test_success')} (${model})` }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      testResult = { ok: false, message: `${t('llm.test_failed')}: ${msg}` }
    }
    testing = false
  }

  function modelPlaceholder(provider: string): string {
    switch (provider) {
      case 'openai': return 'gpt-4o'
      case 'anthropic': return 'claude-sonnet-4-20250514'
      case 'ollama': return 'llama3'
      default: return t('llm.model_placeholder')
    }
  }

  function baseUrlPlaceholder(provider: string): string {
    switch (provider) {
      case 'openai': return 'https://api.openai.com/v1'
      case 'anthropic': return 'https://api.anthropic.com'
      case 'ollama': return 'http://localhost:11434'
      default: return t('llm.base_url_placeholder')
    }
  }

  let llmDisabled = $derived(!llm.provider)

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
    { tab: 'ai', key: 'provider', label: 'ai provider openai anthropic ollama' },
    { tab: 'ai', key: 'model', label: 'ai model' },
    { tab: 'ai', key: 'api_key', label: 'api key' },
    { tab: 'ai', key: 'base_url', label: 'base url endpoint' },
    { tab: 'ai', key: 'auto_pin', label: 'auto pin behavior' },
    { tab: 'ai', key: 'context', label: 'ai context additional' },
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
      const first = (['general', 'ai'] as TabId[]).find(t => matchingTabs!.has(t))
      if (first) activeTab = first
    }
  })

  const tabs: { id: TabId; labelKey: string }[] = [
    { id: 'general', labelKey: 'prefs.tab_general' },
    { id: 'ai', labelKey: 'prefs.tab_ai' },
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
        {/if}

        <!-- AI TAB -->
        {#if activeTab === 'ai'}
          {#if fieldVisible('provider')}
            <label class="field">
              <span class="field-label">{t('llm.provider')}</span>
              <select bind:value={llm.provider} onchange={() => { testResult = null }}>
                <option value="">{t('llm.provider_none')}</option>
                <option value="openai">{t('llm.provider_openai')}</option>
                <option value="anthropic">{t('llm.provider_anthropic')}</option>
                <option value="ollama">{t('llm.provider_ollama')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('model')}
            <label class="field">
              <span class="field-label">{t('llm.model')}</span>
              <input type="text" bind:value={llm.model} placeholder={modelPlaceholder(llm.provider)} disabled={llmDisabled} />
            </label>
          {/if}

          {#if fieldVisible('api_key') && llm.provider !== 'ollama'}
            <label class="field">
              <span class="field-label">{t('llm.api_key')}</span>
              <div class="key-row">
                <input type={showKey ? 'text' : 'password'} bind:value={llm.api_key} placeholder={t('llm.api_key_placeholder')} disabled={llmDisabled} />
                <button class="icon-btn" onclick={() => showKey = !showKey} title={showKey ? 'Hide' : 'Show'} disabled={llmDisabled}>
                  {#if showKey}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                </button>
              </div>
            </label>
          {/if}

          {#if fieldVisible('base_url')}
            <label class="field">
              <span class="field-label">{t('llm.base_url')}</span>
              <input type="text" bind:value={llm.base_url} placeholder={baseUrlPlaceholder(llm.provider)} disabled={llmDisabled} />
            </label>
          {/if}

          {#if fieldVisible('auto_pin')}
            <label class="field">
              <span class="field-label">{t('llm.auto_pin')}</span>
              <select bind:value={llm.auto_pin} disabled={llmDisabled}>
                <option value="off">{t('llm.auto_pin_off')}</option>
                <option value="suggest">{t('llm.auto_pin_suggest')}</option>
                <option value="auto">{t('llm.auto_pin_auto')}</option>
              </select>
            </label>
          {/if}

          {#if fieldVisible('context')}
            <label class="field">
              <span class="field-label">{t('llm.context')}</span>
              <textarea rows="4" bind:value={llm.context} placeholder={t('llm.context_placeholder')}></textarea>
            </label>
          {/if}

          {#if llm.provider}
            <div class="test-row">
              <button class="btn btn-outline" onclick={testConnection} disabled={testing}>
                <Zap size={14} />
                {testing ? 'Testing...' : t('llm.test_connection')}
              </button>
              {#if testResult}
                <span class="test-result" class:success={testResult.ok} class:error={!testResult.ok}>
                  {testResult.message}
                </span>
              {/if}
            </div>
          {/if}
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
    padding: 0.3rem 0.6rem;
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
    border: none;
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

  select, input[type="text"], input[type="password"], .text-input, textarea {
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
  select:focus, input[type="text"]:focus, input[type="password"]:focus, .text-input:focus, textarea:focus {
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

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    flex-shrink: 0;
  }
  .icon-btn:hover { color: var(--text-primary); }
  .icon-btn:disabled { opacity: 0.4; cursor: default; }

  .test-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .test-result {
    font-size: 0.8rem;
  }
  .test-result.success { color: var(--success); }
  .test-result.error { color: var(--danger, #ef4444); }

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
</style>
