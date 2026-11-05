<script lang="ts">
  // Settings → AI, under Chat engine when it's UNCLI: the CLI and model a
  // new card chat and project chat open on (LLMConfig.uncli_chats), from
  // what the server's UNCLI can run (GetUncliProviders). A CLI that can't
  // run now is listed, disabled, with UNCLI's reason; a saved choice no
  // longer offered stays selected, marked, so it isn't changed unseen.
  import { onMount } from 'svelte'
  import { GetUncliProviders } from '@shared/api'
  import { errorDetail } from '@shared/errors'
  import type { LLMConfig, UncliChatKind, UncliProvider } from '@shared/types'
  import { parseRunsOn, runsOnGroups, runsOnValue } from '@shared/uncliChat'
  import { t } from '../lib/i18n.svelte'

  let { chats = $bindable() }: { chats: LLMConfig['uncli_chats'] } = $props()

  const KINDS: UncliChatKind[] = ['card_chat', 'project_chat']

  let providers = $state<UncliProvider[] | null>(null)
  let error = $state('')

  onMount(() => {
    GetUncliProviders()
      .then(p => { providers = p ?? [] })
      .catch(e => { error = t('uncli.runs_on_load_failed', { detail: errorDetail(e) }) })
  })

  const groups = $derived(providers ? runsOnGroups(providers, t) : [])
  const offered = $derived(new Set(groups.flatMap(g => g.options.map(o => o.value))))

  function set(kind: UncliChatKind, value: string) {
    const next = { ...chats }
    const choice = parseRunsOn(value)
    if (choice) next[kind] = choice
    else delete next[kind]
    chats = next
  }
</script>

{#each KINDS as kind (kind)}
  {@const value = runsOnValue(chats?.[kind])}
  <label class="field">
    <span class="field-label">{t(`uncli.runs_on_${kind}`)}</span>
    <select {value} disabled={!providers} onchange={e => set(kind, e.currentTarget.value)}>
      <option value="">{t('uncli.runs_on_default')}</option>
      {#if value && providers && !offered.has(value)}
        <option {value}>{t('uncli.runs_on_unknown', { choice: [chats?.[kind]?.provider, chats?.[kind]?.model].filter(Boolean).join(' · ') })}</option>
      {/if}
      {#each groups as group (group.label)}
        <optgroup label={group.label}>
          {#each group.options as option (option.value)}
            <option value={option.value} disabled={option.disabled}>{option.label}</option>
          {/each}
        </optgroup>
      {/each}
    </select>
  </label>
{/each}
<span class="field-hint" class:error={!!error}>
  {error || (providers ? t('uncli.runs_on_hint') : t('uncli.runs_on_loading'))}
</span>

<style>
  .error {
    color: var(--danger-light);
  }
</style>
