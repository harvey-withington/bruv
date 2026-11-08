<script lang="ts">
  // Resolve an offline card's conflicts: for each part both this device and
  // home changed, show both versions and let the user keep one (or, for
  // text, both). Applying records every choice; the card then syncs.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap, portal } from '../lib/actions'
  import { keyLayer } from '../lib/keyLayer'
  import { showToast } from '../lib/toast.svelte'
  import { offline } from '../lib/offline.svelte'
  import { canKeepBoth, type ConflictChoice, type OfflineConflict } from '@shared/offline/merge'
  import { conflictLabel, conflictValueText } from '@shared/offline/labels'
  import { errorDetail } from '@shared/errors'

  let { cardId, onClose }: { cardId: string; onClose: () => void } = $props()

  let conflicts = $state<OfflineConflict[]>([])
  let choices = $state<Record<string, ConflictChoice>>({})
  let loading = $state(true)
  let applying = $state(false)

  onMount(async () => {
    try {
      const got = await offline.conflictsFor(cardId)
      conflicts = got.conflicts
      choices = { ...got.resolutions }
    } catch (e) {
      showToast(`${t('offline.resolve_failed')}: ${errorDetail(e)}`, 'error')
    } finally {
      loading = false
    }
  })

  const ready = $derived(!loading && !applying && conflicts.length > 0 && conflicts.every((c) => choices[c.id]))

  async function apply() {
    if (!ready) return
    applying = true
    try {
      for (const c of conflicts) await offline.resolve(cardId, c.id, choices[c.id])
      onClose()
    } catch (e) {
      showToast(`${t('offline.resolve_failed')}: ${errorDetail(e)}`, 'error')
    } finally {
      applying = false
    }
  }

  function options(c: OfflineConflict): Array<{ choice: ConflictChoice; label: string }> {
    const opts: Array<{ choice: ConflictChoice; label: string }> = [
      { choice: 'mine', label: t('offline.keep_mine') },
      { choice: 'theirs', label: t('offline.keep_theirs') },
    ]
    if (canKeepBoth(c)) opts.push({ choice: 'both', label: t('offline.keep_both') })
    return opts
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) onClose() }} out:fade={{ duration: 150 }}>
  <div class="dialog" role="dialog" tabindex="-1" aria-label={t('offline.conflicts_title')} use:focusTrap use:keyLayer={{ onEscape: onClose }}>
    <div class="dialog-header">
      <h2>{t('offline.conflicts_title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <p class="intro">{t('offline.conflicts_intro')}</p>
      {#if loading}
        <p class="intro">{t('common.loading')}</p>
      {/if}
      {#each conflicts as c (c.id)}
        <section class="conflict">
          <h3>{conflictLabel(c, t)}</h3>
          <div class="sides">
            <div class="side">
              <span class="side-label">{t('offline.side_mine')}</span>
              <pre class="value">{conflictValueText(c, 'mine', t)}</pre>
            </div>
            <div class="side">
              <span class="side-label">{t('offline.side_theirs')}</span>
              <pre class="value">{conflictValueText(c, 'theirs', t)}</pre>
            </div>
          </div>
          <div class="choices" role="radiogroup" aria-label={conflictLabel(c, t)}>
            {#each options(c) as o (o.choice)}
              <button
                class="btn"
                class:primary={choices[c.id] === o.choice}
                role="radio"
                aria-checked={choices[c.id] === o.choice}
                onclick={() => choices[c.id] = o.choice}
              >{o.label}</button>
            {/each}
          </div>
        </section>
      {/each}
    </div>

    <div class="dialog-footer">
      <button class="btn" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn primary" onclick={apply} disabled={!ready}>
        {applying ? t('common.working') : t('offline.apply_choices')}
      </button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: min(760px, 92vw);
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 { font-size: 1.1rem; font-weight: 600; margin: 0; }
  .close-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 0.25rem; line-height: 1; border-radius: 4px; }
  .close-btn:hover, .close-btn:focus-visible { color: var(--text-primary); }
  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 1rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }
  .intro { margin: 0; font-size: 0.8rem; color: var(--text-muted); }

  .conflict { display: flex; flex-direction: column; gap: 0.5rem; }
  .conflict h3 { margin: 0; font-size: 0.85rem; font-weight: 600; color: var(--text-primary); }
  .sides { display: grid; grid-template-columns: 1fr 1fr; gap: 0.6rem; }
  .side { display: flex; flex-direction: column; gap: 0.25rem; min-width: 0; }
  .side-label { font-size: 0.72rem; font-weight: 500; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.03em; }
  .value {
    margin: 0;
    max-height: 12rem;
    overflow: auto;
    padding: 0.5rem 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-family: inherit;
    font-size: 0.8rem;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text-primary);
  }
  .choices { display: flex; gap: 0.4rem; flex-wrap: wrap; }
</style>
