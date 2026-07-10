<script lang="ts">
  import type { ActivityEntry } from '@shared/types'
  import { formatRelativeTime } from '@shared/relativeTime'
  import { t } from '../lib/i18n.svelte'

  let {
    entries,
    onCardClick,
  }: {
    entries: ActivityEntry[]
    onCardClick: (id: string) => void
  } = $props()

  function navigateToProject(entry: ActivityEntry) {
    if (!entry.brand_slug || !entry.stream_slug || !entry.project_slug) return
    document.dispatchEvent(new CustomEvent('bruv:select-project', {
      detail: {
        brandSlug: entry.brand_slug,
        streamSlug: entry.stream_slug,
        projectSlug: entry.project_slug,
      },
    }))
  }

  function actionLabel(entry: ActivityEntry): string {
    switch (entry.action) {
      case 'created':          return t('activity.created')
      case 'deleted':          return t('activity.deleted')
      case 'updated_title':    return t('activity.updated_title')
      case 'updated_type':     return t('activity.updated_type')
      case 'updated_field':    return t('activity.updated_field', { field: entry.field || t('activity.field_content') })
      case 'updated_tags':     return t('activity.updated_tags')
      case 'updated_due_date': return t('activity.updated_due_date')
      case 'pinned':           return t('activity.pinned')
      case 'unpinned':         return t('activity.unpinned')
      default:                 return entry.action
    }
  }

  function breadcrumb(entry: ActivityEntry): string {
    const parts = [entry.brand_name, entry.stream_name, entry.project_name].filter(Boolean)
    if (entry.category_name) parts.push(entry.category_name)
    return parts.join(' / ')
  }

  function relativeTime(iso: string): string {
    return formatRelativeTime(iso, t)
  }
</script>

{#if entries.length === 0}
  <p class="empty">{t('board.inbox_activity_empty')}</p>
{:else}
  <ol class="activity-list">
    {#each entries as entry (entry.id)}
      <li class="activity-entry">
        <div class="entry-meta">
          <span class="actor" class:llm={entry.actor_type === 'llm'}>{entry.actor}</span>
          <span class="action">{actionLabel(entry)}</span>
        </div>

        <div class="entry-card">
          {#if entry.brand_name}
            <button class="card-path" title={breadcrumb(entry)} onclick={() => navigateToProject(entry)}>
              {breadcrumb(entry)}
            </button>
          {/if}
          <button class="card-link" onclick={() => onCardClick(entry.card_id)}>
            {entry.card_title || entry.card_id}
          </button>
        </div>

        <time class="entry-time" datetime={entry.timestamp}>{relativeTime(entry.timestamp)}</time>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .empty {
    color: var(--text-faint);
    font-size: 0.82rem;
    margin: 0;
  }

  .activity-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }

  .activity-entry {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: 0.55rem 0.2rem;
    border-bottom: 1px solid var(--border-muted, var(--border));
    min-width: 0;
  }
  .activity-entry:last-child {
    border-bottom: none;
  }

  .entry-meta {
    display: flex;
    align-items: baseline;
    gap: 0.3rem;
    flex-wrap: wrap;
    min-width: 0;
  }

  .actor {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-body);
    white-space: nowrap;
  }

  .actor.llm {
    color: var(--accent);
  }

  .action {
    font-size: 0.73rem;
    color: var(--text-secondary);
  }

  .entry-card {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .card-link {
    background: none;
    border: none;
    padding: 0;
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--text-strong);
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100%;
  }
  .card-link:hover {
    text-decoration: underline;
  }

  .card-path {
    display: block;
    width: 100%;
    background: none;
    border: none;
    padding: 0;
    font-size: 0.67rem;
    font-family: inherit;
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color var(--duration-fast);
  }
  .card-path:hover {
    color: var(--accent);
  }

  .entry-time {
    font-size: 0.65rem;
    color: var(--text-secondary);
    margin-top: 0.05rem;
  }
</style>
