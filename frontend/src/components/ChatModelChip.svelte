<script lang="ts">
  // The chat's model: shows what will answer the next message and lets
  // the user pin this chat to another model or router. "Default" clears
  // the chat's choice so it follows the task assignment again.
  import { untrack } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { LLMTask, ModelRef } from '@shared/types'
  import { llmRegistry, loadLLMRegistry } from '../lib/llmRegistry'
  import { effectiveRef, refLabel } from '@shared/modelRefs'
  import ModelChoiceSelect from './ModelChoiceSelect.svelte'

  let { task, reloadKey, loadChoice, saveChoice }: {
    task: LLMTask
    /** Changes when the chat's owner changes (another card / project). */
    reloadKey: string
    loadChoice: () => Promise<ModelRef>
    saveChoice: (ref: ModelRef) => Promise<void>
  } = $props()

  let choice = $state<ModelRef>('')
  let ready = $state(false)

  // Reload when the chat's owner changes; untracked so the registry
  // state the load writes doesn't re-trigger it.
  $effect(() => {
    void reloadKey
    untrack(load)
  })

  async function load() {
    ready = false
    try {
      // Force: the registry may have changed in Settings or on another
      // connection since the last chat opened.
      const [c] = await Promise.all([loadChoice(), loadLLMRegistry(true)])
      choice = c
      ready = true
    } catch {
      showToast(t('llm_routing.load_failed'), 'error')
    }
  }

  let inheritLabel = $derived(t('llm_routing.chat_default', {
    label: refLabel(effectiveRef('', task, llmRegistry.routing), llmRegistry.routing, t, t('llm_routing.first_model')),
  }))

  async function change(ref: ModelRef) {
    const previous = choice
    choice = ref
    try {
      await saveChoice(ref)
    } catch {
      choice = previous
      showToast(t('llm_routing.save_failed'), 'error')
    }
  }
</script>

{#if ready && llmRegistry.routing.models.length > 0}
  <ModelChoiceSelect
    value={choice}
    routing={llmRegistry.routing}
    accounts={llmRegistry.accounts}
    {inheritLabel}
    variant="chip"
    title={t('llm_routing.chat_model_hint')}
    onchange={change}
  />
{/if}
