<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { ChevronLeft, User, Bot } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate, cardURL } from '../lib/router.svelte'
  import { onEvent } from '../lib/events.svelte'
  import { t } from '../lib/i18n.svelte'
  import type { ActivityEntry } from '@shared/types'

  let entries = $state<ActivityEntry[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)

  async function reload() {
    try {
      const list = (await repoRPC<ActivityEntry[]>('ListActivityLog', [200])) ?? []
      entries = list
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('activity.err_load')
    } finally {
      loading = false
    }
  }

  onMount(reload)

  // Live: any card / category mutation produces an activity entry.
  // Coalesce reloads so a flurry of events doesn't thrash.
  let liveTimer: ReturnType<typeof setTimeout> | null = null
  const unsub = onEvent((ev) => {
    if (
      ev.topic === 'card:created' ||
      ev.topic === 'card:updated' ||
      ev.topic === 'card:deleted' ||
      ev.topic === 'category:updated' ||
      ev.topic === 'category:deleted'
    ) {
      if (liveTimer) clearTimeout(liveTimer)
      liveTimer = setTimeout(() => {
        liveTimer = null
        void reload()
      }, 200)
    }
  })
  onDestroy(() => {
    if (liveTimer) clearTimeout(liveTimer)
    unsub()
  })

  function formatTime(iso: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    const now = Date.now()
    const ms = now - d.getTime()
    const min = 60_000, hr = 60 * min, day = 24 * hr
    if (ms < min) return t('activity.now')
    if (ms < hr) return t('activity.minutes_ago', { n: Math.floor(ms / min) })
    if (ms < day) return t('activity.hours_ago', { n: Math.floor(ms / hr) })
    if (ms < 7 * day) return t('activity.days_ago', { n: Math.floor(ms / day) })
    return d.toLocaleDateString()
  }

  function describe(e: ActivityEntry): string {
    // Lightweight, no markdown. Mirrors desktop's InboxActivity verb
    // table (created / updated_title / updated_field / pinned / ...).
    switch (e.action) {
      case 'created': return t('activity.verb_created')
      case 'deleted': return t('activity.verb_deleted')
      case 'pinned': return t('activity.verb_pinned')
      case 'unpinned': return t('activity.verb_unpinned')
      case 'updated_title': return t('activity.verb_renamed')
      case 'updated_description': return t('activity.verb_described')
      case 'updated_field': return e.field ? t('activity.verb_updated_field', { field: e.field }) : t('activity.verb_updated')
      case 'updated_tags': return t('activity.verb_tagged')
      default: return e.action.replace(/_/g, ' ')
    }
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => history.back()} aria-label={t('common.back')}>
    <ChevronLeft size={20} />
  </button>
  <h1>{t('activity.title')}</h1>
  <span class="spacer"></span>
</header>

<main>
  {#if loading && entries.length === 0}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg && entries.length === 0}
    <p class="error">{errorMsg}</p>
  {:else if entries.length === 0}
    <p class="status">{t('activity.empty')}</p>
  {:else}
    <ul class="entries">
      {#each entries as e (e.id)}
        <li>
          <button type="button" class="entry" onclick={() => navigate(cardURL(e.card_id))}>
            <span class="actor-icon" class:llm={e.actor_type === 'llm'} aria-hidden="true">
              {#if e.actor_type === 'llm'}
                <Bot size={14} />
              {:else}
                <User size={14} />
              {/if}
            </span>
            <div class="entry-text">
              <div class="entry-line">
                <span class="actor">{e.actor || t('activity.someone')}</span>
                <span class="verb">{describe(e)}</span>
                <span class="card-title">{e.card_title || t('inbox.untitled')}</span>
              </div>
              {#if e.brand_name || e.project_name}
                <div class="entry-context">
                  {[e.brand_name, e.stream_name, e.project_name].filter(Boolean).join(' › ')}
                </div>
              {/if}
            </div>
            <span class="time">{formatTime(e.timestamp)}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }
  .topbar h1 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
  }
  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    justify-self: start;
    display: inline-flex;
  }
  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    padding: 0.5rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .entries {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .entry {
    display: flex;
    align-items: flex-start;
    gap: 0.65rem;
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.7rem 0.85rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
  }
  .entry:hover,
  .entry:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .actor-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    background: var(--bg);
    color: var(--text-muted);
    flex-shrink: 0;
    margin-top: 2px;
  }
  .actor-icon.llm {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 15%, var(--bg));
  }

  .entry-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }
  .entry-line {
    font-size: 0.88rem;
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .actor {
    font-weight: 600;
    color: var(--text);
  }
  .verb {
    color: var(--text-muted);
    margin: 0 0.25rem;
  }
  .card-title {
    color: var(--text);
  }
  .entry-context {
    font-size: 0.72rem;
    color: var(--text-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .time {
    font-size: 0.7rem;
    color: var(--text-faint);
    flex-shrink: 0;
    margin-top: 4px;
  }

  .status {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
  }
  .error {
    margin: 2rem 0;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    text-align: center;
  }
</style>
