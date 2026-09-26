<script lang="ts">
  // Edits one rules router: name, ordered rules (drag to reorder), the
  // fallback, the heuristic's thresholds and keyword lists, and a Try-it
  // preview.
  import { Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { RowReorder } from '../lib/rowReorder.svelte'
  import type { LLMProviderSummary, LLMRouter, LLMRouting, RoutingRule } from '@shared/types'
  import RoutingRuleRow from './RoutingRuleRow.svelte'
  import RouteTryIt from './RouteTryIt.svelte'
  import ModelChoiceSelect from './ModelChoiceSelect.svelte'

  let { router, routing, accounts, onchange }: {
    router: LLMRouter
    routing: LLMRouting
    accounts: LLMProviderSummary[]
    onchange: (next: LLMRouter) => void
  } = $props()

  let expandedRuleId = $state<string | null>(null)
  let rules = $derived(router.rules ?? [])

  const drag = new RowReorder<RoutingRule>(() => rules, next => patch({ rules: next }))

  function patch(p: Partial<LLMRouter>) {
    onchange({ ...router, ...p })
  }

  function setRule(next: RoutingRule) {
    patch({ rules: rules.map(r => r.id === next.id ? next : r) })
  }

  function addRule() {
    const rule: RoutingRule = { id: crypto.randomUUID().slice(0, 8), target: 'tier:balanced' }
    patch({ rules: [...rules, rule] })
    expandedRuleId = rule.id
  }

  function keywordList(text: string): string[] | undefined {
    const list = text.split(',').map(s => s.trim()).filter(Boolean)
    return list.length ? list : undefined
  }

  function threshold(e: Event): number | undefined {
    const n = Math.round(Number((e.target as HTMLInputElement).value))
    return n > 0 && n <= 100 ? n : undefined
  }
</script>

<div class="router-editor">
  <label class="rf"><span>{t('llm_routing.router_name')}</span>
    <input type="text" value={router.name} oninput={(e) => patch({ name: (e.target as HTMLInputElement).value })} />
  </label>

  <div class="rf">
    <span>{t('llm_routing.rules_title')}</span>
    <span class="hint">{t('llm_routing.rules_hint')}</span>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="rules" role="list" ondrop={drag.drop} ondragover={drag.containerOver}>
      {#each rules as rule, i (rule.id)}
        {#if drag.showsIndicatorBefore(rule.id)}<div class="drop-indicator"></div>{/if}
        <RoutingRuleRow
          {rule}
          index={i}
          {routing}
          {accounts}
          {drag}
          expanded={expandedRuleId === rule.id}
          ontoggle={() => expandedRuleId = expandedRuleId === rule.id ? null : rule.id}
          onchange={setRule}
          onremove={() => patch({ rules: rules.filter(r => r.id !== rule.id) })}
        />
      {/each}
      {#if drag.showsIndicatorAtEnd()}<div class="drop-indicator"></div>{/if}
    </div>
    <button class="btn subtle add-rule" onclick={addRule}><Plus size={12} /> {t('llm_routing.rule_add')}</button>
  </div>

  <div class="rf"><span>{t('llm_routing.fallback')}</span>
    <ModelChoiceSelect value={router.fallback ?? 'tier:balanced'} {routing} {accounts} tiers routers={false} onchange={(ref) => patch({ fallback: ref })} />
    <span class="hint">{t('llm_routing.fallback_hint')}</span>
  </div>

  <details class="advanced">
    <summary>{t('llm_routing.heuristic_title')}</summary>
    <div class="thresholds">
      <label class="rf"><span>{t('llm_routing.threshold_medium')}</span>
        <input type="number" min="1" max="100" value={router.thresholds.medium ?? ''} onchange={(e) => patch({ thresholds: { ...router.thresholds, medium: threshold(e) } })} placeholder="30" />
      </label>
      <label class="rf"><span>{t('llm_routing.threshold_high')}</span>
        <input type="number" min="1" max="100" value={router.thresholds.high ?? ''} onchange={(e) => patch({ thresholds: { ...router.thresholds, high: threshold(e) } })} placeholder="60" />
      </label>
    </div>
    <label class="rf"><span>{t('llm_routing.reasoning_keywords')}</span>
      <input type="text" value={(router.reasoning_keywords ?? []).join(', ')} onchange={(e) => patch({ reasoning_keywords: keywordList((e.target as HTMLInputElement).value) })} placeholder={t('llm_routing.keywords_default')} />
    </label>
    <label class="rf"><span>{t('llm_routing.light_keywords')}</span>
      <input type="text" value={(router.light_keywords ?? []).join(', ')} onchange={(e) => patch({ light_keywords: keywordList((e.target as HTMLInputElement).value) })} placeholder={t('llm_routing.keywords_default')} />
    </label>
  </details>

  <div class="rf"><span>{t('llm_routing.try_title')}</span>
    <RouteTryIt routerId={router.id} {routing} {accounts} />
  </div>
</div>

<style>
  .router-editor { display: flex; flex-direction: column; gap: 0.6rem; }
  .rf { display: flex; flex-direction: column; gap: 0.25rem; }
  .rf > span:first-child { font-size: 0.75rem; font-weight: 500; color: var(--text-secondary); }
  .hint { font-size: 0.7rem; color: var(--text-muted); }
  .rules { display: flex; flex-direction: column; gap: 0.3rem; }
  .drop-indicator { height: 2px; background: var(--accent); border-radius: 1px; }
  .add-rule { align-self: flex-start; }
  .advanced summary { cursor: pointer; font-size: 0.75rem; font-weight: 500; color: var(--text-secondary); }
  .advanced[open] { display: flex; flex-direction: column; gap: 0.45rem; }
  .thresholds { display: flex; gap: 0.75rem; margin-top: 0.35rem; }
  .thresholds input { width: 6rem; }
  input[type='text'], input[type='number'] {
    padding: 0.3rem 0.45rem; border-radius: 5px; border: 1px solid var(--border);
    background: var(--bg-base); color: var(--text-primary);
    font-size: 0.8rem; font-family: inherit; outline: none;
  }
  input:focus { border-color: var(--accent); }
</style>
