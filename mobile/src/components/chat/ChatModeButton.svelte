<script lang="ts">
  // The chat's Edit / Suggest / Chat button (shared/aiMode): a tap moves
  // to the next mode and saves it (ai_mode, one setting for BRUV's own
  // chat and the UNCLI engine). A failed save goes back and says so
  // through onerror.
  import { MessageCircle, PencilLine, ListChecks } from 'lucide-svelte'
  import { nextAIMode, type AIMode } from '@shared/aiMode'
  import { errorDetail } from '@shared/errors'
  import type { LLMConfig } from '@shared/types'
  import { machineRPC } from '../../lib/auth'
  import { t } from '../../lib/i18n.svelte'

  let { mode = $bindable('edit'), onerror }: { mode: AIMode; onerror: (message: string) => void } = $props()

  async function cycle() {
    const previous = mode
    mode = nextAIMode(mode)
    try {
      const cfg = (await machineRPC<LLMConfig>('GetLLMConfig')) ?? ({} as LLMConfig)
      await machineRPC('SetLLMConfig', [{ ...cfg, ai_mode: mode }])
    } catch (e) {
      mode = previous
      onerror(t('chat.mode_save_failed', { detail: errorDetail(e) }))
    }
  }
</script>

<button type="button" class="mode-btn mode-{mode}" onclick={cycle} title={t(`chat.mode_${mode}_hint`)}>
  {#if mode === 'edit'}
    <PencilLine size={12} />
  {:else if mode === 'suggest'}
    <ListChecks size={12} />
  {:else}
    <MessageCircle size={12} />
  {/if}
  {t(`chat.mode_${mode}`)}
</button>

<style>
  .mode-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    color: var(--text-muted);
    border-radius: 999px;
    padding: 0.3rem 0.5rem;
    font: inherit;
    font-size: 0.7rem;
    cursor: pointer;
    width: 100%;
  }
  .mode-edit { color: var(--danger-border); border-color: var(--danger-border); }
  .mode-suggest { color: var(--warn-border); border-color: var(--warn-border); }
  .mode-chat { color: var(--success-border); border-color: var(--success-border); }
</style>
