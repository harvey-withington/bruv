<script lang="ts">
  // The models set up on one provider: label, provider model id, tier,
  // tool support, enabled, plus Test / delete per row and Find / Add at
  // the foot. Edits are staged into `routing` like every other Settings
  // change; Test and Find save first so the backend sees the draft.
  import { Plus, Search, Trash2, TriangleAlert, Zap } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { TestLLMModel } from '@shared/api'
  import type { DiscoveredModel, LLMAccount, LLMModel, LLMRouting, ModelTier } from '@shared/types'
  import { MODEL_TIERS, modelDisplayLabel, removeModel } from '@shared/modelRefs'
  import { defaultModelForProvider } from '@shared/llmDefaults'
  import ModelDiscoveryPanel from './ModelDiscoveryPanel.svelte'

  let { account, routing = $bindable(), persist, autoDiscover = false }: {
    account: LLMAccount
    routing: LLMRouting
    persist: () => Promise<void>
    /** Open Find models straight away (a provider that was just added). */
    autoDiscover?: boolean
  } = $props()

  let models = $derived(routing.models.filter(m => m.account_id === account.id))
  let existingNames = $derived(new Set(models.map(m => m.name)))
  // Initial-value capture is intended: only a fresh mount auto-discovers.
  /* svelte-ignore state_referenced_locally */
  let discovering = $state(autoDiscover)
  let testingId = $state<string | null>(null)
  let testResults = $state<Record<string, { ok: boolean; message: string }>>({})

  function newId(): string {
    return crypto.randomUUID().slice(0, 8)
  }

  function update(id: string, patch: Partial<LLMModel>) {
    routing = { ...routing, models: routing.models.map(m => m.id === id ? { ...m, ...patch } : m) }
  }

  function addModels(entries: Pick<LLMModel, 'name' | 'label' | 'tier'>[]) {
    const added: LLMModel[] = entries.map(e => ({
      id: newId(),
      account_id: account.id,
      name: e.name,
      label: e.label,
      tier: e.tier,
      supports_tools: true,
      enabled: true,
    }))
    // The first model ever set up becomes the default, so AI works
    // without a trip to the Use-for table.
    const firstEver = routing.models.length === 0 && added.length > 0
    routing = {
      ...routing,
      models: [...routing.models, ...added],
      default: firstEver ? `model:${added[0].id}` : routing.default,
    }
  }

  function addManual() {
    addModels([{ name: models.length === 0 ? defaultModelForProvider(account.provider) : '', label: '', tier: 'balanced' }])
  }

  function addDiscovered(found: DiscoveredModel[]) {
    addModels(found.map(m => ({ name: m.id, label: m.label ?? '', tier: m.tier })))
    discovering = false
  }

  async function remove(m: LLMModel) {
    if (!await showConfirm(t('llm.model_delete_confirm', { label: modelDisplayLabel(m) }))) return
    routing = removeModel(routing, m.id)
  }

  async function test(id: string) {
    testingId = id
    const { [id]: _, ...rest } = testResults
    testResults = rest
    try {
      await persist()
      const echoed = await TestLLMModel(id)
      testResults = { ...testResults, [id]: { ok: true, message: t('llm.model_test_ok', { model: echoed }) } }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      testResults = { ...testResults, [id]: { ok: false, message: t('llm.model_test_fail', { error: msg }) } }
    }
    testingId = null
  }
</script>

<div class="models">
  <div class="models-title">{t('llm.models_title')}</div>
  {#if models.length === 0 && !discovering}
    <div class="models-empty" role="status">
      <TriangleAlert size={14} />
      <span>{t('llm.models_empty')}</span>
    </div>
  {/if}

  {#each models as m (m.id)}
    <div class="model-row" class:disabled={!m.enabled}>
      <div class="model-line">
        <input type="checkbox" checked={m.enabled} onchange={(e) => update(m.id, { enabled: (e.target as HTMLInputElement).checked })} title={t('llm.model_enabled')} />
        <input class="model-label" type="text" value={m.label ?? ''} oninput={(e) => update(m.id, { label: (e.target as HTMLInputElement).value })} placeholder={t('llm.model_label_placeholder')} />
        <input class="model-name" type="text" value={m.name} oninput={(e) => update(m.id, { name: (e.target as HTMLInputElement).value.trim() })} placeholder={t('llm.model_name_placeholder')} />
      </div>
      <div class="model-line">
        <select value={m.tier} onchange={(e) => update(m.id, { tier: (e.target as HTMLSelectElement).value as ModelTier })} title={t('llm.model_tier_hint')}>
          {#each MODEL_TIERS as tier (tier)}
            <option value={tier}>{t(`llm_routing.tier_${tier}`)}</option>
          {/each}
        </select>
        <label class="model-tools" title={t('llm.model_tools_hint')}>
          <input type="checkbox" checked={m.supports_tools} onchange={(e) => update(m.id, { supports_tools: (e.target as HTMLInputElement).checked })} />
          {t('llm.model_tools')}
        </label>
        <button class="btn subtle" onclick={() => test(m.id)} disabled={testingId === m.id || !m.name}>
          <Zap size={12} /> {testingId === m.id ? t('llm.account_testing') : t('llm.account_test')}
        </button>
        {#if testResults[m.id]}
          <span class="test-result" class:ok={testResults[m.id].ok}>{testResults[m.id].message}</span>
        {/if}
        <button class="btn subtle model-delete" onclick={() => remove(m)} title={t('llm.model_delete_hint')}>
          <Trash2 size={12} /> {t('common.delete')}
        </button>
      </div>
    </div>
  {/each}

  {#if discovering}
    <ModelDiscoveryPanel accountId={account.id} {existingNames} {persist} onadd={addDiscovered} oncancel={() => discovering = false} />
  {:else}
    <div class="models-actions">
      <button class="btn" onclick={() => discovering = true}><Search size={12} /> {t('llm.discover')}</button>
      <button class="btn subtle" onclick={addManual}><Plus size={12} /> {t('llm.model_add')}</button>
    </div>
  {/if}
</div>

<style>
  .models {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--border-muted);
  }
  .models-title {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-secondary);
  }
  .models-empty {
    display: flex;
    align-items: flex-start;
    gap: 0.4rem;
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--warning-border);
    border-radius: 6px;
    background: color-mix(in srgb, var(--warning) 10%, transparent);
    font-size: 0.78rem;
    color: var(--text-primary);
  }
  .models-empty :global(svg) { flex-shrink: 0; margin-top: 1px; color: var(--warning); }
  .model-row {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.4rem;
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    background: var(--bg-surface);
  }
  .model-row.disabled .model-label,
  .model-row.disabled .model-name { opacity: 0.55; }
  .model-line {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-wrap: wrap;
  }
  .model-label { flex: 1 1 8rem; min-width: 0; }
  .model-name { flex: 1 1 10rem; min-width: 0; font-family: monospace; }
  .model-tools {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.75rem;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .model-delete { margin-left: auto; }
  .model-delete:hover { color: var(--danger); }
  .test-result { font-size: 0.72rem; color: var(--danger); }
  .test-result.ok { color: var(--success); }
  .models-actions { display: flex; gap: 0.4rem; }
  input[type='text'], select {
    padding: 0.3rem 0.45rem;
    border-radius: 5px;
    border: 1px solid var(--border);
    background: var(--bg-base);
    color: var(--text-primary);
    font-size: 0.8rem;
    font-family: inherit;
    outline: none;
  }
  .model-name { font-family: monospace; }
  input:focus, select:focus { border-color: var(--accent); }
</style>
