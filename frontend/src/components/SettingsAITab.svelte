<script lang="ts">
  // Settings → AI: providers and their models, which model serves each
  // task, routers, and AI behaviour.
  import { t } from '../lib/i18n.svelte'
  import { CHAT_BUDGET_MIN, CHAT_BUDGET_MAX } from '@shared/chatTurn.svelte'
  import type { SettingsForm } from '../lib/settingsForm.svelte'
  import type { FieldVisible } from '../lib/settingsSearch'
  import LLMProvidersManager from './LLMProvidersManager.svelte'
  import TaskRoutingTable from './TaskRoutingTable.svelte'
  import LLMRoutersManager from './LLMRoutersManager.svelte'

  let { form, visible }: { form: SettingsForm; visible: FieldVisible } = $props()
  const llm = $derived(form.llm)
</script>

{#if visible('accounts')}
  <div class="field-section-label">{t('llm.accounts_title')}</div>
  <LLMProvidersManager bind:accounts={form.llmAccounts} bind:routing={form.llmRouting} persist={form.persistLLM} />
{/if}

{#if visible('use_for')}
  <div class="field-section-label">{t('llm_routing.use_for_title')}</div>
  <TaskRoutingTable bind:routing={form.llmRouting} accounts={form.llmAccounts} />
{/if}

{#if visible('routers')}
  <div class="field-section-label">{t('llm_routing.routers_title')}</div>
  <p class="field-hint">{t('llm_routing.routers_hint')}</p>
  <LLMRoutersManager bind:routing={form.llmRouting} accounts={form.llmAccounts} />
{/if}

{#if visible('ai_mode') || visible('min_confidence') || visible('chat_budget') || visible('prompt_cache') || visible('context')}
  <div class="field-section-label">{t('llm.behavior_title')}</div>
{/if}

{#if visible('ai_mode')}
  <label class="field">
    <span class="field-label">{t('llm.ai_mode')}</span>
    <select bind:value={llm.ai_mode}>
      <option value="edit">{t('llm.ai_mode_edit')}</option>
      <option value="suggest">{t('llm.ai_mode_suggest')}</option>
      <option value="chat">{t('llm.ai_mode_chat')}</option>
    </select>
  </label>
{/if}

{#if visible('min_confidence')}
  <label class="field">
    <span class="field-label">{t('llm.min_confidence')}</span>
    <select bind:value={llm.min_confidence}>
      <option value="">{t('llm.min_confidence_any')}</option>
      <option value="low">{t('llm.min_confidence_low')}</option>
      <option value="medium">{t('llm.min_confidence_medium')}</option>
      <option value="high">{t('llm.min_confidence_high')}</option>
    </select>
    <span class="field-hint">{t('llm.min_confidence_hint')}</span>
  </label>
{/if}

{#if visible('chat_budget')}
  <label class="field">
    <span class="field-label">{t('llm.chat_budget_mode')}</span>
    <select bind:value={llm.chat_budget_mode}>
      <option value="warn">{t('llm.chat_budget_warn')}</option>
      <option value="hard">{t('llm.chat_budget_hard')}</option>
      <option value="off">{t('llm.chat_budget_off')}</option>
    </select>
  </label>
  {#if llm.chat_budget_mode !== 'off'}
    <label class="field">
      <span class="field-label">{t('llm.chat_budget_tokens')}</span>
      <input type="number" min={CHAT_BUDGET_MIN} max={CHAT_BUDGET_MAX} step="1000" bind:value={llm.chat_budget_tokens} />
    </label>
  {/if}
  <span class="field-hint">{t(`llm.chat_budget_${llm.chat_budget_mode}_hint`, { max: CHAT_BUDGET_MAX.toLocaleString() })}</span>
{/if}

{#if visible('prompt_cache')}
  <label class="field">
    <span class="field-label">{t('llm.prompt_cache')}</span>
    <select bind:value={llm.prompt_cache}>
      <option value="5m">{t('llm.prompt_cache_short')}</option>
      <option value="1h">{t('llm.prompt_cache_long')}</option>
    </select>
    <span class="field-hint">{t(llm.prompt_cache === '1h' ? 'llm.prompt_cache_long_hint' : 'llm.prompt_cache_short_hint')}</span>
    <span class="field-hint">{t('llm.prompt_cache_scope_hint')}</span>
  </label>
{/if}

{#if visible('context')}
  <label class="field">
    <span class="field-label">{t('llm.context')}</span>
    <textarea rows="4" bind:value={llm.context} placeholder={t('llm.context_placeholder')}></textarea>
  </label>
{/if}
