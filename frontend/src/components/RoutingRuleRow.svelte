<script lang="ts">
  // One rule of a rules router: collapsed it reads as a sentence
  // ("Low complexity → Fast tier"); expanded it edits the conditions.
  // Every set condition must hold; empty ones match anything.
  import { ChevronDown, ChevronRight, GripVertical, X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import type { ComplexityBand, LLMProviderSummary, LLMRouting, RoutingRule, RuleToolsCondition } from '@shared/types'
  import { LLM_TASKS, refLabel } from '@shared/modelRefs'
  import type { RowReorder } from '../lib/rowReorder.svelte'
  import ModelChoiceSelect from './ModelChoiceSelect.svelte'

  let { rule, index, routing, accounts, expanded, drag, ontoggle, onchange, onremove }: {
    rule: RoutingRule
    index: number
    routing: LLMRouting
    accounts: LLMProviderSummary[]
    expanded: boolean
    drag: RowReorder<RoutingRule>
    ontoggle: () => void
    onchange: (next: RoutingRule) => void
    onremove: () => void
  } = $props()

  const BANDS: ComplexityBand[] = ['low', 'medium', 'high']

  function patch(p: Partial<RoutingRule>) {
    onchange({ ...rule, ...p })
  }

  function toggleIn<T extends string>(list: T[] | undefined, item: T): T[] {
    const cur = list ?? []
    return cur.includes(item) ? cur.filter(x => x !== item) : [...cur, item]
  }

  function parseKeywords(text: string): string[] {
    return text.split(',').map(s => s.trim()).filter(Boolean)
  }

  function tokens(e: Event): number {
    return Math.max(0, Math.round(Number((e.target as HTMLInputElement).value) || 0))
  }

  let summary = $derived.by(() => {
    const parts: string[] = []
    if (rule.bands?.length) parts.push(t('llm_routing.rule_sum_bands', { bands: rule.bands.map(b => t(`llm_routing.band_${b}`)).join(', ') }))
    if (rule.tasks?.length) parts.push(rule.tasks.map(x => t(`llm_routing.task_${x}`)).join(', '))
    if (rule.keywords?.length) parts.push(t('llm_routing.rule_sum_keywords', { keywords: rule.keywords.join(', ') }))
    if (rule.min_tokens) parts.push(t('llm_routing.rule_sum_min', { n: rule.min_tokens }))
    if (rule.max_tokens) parts.push(t('llm_routing.rule_sum_max', { n: rule.max_tokens }))
    if (rule.tools) parts.push(t(`llm_routing.rule_sum_tools_${rule.tools}`))
    const when = parts.length ? parts.join(' · ') : t('llm_routing.rule_sum_always')
    return `${when} → ${refLabel(rule.target, routing, t, t('llm_routing.choose'))}`
  })
</script>

<div class="rule" class:expanded class:dragging={drag.draggingId === rule.id} role="listitem" ondragover={(e) => drag.over(e, rule.id, index)}>
  <div class="rule-head">
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <span class="drag-handle" draggable={true} ondragstart={(e) => drag.start(e, rule.id)} ondragend={drag.end}
      role="button" tabindex="-1" title={t('llm_routing.rule_drag')} aria-label={t('llm_routing.rule_drag')}
    ><GripVertical size={13} /></span>
    <button class="rule-toggle" onclick={ontoggle}>
      {#if expanded}<ChevronDown size={13} />{:else}<ChevronRight size={13} />{/if}
      <span class="rule-index">#{index + 1}</span>
      {#if rule.name}<span class="rule-name">{rule.name}</span>{/if}
      <span class="rule-summary">{summary}</span>
    </button>
    <button class="rule-remove" onclick={onremove} title={t('llm_routing.rule_remove')} aria-label={t('llm_routing.rule_remove')}><X size={13} /></button>
  </div>

  {#if expanded}
    <div class="rule-body">
      <label class="rf"><span>{t('llm_routing.rule_name')}</span>
        <input type="text" value={rule.name ?? ''} oninput={(e) => patch({ name: (e.target as HTMLInputElement).value })} placeholder={t('llm_routing.rule_name_placeholder')} />
      </label>
      <div class="rf"><span>{t('llm_routing.rule_bands')}</span>
        <div class="checks">
          {#each BANDS as band (band)}
            <label><input type="checkbox" checked={rule.bands?.includes(band) ?? false} onchange={() => patch({ bands: toggleIn(rule.bands, band) })} /> {t(`llm_routing.band_${band}`)}</label>
          {/each}
        </div>
      </div>
      <div class="rf"><span>{t('llm_routing.rule_tasks')}</span>
        <div class="checks">
          {#each LLM_TASKS as task (task)}
            <label><input type="checkbox" checked={rule.tasks?.includes(task) ?? false} onchange={() => patch({ tasks: toggleIn(rule.tasks, task) })} /> {t(`llm_routing.task_${task}`)}</label>
          {/each}
        </div>
      </div>
      <label class="rf"><span>{t('llm_routing.rule_keywords')}</span>
        <input type="text" value={(rule.keywords ?? []).join(', ')} onchange={(e) => patch({ keywords: parseKeywords((e.target as HTMLInputElement).value) })} placeholder={t('llm_routing.rule_keywords_placeholder')} />
      </label>
      <div class="rf"><span>{t('llm_routing.rule_tokens')}</span>
        <div class="checks">
          <input class="num" type="number" min="0" value={rule.min_tokens || ''} onchange={(e) => patch({ min_tokens: tokens(e) })} placeholder={t('llm_routing.rule_min')} />
          <input class="num" type="number" min="0" value={rule.max_tokens || ''} onchange={(e) => patch({ max_tokens: tokens(e) })} placeholder={t('llm_routing.rule_max')} />
        </div>
      </div>
      <label class="rf"><span>{t('llm_routing.rule_tools')}</span>
        <select value={rule.tools ?? ''} onchange={(e) => patch({ tools: (e.target as HTMLSelectElement).value as RuleToolsCondition })}>
          <option value="">{t('llm_routing.rule_tools_any')}</option>
          <option value="yes">{t('llm_routing.rule_tools_yes')}</option>
          <option value="no">{t('llm_routing.rule_tools_no')}</option>
        </select>
      </label>
      <div class="rf"><span>{t('llm_routing.rule_target')}</span>
        <ModelChoiceSelect value={rule.target} {routing} {accounts} tiers routers={false} onchange={(ref) => patch({ target: ref })} />
      </div>
    </div>
  {/if}
</div>

<style>
  .rule { border: 1px solid var(--border-muted); border-radius: 6px; background: var(--bg-surface); }
  .rule.expanded { border-color: color-mix(in srgb, var(--accent) 35%, var(--border)); }
  .rule.dragging { opacity: 0.4; }
  .rule-head { display: flex; align-items: center; }
  .drag-handle {
    display: inline-flex; align-items: center; justify-content: center;
    width: 20px; flex-shrink: 0; align-self: stretch;
    color: var(--text-faint); cursor: grab; opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .rule:hover .drag-handle, .drag-handle:focus-visible { opacity: 1; }
  .rule-toggle {
    flex: 1; min-width: 0;
    display: flex; align-items: center; gap: 0.35rem;
    padding: 0.35rem 0.4rem;
    background: none; border: none; cursor: pointer; text-align: left;
    color: var(--text-body); font-size: 0.78rem; font-family: inherit;
  }
  .rule-toggle:hover { background: var(--bg-subtle-hover); }
  .rule-index { color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .rule-name { font-weight: 600; }
  .rule-summary { color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .rule-remove {
    background: none; border: none; cursor: pointer; padding: 0.3rem 0.45rem;
    color: var(--text-muted); display: flex;
  }
  .rule-remove:hover { color: var(--danger); }
  .rule-body {
    display: flex; flex-direction: column; gap: 0.45rem;
    padding: 0.5rem 0.6rem 0.6rem 1.6rem;
    border-top: 1px solid var(--border-muted);
  }
  .rf { display: flex; flex-direction: column; gap: 0.2rem; }
  .rf > span { font-size: 0.72rem; font-weight: 500; color: var(--text-secondary); }
  .checks { display: flex; flex-wrap: wrap; gap: 0.3rem 0.8rem; font-size: 0.78rem; color: var(--text-body); }
  .checks label { display: flex; align-items: center; gap: 0.25rem; cursor: pointer; }
  .num { width: 7rem; }
  input[type='text'], input[type='number'], select {
    padding: 0.3rem 0.45rem; border-radius: 5px; border: 1px solid var(--border);
    background: var(--bg-base); color: var(--text-primary);
    font-size: 0.8rem; font-family: inherit; outline: none;
  }
  input:focus, select:focus { border-color: var(--accent); }
</style>
