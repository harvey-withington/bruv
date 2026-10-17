<script lang="ts">
  // Resolve an offline card's conflicts on a phone: each part both this
  // device and home changed, one under the other, with keep-this-phone's /
  // keep-home's / keep-both. Applying records every choice; the card then
  // syncs.
  import { onMount } from 'svelte'
  import BottomSheet from './BottomSheet.svelte'
  import { offline } from '../lib/offline.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'
  import { canKeepBoth, type ConflictChoice, type OfflineConflict } from '@shared/offline/merge'
  import { conflictLabel, conflictValueText } from '@shared/offline/labels'

  let { cardId, onClose }: { cardId: string; onClose: () => void } = $props()

  let conflicts = $state<OfflineConflict[]>([])
  let choices = $state<Record<string, ConflictChoice>>({})
  let applying = $state(false)

  onMount(async () => {
    try {
      const got = await offline.conflictsFor(cardId)
      conflicts = got.conflicts
      choices = { ...got.resolutions }
    } catch (e) {
      showToast(`${t('offline.resolve_failed')}: ${e instanceof Error ? e.message : ''}`, 'error')
    }
  })

  const ready = $derived(!applying && conflicts.length > 0 && conflicts.every((c) => choices[c.id]))

  async function apply() {
    if (!ready) return
    applying = true
    try {
      for (const c of conflicts) await offline.resolve(cardId, c.id, choices[c.id])
      onClose()
    } catch (e) {
      showToast(`${t('offline.resolve_failed')}: ${e instanceof Error ? e.message : ''}`, 'error')
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

<BottomSheet title={t('offline.conflicts_title')} subtitle={t('offline.conflicts_intro')} historyKey="offlineConflicts" {onClose}>
  <div class="conflicts">
    {#each conflicts as c (c.id)}
      <section class="conflict">
        <h3>{conflictLabel(c, t)}</h3>
        <span class="side-label">{t('offline.side_mine')}</span>
        <pre class="value">{conflictValueText(c, 'mine', t)}</pre>
        <span class="side-label">{t('offline.side_theirs')}</span>
        <pre class="value">{conflictValueText(c, 'theirs', t)}</pre>
        <div class="choices" role="radiogroup" aria-label={conflictLabel(c, t)}>
          {#each options(c) as o (o.choice)}
            <button
              type="button"
              class="choice"
              class:on={choices[c.id] === o.choice}
              role="radio"
              aria-checked={choices[c.id] === o.choice}
              onclick={() => (choices[c.id] = o.choice)}
            >{o.label}</button>
          {/each}
        </div>
      </section>
    {/each}
    <button type="button" class="apply" onclick={apply} disabled={!ready}>
      {applying ? t('common.working') : t('offline.apply_choices')}
    </button>
  </div>
</BottomSheet>

<style>
  .conflicts { display: flex; flex-direction: column; gap: 1.1rem; padding-bottom: 0.5rem; }
  .conflict { display: flex; flex-direction: column; gap: 0.35rem; }
  .conflict h3 { margin: 0; font-size: 0.95rem; font-weight: 600; color: var(--text); }
  .side-label { font-size: 0.72rem; font-weight: 600; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.03em; }
  .value {
    margin: 0;
    max-height: 9rem;
    overflow: auto;
    padding: 0.55rem 0.65rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: inherit;
    font-size: 0.85rem;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text);
  }
  .choices { display: flex; gap: 0.4rem; flex-wrap: wrap; margin-top: 0.2rem; }
  .choice, .apply {
    min-height: 40px;
    padding: 0 0.85rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg-elev-1);
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .choice.on { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .apply { border-color: var(--accent); color: var(--accent); font-weight: 600; min-height: 44px; }
  .apply:disabled { opacity: 0.6; cursor: default; }
</style>
