<script lang="ts">
  // Settings → AI → Use for: which model or router serves each AI task.
  // Tasks left on "Default" follow the Default row.
  import { t } from '../lib/i18n.svelte'
  import type { LLMProviderSummary, LLMRouting, ModelRef } from '@shared/types'
  import { LLM_TASKS, refLabel } from '@shared/modelRefs'
  import ModelChoiceSelect from './ModelChoiceSelect.svelte'

  let { routing = $bindable(), accounts }: {
    routing: LLMRouting
    accounts: LLMProviderSummary[]
  } = $props()

  let defaultLabel = $derived(t('llm_routing.inherit_default', {
    label: refLabel(routing.default, routing, t, t('llm_routing.first_model')),
  }))

  function setTask(task: string, ref: ModelRef) {
    const tasks = { ...routing.tasks }
    if (ref) tasks[task] = ref
    else delete tasks[task]
    routing = { ...routing, tasks }
  }
</script>

{#if routing.models.length === 0}
  <p class="use-for-empty">{t('llm_routing.use_for_empty')}</p>
{:else}
  <div class="use-for">
    <span class="use-for-label">{t('llm_routing.default_row')}</span>
    <ModelChoiceSelect
      value={routing.default ?? ''}
      {routing}
      {accounts}
      inheritLabel={t('llm_routing.first_model')}
      onchange={(ref) => routing = { ...routing, default: ref }}
    />
    <span class="use-for-hint">{t('llm_routing.default_row_hint')}</span>

    {#each LLM_TASKS as task (task)}
      <span class="use-for-label">{t(`llm_routing.task_${task}`)}</span>
      <ModelChoiceSelect
        value={routing.tasks[task] ?? ''}
        {routing}
        {accounts}
        inheritLabel={defaultLabel}
        onchange={(ref) => setTask(task, ref)}
      />
      <span class="use-for-hint">{t(`llm_routing.task_${task}_hint`)}</span>
    {/each}
  </div>
{/if}

<style>
  .use-for {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.3rem 0.75rem;
    align-items: center;
  }
  .use-for-label {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .use-for-hint {
    grid-column: 2;
    margin-top: -0.15rem;
    margin-bottom: 0.3rem;
    font-size: 0.7rem;
    color: var(--text-muted);
  }
  .use-for-empty { margin: 0; font-size: 0.8rem; color: var(--text-muted); font-style: italic; }
</style>
