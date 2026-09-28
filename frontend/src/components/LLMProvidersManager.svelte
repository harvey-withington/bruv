<script lang="ts">
  // Settings → AI → Providers. Each provider is a connection (kind, key,
  // base URL) that expands to its connection fields and its models.
  // Provider order is preference order: routers picking "the first fast
  // model" walk providers top to bottom, so rows drag to reorder.
  // Everything is staged into the bound arrays and saved with the dialog.
  import { Plus, Trash2, ChevronDown, ChevronRight, GripVertical, TriangleAlert } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { RowReorder } from '../lib/rowReorder.svelte'
  import type { LLMAccount, LLMRouting } from '@shared/types'
  import { accountDisplayLabel, removeModel } from '@shared/modelRefs'
  import LLMProviderFields from './LLMProviderFields.svelte'
  import LLMModelsList from './LLMModelsList.svelte'

  let { accounts = $bindable([]), routing = $bindable(), persist }: {
    accounts: LLMAccount[]
    routing: LLMRouting
    /** Saves the settings being edited (Test / Find models need them on the backend). */
    persist: () => Promise<void>
  } = $props()

  let expandedId = $state<string | null>(null)
  let draft = $state<LLMAccount | null>(null)
  // A provider without models does nothing, so a just-added one opens
  // straight into Find models.
  let justAddedId = $state<string | null>(null)

  const drag = new RowReorder(() => accounts, next => { accounts = next })

  function emptyDraft(): LLMAccount {
    return { id: '', label: '', provider: 'anthropic', api_key: '', base_url: '' }
  }

  function addProvider() {
    if (!draft) return
    const acct: LLMAccount = { ...draft, id: crypto.randomUUID().slice(0, 8) }
    accounts = [...accounts, acct]
    expandedId = acct.id
    justAddedId = acct.id
    draft = null
  }

  function update(id: string, field: keyof LLMAccount, value: string) {
    accounts = accounts.map(a => a.id === id ? { ...a, [field]: value } : a)
  }

  async function removeProvider(acct: LLMAccount) {
    const count = routing.models.filter(m => m.account_id === acct.id).length
    if (!await showConfirm(t('llm.provider_delete_confirm', { label: accountDisplayLabel(acct), count }))) return
    let next = routing
    for (const m of routing.models.filter(m => m.account_id === acct.id)) next = removeModel(next, m.id)
    routing = next
    accounts = accounts.filter(a => a.id !== acct.id)
    if (expandedId === acct.id) expandedId = null
  }

  function modelCount(id: string): number {
    return routing.models.filter(m => m.account_id === id).length
  }
</script>

<div class="providers">
  {#if accounts.length === 0 && !draft}
    <p class="providers-empty">{t('llm.accounts_empty')}</p>
  {/if}

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="providers-list" role="list" ondrop={drag.drop} ondragover={drag.containerOver}>
    {#each accounts as acct, idx (acct.id)}
      {#if drag.showsIndicatorBefore(acct.id)}<div class="drop-indicator"></div>{/if}
      <div
        class="provider-row"
        class:expanded={expandedId === acct.id}
        class:dragging={drag.draggingId === acct.id}
        role="listitem"
        ondragover={(e) => drag.over(e, acct.id, idx)}
      >
        <div class="provider-header-row">
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <span
            class="drag-handle"
            draggable={true}
            ondragstart={(e) => drag.start(e, acct.id)}
            ondragend={drag.end}
            role="button"
            tabindex="-1"
            aria-label={t('tooltip.drag_llm_account')}
            title={t('tooltip.drag_llm_account')}
          ><GripVertical size={14} /></span>
          <button class="provider-header" onclick={() => expandedId = expandedId === acct.id ? null : acct.id}>
            <span class="expand-icon">
              {#if expandedId === acct.id}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
            </span>
            <span class="provider-label">{accountDisplayLabel(acct)}</span>
            <span class="provider-badge">{t(`llm.provider_${acct.provider}`)}</span>
            {#if modelCount(acct.id) === 0}
              <span class="provider-warning" title={t('llm.provider_no_models_hint')}><TriangleAlert size={12} /> {t('llm.provider_no_models')}</span>
            {:else}
              <span class="provider-count">{t('llm.model_count', { count: modelCount(acct.id) })}</span>
            {/if}
          </button>
        </div>

        {#if expandedId === acct.id}
          <div class="provider-detail">
            <LLMProviderFields account={acct} onupdate={(field, value) => update(acct.id, field, value)} />
            <LLMModelsList account={acct} bind:routing {persist} autoDiscover={justAddedId === acct.id} />
            <div class="provider-actions">
              <button class="btn subtle provider-delete" onclick={() => removeProvider(acct)} title={t('llm.provider_delete_hint')}>
                <Trash2 size={12} /> {t('common.delete')}
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/each}
    {#if drag.showsIndicatorAtEnd()}<div class="drop-indicator"></div>{/if}
  </div>

  {#if draft}
    <div class="provider-row expanded">
      <div class="provider-detail">
        <LLMProviderFields account={draft} onupdate={(field, value) => { draft = { ...draft!, [field]: value } }} />
        <div class="provider-actions">
          <button class="btn primary" onclick={addProvider}>{t('common.add')}</button>
          <button class="btn subtle" onclick={() => draft = null}>{t('common.cancel')}</button>
        </div>
      </div>
    </div>
  {:else}
    <button class="add-btn" onclick={() => draft = emptyDraft()}>
      <Plus size={14} /> {t('llm.provider_add')}
    </button>
  {/if}
</div>

<style>
  .providers { display: flex; flex-direction: column; gap: 0.5rem; }
  .providers-empty { font-size: 0.8rem; color: var(--text-muted); font-style: italic; margin: 0; }
  .providers-list { display: flex; flex-direction: column; gap: 0.5rem; }
  .drop-indicator { height: 2px; background: var(--accent); border-radius: 1px; margin: 1px 0; }

  .provider-row {
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    transition: border-color var(--duration-normal);
  }
  .provider-row.expanded { border-color: color-mix(in srgb, var(--accent) 40%, var(--border)); }
  .provider-row.dragging { opacity: 0.4; }

  .provider-header-row { display: flex; align-items: stretch; }
  /* Grip revealed on row hover — sanctioned grip row (UI-CONVENTIONS
     §12.5): clicking the row expands its editor. */
  .drag-handle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    flex-shrink: 0;
    color: var(--text-faint);
    cursor: grab;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .provider-row:hover .drag-handle,
  .drag-handle:focus-visible { opacity: 1; }
  .drag-handle:active { cursor: grabbing; }

  .provider-header {
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
    transition: background var(--duration-fast);
  }
  .provider-header:hover { background: var(--bg-subtle-hover); }
  .expand-icon { color: var(--text-muted); display: flex; }
  .provider-label { font-weight: 500; }
  .provider-badge {
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
  .provider-count { margin-left: auto; font-size: 0.7rem; color: var(--text-muted); }
  .provider-warning {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--warning);
  }

  .provider-detail {
    padding: 0.65rem;
    border-top: 1px solid var(--border-muted);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    background: var(--bg-elevated);
  }
  .provider-row.expanded:not(:has(.provider-header-row)) .provider-detail { border-top: none; }
  .provider-actions { display: flex; gap: 0.4rem; }
  .provider-delete { margin-left: auto; }
  .provider-delete:hover { color: var(--danger); }

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
    transition: border-color var(--duration-normal), color var(--duration-normal);
  }
  .add-btn:hover { border-color: var(--accent); color: var(--accent); }
</style>
