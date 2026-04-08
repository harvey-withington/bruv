<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import type { LLMAccount } from '../lib/types'

  let {
    accounts,
    selectedAccountId = $bindable(''),
    selectedModel = $bindable(''),
    onchange,
  }: {
    accounts: LLMAccount[]
    selectedAccountId: string
    selectedModel: string
    onchange?: (accountId: string, model: string) => void
  } = $props()

  let defaultAccount = $derived(accounts.find(a => a.is_default))

  function handleAccountChange(e: Event) {
    selectedAccountId = (e.target as HTMLSelectElement).value
    onchange?.(selectedAccountId, selectedModel)
  }

  function handleModelInput(e: Event) {
    selectedModel = (e.target as HTMLInputElement).value
    onchange?.(selectedAccountId, selectedModel)
  }

  function modelPlaceholder(): string {
    const acct = selectedAccountId
      ? accounts.find(a => a.id === selectedAccountId)
      : defaultAccount
    if (acct?.model) return acct.model
    switch (acct?.provider) {
      case 'openai': return 'gpt-4o'
      case 'anthropic': return 'claude-sonnet-4-20250514'
      case 'ollama': return 'llama3'
      default: return t('agent.llm_model_placeholder')
    }
  }
</script>

<div class="account-select">
  {#if accounts.length === 0}
    <span class="no-accounts">{t('agent.llm_account_none')}</span>
  {:else}
    <select class="agent-select" value={selectedAccountId} onchange={handleAccountChange}>
      <option value="">
        {defaultAccount ? t('agent.llm_account_default', { label: defaultAccount.label || defaultAccount.provider }) : '—'}
      </option>
      {#each accounts.filter(a => !a.is_default) as acct (acct.id)}
        <option value={acct.id}>{acct.label || acct.provider}</option>
      {/each}
    </select>
    <input
      type="text"
      class="model-input"
      value={selectedModel}
      oninput={handleModelInput}
      placeholder={modelPlaceholder()}
    />
  {/if}
</div>

<style>
  .account-select {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .no-accounts {
    font-size: 0.75rem;
    color: var(--text-muted);
    font-style: italic;
  }

  .agent-select {
    flex: 1;
    min-width: 0;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: inherit;
    cursor: pointer;
  }
  .agent-select:focus { outline: none; border-color: var(--accent); }

  .model-input {
    flex: 1;
    min-width: 0;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: monospace;
  }
  .model-input:focus { outline: none; border-color: var(--accent); }
  .model-input::placeholder { font-family: monospace; color: var(--text-muted); }
</style>
