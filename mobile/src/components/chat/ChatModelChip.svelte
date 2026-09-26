<script lang="ts">
  // The chat's model on the phone: a native select (the OS picker sheet)
  // naming what will answer the next message. "Default" clears the chat's
  // own choice so it follows Settings → AI → Use for. Mirrors desktop's
  // ChatModelChip; configuring models stays on desktop.
  import { untrack } from 'svelte'
  import { t } from '../../lib/i18n.svelte'
  import { repoRPC } from '../../lib/auth'
  import { llmRegistry, loadLLMRegistry } from '../../lib/llmRegistry'
  import { onReconnect } from '../../lib/connectivity.svelte'
  import { effectiveRef, isDanglingRef, modelChoiceGroups, modelDisplayLabel, modelRef, refLabel } from '@shared/modelRefs'
  import type { ModelRef } from '@shared/types'
  import type { ChatScope } from './scope'

  let { scope, onerror }: { scope: ChatScope; onerror: (message: string) => void } = $props()

  let choice = $state<ModelRef>('')
  let ready = $state(false)

  let task = $derived(scope.kind === 'card' ? 'card_chat' as const : 'project_chat' as const)
  let groups = $derived(modelChoiceGroups(llmRegistry.routing, llmRegistry.accounts, choice))
  let inheritLabel = $derived(t('llm_routing.chat_default', {
    label: refLabel(effectiveRef('', task, llmRegistry.routing), llmRegistry.routing, t, t('llm_routing.first_model')),
  }))

  function scopeParams(): string[] {
    return scope.kind === 'card' ? [scope.cardID] : [scope.brand, scope.stream, scope.project]
  }

  // Reload when the scope changes; untracked so the registry state the
  // load writes doesn't re-trigger it.
  $effect(() => {
    void scope
    untrack(load)
  })

  $effect(() => onReconnect(() => {
    if (!ready) void load()
  }))

  async function load() {
    ready = false
    try {
      const method = scope.kind === 'card' ? 'GetCardChatModel' : 'GetProjectChatModel'
      const [c] = await Promise.all([repoRPC<ModelRef>(method, scopeParams()), loadLLMRegistry(true)])
      choice = c
      ready = true
    } catch {
      onerror(t('llm_routing.load_failed'))
    }
  }

  async function change(e: Event) {
    const ref = (e.target as HTMLSelectElement).value as ModelRef
    const previous = choice
    choice = ref
    try {
      const method = scope.kind === 'card' ? 'SetCardChatModel' : 'SetProjectChatModel'
      await repoRPC(method, [...scopeParams(), ref])
    } catch {
      choice = previous
      onerror(t('llm_routing.save_failed'))
    }
  }
</script>

{#if ready && llmRegistry.routing.models.length > 0}
  <div class="model-row">
    <select class="model-chip" value={choice} onchange={change} aria-label={t('llm_routing.chat_model_hint')}>
      <option value="">{inheritLabel}</option>
      {#if isDanglingRef(choice, llmRegistry.routing)}
        <option value={choice}>{t('llm_routing.missing')}</option>
      {/if}
      {#each groups as g (g.id)}
        <optgroup label={g.label}>
          {#each g.models as m (m.id)}
            <option value={modelRef('model', m.id)}>{modelDisplayLabel(m)}</option>
          {/each}
        </optgroup>
      {/each}
      {#if llmRegistry.routing.routers.length > 0}
        <optgroup label={t('llm_routing.routers_group')}>
          {#each llmRegistry.routing.routers as r (r.id)}
            <option value={modelRef('router', r.id)}>{t('llm_routing.auto_router', { name: r.name })}</option>
          {/each}
        </optgroup>
      {/if}
    </select>
  </div>
{/if}

<style>
  .model-row {
    display: flex;
    padding: 0.4rem 0.85rem 0;
  }
  .model-chip :global(option),
  .model-chip :global(optgroup) {
    background-color: var(--bg-elev-1);
    color: var(--text);
  }
  .model-chip {
    max-width: 100%;
    min-width: 0;
    padding: 0.3rem 0.55rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.78rem;
    font-family: inherit;
    text-overflow: ellipsis;
  }
</style>
