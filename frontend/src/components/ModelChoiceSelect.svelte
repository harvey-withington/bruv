<script lang="ts">
  // One picker for every "which model serves this" setting: task
  // assignments, the default, an agent's model, a chat's model, router
  // rule targets. Models are grouped under their providers; routers and
  // tiers are offered when the setting accepts them.
  import { t } from '../lib/i18n.svelte'
  import type { LLMProviderSummary, LLMRouting, ModelRef } from '@shared/types'
  import { MODEL_TIERS, modelChoiceGroups, modelDisplayLabel, modelRef, isDanglingRef } from '@shared/modelRefs'

  let {
    value = $bindable(''),
    routing,
    accounts,
    inheritLabel,
    routers = true,
    tiers = false,
    variant = 'field',
    title,
    onchange,
  }: {
    value?: ModelRef
    routing: LLMRouting
    accounts: LLMProviderSummary[]
    /** Label for the '' (inherit) option; omit to require a choice. */
    inheritLabel?: string
    routers?: boolean
    tiers?: boolean
    variant?: 'field' | 'chip'
    title?: string
    onchange?: (ref: ModelRef) => void
  } = $props()

  let groups = $derived(modelChoiceGroups(routing, accounts, value))
  let dangling = $derived(isDanglingRef(value, routing))

  function handleChange(e: Event) {
    value = (e.target as HTMLSelectElement).value as ModelRef
    onchange?.(value)
  }
</script>

<select class="model-choice {variant}" {value} onchange={handleChange} {title} aria-label={title}>
  {#if inheritLabel !== undefined}
    <option value="">{inheritLabel}</option>
  {:else if !value}
    <option value="" disabled>{t('llm_routing.choose')}</option>
  {/if}
  {#if dangling}
    <option value={value}>{t('llm_routing.missing')}</option>
  {/if}
  {#each groups as g (g.id)}
    <optgroup label={g.label}>
      {#each g.models as m (m.id)}
        <option value={modelRef('model', m.id)}>{modelDisplayLabel(m)}{m.enabled ? '' : ` (${t('llm_routing.disabled')})`}</option>
      {/each}
    </optgroup>
  {/each}
  {#if tiers}
    <optgroup label={t('llm_routing.tiers_group')}>
      {#each MODEL_TIERS as tier (tier)}
        <option value={modelRef('tier', tier)}>{t(`llm_routing.tier_${tier}`)}</option>
      {/each}
    </optgroup>
  {/if}
  {#if routers && routing.routers.length > 0}
    <optgroup label={t('llm_routing.routers_group')}>
      {#each routing.routers as r (r.id)}
        <option value={modelRef('router', r.id)}>{t('llm_routing.auto_router', { name: r.name })}</option>
      {/each}
    </optgroup>
  {/if}
</select>

<style>
  .model-choice {
    min-width: 0;
    font-family: inherit;
    color: var(--text-body);
    cursor: pointer;
  }
  .model-choice:focus { outline: none; border-color: var(--accent); }
  /* The popup list: explicit theme colours, never the chip's transparent
     background or muted text (which drew white-on-white / grey rows). */
  .model-choice :global(option),
  .model-choice :global(optgroup) {
    background-color: var(--bg-elevated);
    color: var(--text-primary);
  }
  .model-choice :global(optgroup) { color: var(--text-secondary); }

  .field {
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--bg-surface);
    font-size: 0.82rem;
  }

  .chip {
    max-width: 11rem;
    padding: 0.1rem 0.35rem;
    border: 1px solid var(--border-muted);
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.7rem;
    text-overflow: ellipsis;
  }
  .chip:hover { color: var(--text-primary); border-color: var(--border); }
</style>
