<script lang="ts">
  // Settings → AI → Routers: routing engines that pick a model per
  // request. Choose one anywhere a model is chosen ("Auto · <name>").
  // Today's engine is the rules router over a complexity heuristic.
  import { Plus, Trash2, ChevronDown, ChevronRight } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { NewLLMRouter } from '@shared/api'
  import type { LLMProviderSummary, LLMRouter, LLMRouting } from '@shared/types'
  import { removeRouter } from '@shared/modelRefs'
  import RouterEditor from './RouterEditor.svelte'

  let { routing = $bindable(), accounts }: {
    routing: LLMRouting
    accounts: LLMProviderSummary[]
  } = $props()

  let expandedId = $state<string | null>(null)

  async function addRouter() {
    try {
      const router = await NewLLMRouter(t('llm_routing.router_default_name', { n: routing.routers.length + 1 }))
      routing = { ...routing, routers: [...routing.routers, router] }
      expandedId = router.id
    } catch {
      showToast(t('llm_routing.router_add_failed'), 'error')
    }
  }

  function setRouter(next: LLMRouter) {
    routing = { ...routing, routers: routing.routers.map(r => r.id === next.id ? next : r) }
  }

  async function remove(r: LLMRouter) {
    if (!await showConfirm(t('llm_routing.router_delete_confirm', { name: r.name }))) return
    routing = removeRouter(routing, r.id)
    if (expandedId === r.id) expandedId = null
  }
</script>

<div class="routers">
  {#if routing.routers.length === 0}
    <p class="routers-empty">{t('llm_routing.routers_empty')}</p>
  {/if}
  {#each routing.routers as r (r.id)}
    <div class="router-row" class:expanded={expandedId === r.id}>
      <div class="router-head">
        <button class="router-toggle" onclick={() => expandedId = expandedId === r.id ? null : r.id}>
          {#if expandedId === r.id}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
          <span class="router-name">{r.name}</span>
          <span class="router-meta">{t('llm_routing.router_rule_count', { count: r.rules?.length ?? 0 })}</span>
        </button>
        <button class="btn subtle router-delete" onclick={() => remove(r)} title={t('llm_routing.router_delete_hint')}>
          <Trash2 size={12} /> {t('common.delete')}
        </button>
      </div>
      {#if expandedId === r.id}
        <div class="router-body">
          <RouterEditor router={r} {routing} {accounts} onchange={setRouter} />
        </div>
      {/if}
    </div>
  {/each}
  <button class="add-btn" onclick={addRouter} disabled={routing.models.length === 0}>
    <Plus size={14} /> {t('llm_routing.router_add')}
  </button>
</div>

<style>
  .routers { display: flex; flex-direction: column; gap: 0.5rem; }
  .routers-empty { margin: 0; font-size: 0.8rem; color: var(--text-muted); font-style: italic; }
  .router-row { border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
  .router-row.expanded { border-color: color-mix(in srgb, var(--accent) 40%, var(--border)); }
  .router-head { display: flex; align-items: center; }
  .router-toggle {
    flex: 1; display: flex; align-items: center; gap: 0.4rem;
    padding: 0.5rem 0.65rem; background: none; border: none; cursor: pointer;
    color: var(--text-body); font-size: 0.82rem; font-family: inherit; text-align: left;
  }
  .router-toggle:hover { background: var(--bg-subtle-hover); }
  .router-name { font-weight: 500; }
  .router-meta { margin-left: auto; font-size: 0.7rem; color: var(--text-muted); }
  .router-delete:hover { color: var(--danger); }
  .router-body { padding: 0.65rem; border-top: 1px solid var(--border-muted); background: var(--bg-elevated); }
  .add-btn {
    display: flex; align-items: center; gap: 0.3rem; padding: 0.35rem 0.65rem;
    background: none; border: 1px dashed var(--border); border-radius: 6px;
    color: var(--text-muted); font-size: 0.8rem; cursor: pointer;
  }
  .add-btn:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
  .add-btn:disabled { opacity: 0.5; cursor: default; }
</style>
