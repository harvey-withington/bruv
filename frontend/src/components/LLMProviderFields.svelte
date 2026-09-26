<script lang="ts">
  // The connection fields of one AI provider: label, kind, key, base URL.
  // Used for existing providers and for the new-provider draft.
  import { Eye, EyeOff } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import type { LLMAccount, LLMProviderKind } from '@shared/types'

  type EditableField = 'label' | 'provider' | 'api_key' | 'base_url'

  let { account, onupdate }: {
    account: LLMAccount
    onupdate: (field: EditableField, value: string) => void
  } = $props()

  const KINDS: LLMProviderKind[] = ['anthropic', 'openai', 'ollama', 'openai_compatible']

  let showKey = $state(false)
  let needsKey = $derived(account.provider !== 'ollama')
  let baseUrlRequired = $derived(account.provider === 'openai_compatible')

  function input(field: EditableField) {
    return (e: Event) => onupdate(field, (e.target as HTMLInputElement | HTMLSelectElement).value)
  }
</script>

<label class="acct-field">
  <span class="acct-field-label">{t('llm.account_label')}</span>
  <input type="text" value={account.label} oninput={input('label')} placeholder={t('llm.account_label_placeholder')} />
</label>

<label class="acct-field">
  <span class="acct-field-label">{t('llm.account_provider')}</span>
  <select value={account.provider} onchange={input('provider')}>
    {#each KINDS as kind (kind)}
      <option value={kind}>{t(`llm.provider_${kind}`)}</option>
    {/each}
  </select>
</label>

{#if needsKey}
  <label class="acct-field">
    <span class="acct-field-label">
      {t('llm.account_api_key')}{baseUrlRequired ? ` ${t('llm.optional_suffix')}` : ''}
    </span>
    <div class="key-row">
      <input type={showKey ? 'text' : 'password'} value={account.api_key} oninput={input('api_key')} placeholder={t('llm.api_key_placeholder')} />
      <button class="icon-btn" onclick={() => showKey = !showKey} title={showKey ? t('llm.hide_key') : t('llm.show_key')}>
        {#if showKey}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
      </button>
    </div>
  </label>
{/if}

<label class="acct-field">
  <span class="acct-field-label">{t('llm.account_base_url')}</span>
  <input
    type="text"
    value={account.base_url}
    oninput={input('base_url')}
    placeholder={baseUrlRequired ? t('llm.base_url_compatible_placeholder') : t('llm.base_url_placeholder')}
  />
  {#if baseUrlRequired}
    <span class="acct-field-hint">{t('llm.base_url_compatible_hint')}</span>
  {/if}
</label>

<style>
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
  .acct-field-hint {
    font-size: 0.7rem;
    color: var(--text-muted);
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
