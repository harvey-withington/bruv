<script lang="ts">
  // The chat's Edit / Suggest / Chat button (shared/aiMode): a click moves
  // to the next mode and saves it (ai_mode, one setting for BRUV's own
  // chat and the UNCLI engine). A failed save goes back and says so.
  import { MessageCircle, PencilLine, ListChecks } from 'lucide-svelte'
  import { GetLLMConfig, SetLLMConfig } from '@shared/api'
  import { nextAIMode, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'

  let { mode = $bindable('edit') }: { mode: AIMode } = $props()

  async function cycle() {
    const previous = mode
    mode = nextAIMode(mode)
    try {
      const cfg = await GetLLMConfig()
      await SetLLMConfig({ ...cfg, ai_mode: mode })
    } catch (e) {
      mode = previous
      showToast(t('chat.mode_save_failed', { detail: errorDetail(e) }), 'error')
    }
  }
</script>

<button
  type="button"
  class="mode-btn mode-{mode}"
  onclick={cycle}
  title={t(`chat.mode_${mode}_hint`)}
>
  {#if mode === 'edit'}
    <PencilLine size={11} />
  {:else if mode === 'suggest'}
    <ListChecks size={11} />
  {:else}
    <MessageCircle size={11} />
  {/if}
  {t(`chat.mode_${mode}`)}
</button>

<style>
  .mode-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 4px 0;
    border-radius: 6px;
    font-size: 0.7rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-muted);
    transition: color 0.15s, border-color 0.15s, background 0.15s;
    width: 100%;
  }
  .mode-btn:hover {
    color: var(--text-primary);
  }
  .mode-chat {
    color: var(--success);
    border-color: color-mix(in srgb, var(--success) 50%, transparent);
  }
  .mode-suggest {
    color: var(--warning);
    border-color: color-mix(in srgb, var(--warning) 50%, transparent);
  }
  .mode-edit {
    color: var(--danger);
    border-color: color-mix(in srgb, var(--danger) 50%, transparent);
  }
</style>
