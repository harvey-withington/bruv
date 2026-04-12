<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { Check, Loader } from 'lucide-svelte'

  let { saving = false }: { saving: boolean } = $props()

  let showSaved = $state(false)
  let fadeTimer: ReturnType<typeof setTimeout> | null = null

  // When saving transitions from true → false, flash "Saved" for 2.5s
  let wasSaving = false
  $effect(() => {
    if (wasSaving && !saving) {
      showSaved = true
      if (fadeTimer) clearTimeout(fadeTimer)
      fadeTimer = setTimeout(() => { showSaved = false }, 2500)
    }
    wasSaving = saving
  })
</script>

<span class="save-indicator" class:visible={saving || showSaved}>
  {#if saving}
    <Loader size={12} class="save-spinner" />
    <span class="save-text saving">{t('common.saving')}</span>
  {:else if showSaved}
    <Check size={12} class="save-check" />
    <span class="save-text saved">{t('common.saved')}</span>
  {/if}
</span>

<style>
  .save-indicator {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.72rem;
    font-weight: 500;
    opacity: 0;
    transition: opacity var(--duration-moderate) ease;
    pointer-events: none;
    user-select: none;
    min-width: 0;
  }
  .save-indicator.visible {
    opacity: 1;
  }

  .save-text.saving {
    color: var(--warning);
  }
  .save-text.saved {
    color: var(--success);
  }

  :global(.save-spinner) {
    color: var(--warning);
    animation: spin 1s linear infinite;
  }
  :global(.save-check) {
    color: var(--success);
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
