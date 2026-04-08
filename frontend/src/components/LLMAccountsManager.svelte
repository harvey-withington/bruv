<script lang="ts">
  import { Plus, Trash2, Star, Eye, EyeOff, Zap, ChevronDown, ChevronRight } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { TestLLMAccountConnection, SaveLLMAccounts } from '../lib/api'
  import type { LLMAccount } from '../lib/types'

  let { accounts = $bindable([]), onchange }: {
    accounts: LLMAccount[]
    onchange?: () => void
  } = $props()

  let expandedId = $state<string | null>(null)
  let showKeys = $state<Record<string, boolean>>({})
  let testingId = $state<string | null>(null)
  let testResults = $state<Record<string, { ok: boolean; message: string }>>({})
  let addingNew = $state(false)

  // New account draft
  let draft = $state<LLMAccount>({
    id: '',
    label: '',
    provider: 'openai',
    model: '',
    api_key: '',
    base_url: '',
    is_default: false,
  })

  function genId(): string {
    return Math.random().toString(36).slice(2, 10)
  }

  function resetDraft() {
    draft = { id: '', label: '', provider: 'openai', model: '', api_key: '', base_url: '', is_default: false }
    addingNew = false
  }

  function addAccount() {
    const acct: LLMAccount = { ...draft, id: genId(), is_default: accounts.length === 0 }
    accounts = [...accounts, acct]
    expandedId = acct.id
    resetDraft()
    onchange?.()
  }

  function removeAccount(id: string) {
    const wasDefault = accounts.find(a => a.id === id)?.is_default
    accounts = accounts.filter(a => a.id !== id)
    if (wasDefault && accounts.length > 0) {
      accounts[0].is_default = true
      accounts = [...accounts]
    }
    if (expandedId === id) expandedId = null
    onchange?.()
  }

  function setDefault(id: string) {
    accounts = accounts.map(a => ({ ...a, is_default: a.id === id }))
    onchange?.()
  }

  function updateAccount(id: string, field: keyof LLMAccount, value: string | boolean) {
    accounts = accounts.map(a => a.id === id ? { ...a, [field]: value } : a)
    onchange?.()
  }

  async function testAccount(id: string) {
    // Save first so backend has latest credentials
    testingId = id
    testResults = { ...testResults, [id]: undefined as unknown as { ok: boolean; message: string } }
    try {
      await SaveLLMAccounts(accounts)
      const model = await TestLLMAccountConnection(id)
      testResults = { ...testResults, [id]: { ok: true, message: `${t('llm.account_test_ok')} (${model})` } }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      testResults = { ...testResults, [id]: { ok: false, message: `${t('llm.account_test_fail')}: ${msg}` } }
    }
    testingId = null
  }

  function modelPlaceholder(provider: string): string {
    switch (provider) {
      case 'openai': return 'gpt-4o'
      case 'anthropic': return 'claude-sonnet-4-20250514'
      case 'ollama': return 'llama3'
      default: return ''
    }
  }

  function toggleExpand(id: string) {
    expandedId = expandedId === id ? null : id
  }
</script>

<div class="accounts-manager">
  {#if accounts.length === 0 && !addingNew}
    <p class="accounts-empty">{t('llm.accounts_empty')}</p>
  {/if}

  {#each accounts as acct (acct.id)}
    <div class="account-row" class:expanded={expandedId === acct.id}>
      <button class="account-header" onclick={() => toggleExpand(acct.id)}>
        <span class="expand-icon">
          {#if expandedId === acct.id}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
        </span>
        <span class="account-label">{acct.label || acct.provider}</span>
        <span class="account-provider-badge">{acct.provider}</span>
        {#if acct.is_default}
          <span class="default-badge"><Star size={10} /> {t('llm.account_default_badge')}</span>
        {/if}
        {#if acct.model}
          <span class="account-model-badge">{acct.model}</span>
        {/if}
      </button>

      {#if expandedId === acct.id}
        <div class="account-detail">
          <label class="acct-field">
            <span class="acct-field-label">{t('llm.account_label')}</span>
            <input type="text" value={acct.label} oninput={(e) => updateAccount(acct.id, 'label', (e.target as HTMLInputElement).value)} placeholder={t('llm.account_label_placeholder')} />
          </label>

          <label class="acct-field">
            <span class="acct-field-label">{t('llm.account_provider')}</span>
            <select value={acct.provider} onchange={(e) => updateAccount(acct.id, 'provider', (e.target as HTMLSelectElement).value)}>
              <option value="openai">{t('llm.provider_openai')}</option>
              <option value="anthropic">{t('llm.provider_anthropic')}</option>
              <option value="ollama">{t('llm.provider_ollama')}</option>
            </select>
          </label>

          <label class="acct-field">
            <span class="acct-field-label">{t('llm.account_model')}</span>
            <input type="text" value={acct.model} oninput={(e) => updateAccount(acct.id, 'model', (e.target as HTMLInputElement).value)} placeholder={modelPlaceholder(acct.provider)} />
          </label>

          {#if acct.provider !== 'ollama'}
            <label class="acct-field">
              <span class="acct-field-label">{t('llm.account_api_key')}</span>
              <div class="key-row">
                <input type={showKeys[acct.id] ? 'text' : 'password'} value={acct.api_key} oninput={(e) => updateAccount(acct.id, 'api_key', (e.target as HTMLInputElement).value)} placeholder="sk-..." />
                <button class="icon-btn" onclick={() => showKeys = { ...showKeys, [acct.id]: !showKeys[acct.id] }}>
                  {#if showKeys[acct.id]}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                </button>
              </div>
            </label>
          {/if}

          <label class="acct-field">
            <span class="acct-field-label">{t('llm.account_base_url')}</span>
            <input type="text" value={acct.base_url} oninput={(e) => updateAccount(acct.id, 'base_url', (e.target as HTMLInputElement).value)} placeholder={t('llm.base_url_placeholder')} />
          </label>

          <div class="account-actions">
            <button class="btn btn-sm btn-outline" onclick={() => testAccount(acct.id)} disabled={testingId === acct.id}>
              <Zap size={12} />
              {testingId === acct.id ? t('llm.account_testing') : t('llm.account_test')}
            </button>
            {#if testResults[acct.id]}
              <span class="test-result" class:success={testResults[acct.id].ok} class:error={!testResults[acct.id].ok}>
                {testResults[acct.id].message}
              </span>
            {/if}
            <div class="actions-right">
              {#if !acct.is_default}
                <button class="btn btn-sm btn-ghost" onclick={() => setDefault(acct.id)}>
                  <Star size={12} /> {t('llm.account_set_default')}
                </button>
              {/if}
              <button class="btn btn-sm btn-danger" onclick={() => removeAccount(acct.id)}>
                <Trash2 size={12} /> {t('llm.account_delete')}
              </button>
            </div>
          </div>
        </div>
      {/if}
    </div>
  {/each}

  {#if addingNew}
    <div class="account-row expanded new-account">
      <div class="account-detail">
        <label class="acct-field">
          <span class="acct-field-label">{t('llm.account_label')}</span>
          <input type="text" bind:value={draft.label} placeholder={t('llm.account_label_placeholder')} />
        </label>
        <label class="acct-field">
          <span class="acct-field-label">{t('llm.account_provider')}</span>
          <select bind:value={draft.provider}>
            <option value="openai">{t('llm.provider_openai')}</option>
            <option value="anthropic">{t('llm.provider_anthropic')}</option>
            <option value="ollama">{t('llm.provider_ollama')}</option>
          </select>
        </label>
        <label class="acct-field">
          <span class="acct-field-label">{t('llm.account_model')}</span>
          <input type="text" bind:value={draft.model} placeholder={modelPlaceholder(draft.provider)} />
        </label>
        {#if draft.provider !== 'ollama'}
          <label class="acct-field">
            <span class="acct-field-label">{t('llm.account_api_key')}</span>
            <input type="password" bind:value={draft.api_key} placeholder="sk-..." />
          </label>
        {/if}
        <label class="acct-field">
          <span class="acct-field-label">{t('llm.account_base_url')}</span>
          <input type="text" bind:value={draft.base_url} placeholder={t('llm.base_url_placeholder')} />
        </label>
        <div class="account-actions">
          <button class="btn btn-sm btn-primary" onclick={addAccount} disabled={!draft.label && !draft.provider}>
            {t('llm.account_add')}
          </button>
          <button class="btn btn-sm btn-ghost" onclick={resetDraft}>{t('common.cancel')}</button>
        </div>
      </div>
    </div>
  {/if}

  <button class="add-btn" onclick={() => { addingNew = true }}>
    <Plus size={14} /> {t('llm.account_add')}
  </button>
</div>

<style>
  .accounts-manager {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .accounts-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0;
  }

  .account-row {
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    transition: border-color 0.15s;
  }
  .account-row.expanded {
    border-color: color-mix(in srgb, var(--accent) 40%, var(--border));
  }

  .account-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.5rem 0.65rem;
    width: 100%;
    background: none;
    border: none;
    color: var(--text-body);
    cursor: pointer;
    font-size: 0.82rem;
    text-align: left;
    transition: background 0.1s;
  }
  .account-header:hover { background: var(--bg-hover); }

  .expand-icon { color: var(--text-muted); display: flex; }
  .account-label { font-weight: 500; }

  .account-provider-badge {
    font-size: 0.65rem;
    padding: 0.05rem 0.35rem;
    border-radius: 3px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.03em;
    font-weight: 600;
  }

  .default-badge {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    font-size: 0.6rem;
    padding: 0.05rem 0.35rem;
    border-radius: 3px;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
    font-weight: 600;
    text-transform: uppercase;
  }

  .account-model-badge {
    font-size: 0.7rem;
    color: var(--text-muted);
    margin-left: auto;
    font-family: monospace;
  }

  .account-detail {
    padding: 0.65rem;
    border-top: 1px solid var(--border-muted);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    background: var(--bg-elevated);
  }

  .acct-field {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .acct-field-label {
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .key-row {
    display: flex;
    gap: 4px;
  }
  .key-row input { flex: 1; }

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

  .account-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-top: 0.25rem;
  }

  .actions-right {
    display: flex;
    gap: 0.4rem;
    margin-left: auto;
  }

  .test-result { font-size: 0.75rem; }
  .test-result.success { color: var(--success, #22c55e); }
  .test-result.error { color: var(--danger, #ef4444); }

  .btn { border: none; cursor: pointer; border-radius: 5px; font-size: 0.75rem; font-weight: 500; display: flex; align-items: center; gap: 0.25rem; }
  .btn-sm { padding: 0.2rem 0.5rem; }
  .btn-primary { background: var(--accent); color: white; }
  .btn-primary:hover { filter: brightness(1.1); }
  .btn-primary:disabled { opacity: 0.5; }
  .btn-outline { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
  .btn-outline:hover { border-color: var(--accent); color: var(--accent); }
  .btn-outline:disabled { opacity: 0.5; }
  .btn-ghost { background: transparent; color: var(--text-secondary); }
  .btn-ghost:hover { color: var(--text-primary); }
  .btn-danger { background: transparent; color: var(--danger, #ef4444); }
  .btn-danger:hover { background: color-mix(in srgb, var(--danger, #ef4444) 10%, transparent); }

  .add-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.35rem 0.65rem;
    background: none;
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s;
  }
  .add-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }

  input, select {
    padding: 0.35rem 0.5rem;
    border-radius: 5px;
    border: 1px solid var(--border);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.82rem;
    font-family: inherit;
    outline: none;
  }
  input:focus, select:focus { border-color: var(--accent); }
</style>
