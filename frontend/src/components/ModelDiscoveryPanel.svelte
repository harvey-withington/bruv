<script lang="ts">
  // "Find models": asks a provider what it offers and lets the user tick
  // which to add. Models already set up on the provider show as added.
  import { untrack } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { DiscoverLLMModels } from '@shared/api'
  import type { DiscoveredModel } from '@shared/types'

  let { accountId, existingNames, persist, onadd, oncancel }: {
    accountId: string
    existingNames: Set<string>
    /** Saves the settings being edited, so discovery uses the current key / URL. */
    persist: () => Promise<void>
    onadd: (models: DiscoveredModel[]) => void
    oncancel: () => void
  } = $props()

  let found = $state<DiscoveredModel[]>([])
  let loading = $state(true)
  let error = $state('')
  let filter = $state('')
  let picked = $state<Set<string>>(new Set())

  $effect(() => {
    void accountId
    untrack(discover)
  })

  async function discover() {
    loading = true
    error = ''
    try {
      await persist()
      found = await DiscoverLLMModels(accountId)
    } catch (e: unknown) {
      error = e instanceof Error ? e.message : String(e)
    }
    loading = false
  }

  let visible = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return q ? found.filter(m => m.id.toLowerCase().includes(q) || m.label?.toLowerCase().includes(q)) : found
  })

  function toggle(id: string) {
    const next = new Set(picked)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    picked = next
  }

  function add() {
    onadd(found.filter(m => picked.has(m.id)))
  }
</script>

<div class="discovery">
  {#if loading}
    <p class="discovery-status">{t('llm.discover_loading')}</p>
  {:else if error}
    <p class="discovery-status error">{t('llm.discover_failed', { error })}</p>
  {:else if found.length === 0}
    <p class="discovery-status">{t('llm.discover_none')}</p>
  {:else}
    {#if found.length > 8}
      <input class="discovery-filter" type="search" bind:value={filter} placeholder={t('llm.discover_filter')} />
    {/if}
    <div class="discovery-list">
      {#each visible as m (m.id)}
        {@const added = existingNames.has(m.id)}
        <label class="discovery-row" class:added>
          <input type="checkbox" checked={added || picked.has(m.id)} disabled={added} onchange={() => toggle(m.id)} />
          <span class="discovery-id">{m.id}</span>
          {#if m.label && m.label !== m.id}<span class="discovery-label">{m.label}</span>{/if}
          {#if added}<span class="discovery-added">{t('llm.discover_added')}</span>{/if}
        </label>
      {/each}
    </div>
  {/if}
  <div class="discovery-actions">
    <button class="btn primary" onclick={add} disabled={picked.size === 0}>{t('llm.discover_add', { count: picked.size })}</button>
    <button class="btn subtle" onclick={oncancel}>{t('common.cancel')}</button>
  </div>
</div>

<style>
  .discovery {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    background: var(--bg-surface);
  }
  .discovery-status { margin: 0; font-size: 0.78rem; color: var(--text-muted); }
  .discovery-status.error { color: var(--danger); }
  .discovery-filter {
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--bg-base);
    color: var(--text-primary);
    font-size: 0.8rem;
    font-family: inherit;
  }
  .discovery-list {
    max-height: 14rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
  }
  .discovery-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.25rem;
    font-size: 0.78rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .discovery-row:hover { background: var(--bg-subtle-hover); }
  .discovery-row.added { cursor: default; opacity: 0.7; }
  .discovery-id { font-family: monospace; color: var(--text-body); }
  .discovery-label { color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .discovery-added { margin-left: auto; font-size: 0.68rem; color: var(--text-muted); }
  .discovery-actions { display: flex; gap: 0.4rem; }
</style>
