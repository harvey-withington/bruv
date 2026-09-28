<script lang="ts" generics="Id extends string">
  // Renders a settings tab's fields only once every section they belong
  // to has loaded. A failed section shows why, offers Try again, and says
  // it won't be saved — the form never shows (or saves) defaults in place
  // of settings it couldn't read.
  import type { Snippet } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import type { SettingsSections } from '../lib/settingsSections.svelte'

  let { sections, ids, labels, children }: {
    sections: SettingsSections<Id>
    ids: Id[]
    /** Localized section names, for the error message. */
    labels: Record<Id, string>
    children: Snippet
  } = $props()

  let failed = $derived(ids.filter(id => sections.status[id] === 'failed'))
  let loading = $derived(ids.some(id => sections.status[id] === 'loading'))
</script>

{#if failed.length > 0}
  {#each failed as id (id)}
    <div class="section-error" role="alert">
      <p class="section-error-text">{t('prefs.section_load_failed', { section: labels[id] })}</p>
      {#if sections.errors[id]}<p class="section-error-detail">{sections.errors[id]}</p>{/if}
      <button class="btn" onclick={() => sections.load(id)}>{t('common.retry')}</button>
    </div>
  {/each}
{:else if loading}
  <p class="section-loading">{t('common.loading')}</p>
{:else}
  {@render children()}
{/if}

<style>
  .section-error {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
    padding: 0.65rem 0.75rem;
    border: 1px solid color-mix(in srgb, var(--danger) 45%, var(--border));
    border-radius: 6px;
    background: color-mix(in srgb, var(--danger) 8%, transparent);
  }
  .section-error-text { margin: 0; font-size: 0.82rem; color: var(--text-primary); }
  .section-error-detail { margin: 0; font-size: 0.72rem; color: var(--text-muted); font-family: monospace; }
  .section-loading { margin: 0; font-size: 0.82rem; color: var(--text-muted); }
</style>
