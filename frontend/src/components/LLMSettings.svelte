<script lang="ts">
  import { X, Eye, EyeOff, Zap } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { GetLLMConfig, SetLLMConfig, TestLLMConnection } from '../lib/api'
  import { draggable } from '../lib/draggable'

  let { onClose }: { onClose: () => void } = $props()

  let config = $state({
    context: '',
    provider: '',
    model: '',
    api_key: '',
    base_url: '',
    auto_pin: 'off',
  })
  let loaded = $state(false)
  let saved = $state(false)
  let showKey = $state(false)
  let testing = $state(false)
  let testResult = $state<{ ok: boolean; message: string } | null>(null)

  $effect(() => { load() })

  async function load() {
    try {
      const c = await GetLLMConfig()
      if (c) {
        config.context = c.context || ''
        config.provider = c.provider || ''
        config.model = c.model || ''
        config.api_key = c.api_key || ''
        config.base_url = c.base_url || ''
        config.auto_pin = c.auto_pin || 'off'
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await SetLLMConfig(config)
      onClose()
    } catch (e) { console.error('SetLLMConfig:', e) }
  }

  async function testConnection() {
    testing = true
    testResult = null
    try {
      // Save first so backend reads the latest config
      await SetLLMConfig(config)
      const model = await TestLLMConnection()
      testResult = { ok: true, message: `${t('llm.test_success')} (${model})` }
    } catch (e: any) {
      testResult = { ok: false, message: `${t('llm.test_failed')}: ${e?.message || e}` }
    }
    testing = false
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
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

  let isDisabled = $derived(!config.provider)
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" onclick={handleOverlayClick}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }}>
    <div class="dialog-header">
      <h2>{t('llm.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    {#if loaded}
      <div class="dialog-body">
        <label class="field">
          <span class="field-label">{t('llm.provider')}</span>
          <select bind:value={config.provider} onchange={() => { testResult = null }}>
            <option value="">{t('llm.provider_none')}</option>
            <option value="openai">{t('llm.provider_openai')}</option>
            <option value="anthropic">{t('llm.provider_anthropic')}</option>
            <option value="ollama">{t('llm.provider_ollama')}</option>
          </select>
        </label>

        <label class="field">
          <span class="field-label">{t('llm.model')}</span>
          <input type="text" bind:value={config.model} placeholder={modelPlaceholder(config.provider)} disabled={isDisabled} />
        </label>

        {#if config.provider !== 'ollama'}
          <label class="field">
            <span class="field-label">{t('llm.api_key')}</span>
            <div class="key-row">
              <input type={showKey ? 'text' : 'password'} bind:value={config.api_key} placeholder={t('llm.api_key_placeholder')} disabled={isDisabled} />
              <button class="icon-btn" onclick={() => showKey = !showKey} title={showKey ? 'Hide' : 'Show'} disabled={isDisabled}>
                {#if showKey}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
              </button>
            </div>
          </label>
        {/if}

        <label class="field">
          <span class="field-label">{t('llm.base_url')}</span>
          <input type="text" bind:value={config.base_url} placeholder={baseUrlPlaceholder(config.provider)} disabled={isDisabled} />
        </label>

        <label class="field">
          <span class="field-label">{t('llm.auto_pin')}</span>
          <select bind:value={config.auto_pin} disabled={isDisabled}>
            <option value="off">{t('llm.auto_pin_off')}</option>
            <option value="suggest">{t('llm.auto_pin_suggest')}</option>
            <option value="auto">{t('llm.auto_pin_auto')}</option>
          </select>
        </label>

        <label class="field">
          <span class="field-label">{t('llm.context')}</span>
          <textarea rows="4" bind:value={config.context} placeholder={t('llm.context_placeholder')}></textarea>
        </label>

        {#if config.provider}
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
      </div>

      <div class="dialog-footer">
        {#if saved}<span class="saved-msg">{t('llm.saved')}</span>{/if}
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
    width: 520px;
    max-height: 85vh;
    overflow-y: auto;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .close-btn:hover { color: var(--text-primary); }

  .dialog-body {
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .field-label {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  input, select, textarea {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
  }
  input:focus, select:focus, textarea:focus {
    border-color: var(--accent);
  }
  input:disabled, select:disabled, textarea:disabled {
    opacity: 0.5;
  }

  textarea {
    resize: vertical;
  }

  select {
    cursor: pointer;
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
  }

  .saved-msg {
    font-size: 0.8rem;
    color: var(--success);
    margin-right: auto;
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
