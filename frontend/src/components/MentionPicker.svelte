<script lang="ts">
  import { SearchCards, ListBrands, ListStreams, ListProjects } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import { getCardTypeColor } from '../lib/cardTypes'

  type PickerItem = {
    type: 'card' | 'project'
    label: string
    subtitle: string // breadcrumb path (e.g. "Brand > Stream > Project")
    badge: string
    badgeColor: string
    link: string // bruv:card:<id> or bruv:project:<brand>/<stream>/<project>
  }

  let { visible, anchor, onSelect, onClose }: {
    visible: boolean
    anchor: { top: number; left: number } | null
    onSelect: (markdown: string) => void
    onClose: () => void
  } = $props()

  let query = $state('')
  let items = $state<PickerItem[]>([])
  let selectedIndex = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)
  let projectCache = $state<PickerItem[]>([])
  let projectsLoaded = $state(false)

  $effect(() => {
    if (visible) {
      query = ''
      items = []
      selectedIndex = 0
      if (!projectsLoaded) loadProjects()
      setTimeout(() => inputEl?.focus(), 0)
    }
  })

  async function loadProjects() {
    try {
      const brands = await ListBrands() || []
      const results: PickerItem[] = []
      for (const brand of brands) {
        const streams = await ListStreams(brand.slug) || []
        for (const stream of streams) {
          const projects = await ListProjects(brand.slug, stream.slug) || []
          for (const project of projects) {
            results.push({
              type: 'project',
              label: `${brand.name} / ${stream.name} / ${project.name}`,
              subtitle: '',
              badge: 'project',
              badgeColor: '#71717a',
              link: `bruv:project:${project.id}`,
            })
          }
        }
      }
      projectCache = results
      projectsLoaded = true
    } catch (e) {
      console.error('MentionPicker: failed to load projects', e)
    }
  }

  let debounceTimer: ReturnType<typeof setTimeout> | undefined

  function handleInput() {
    clearTimeout(debounceTimer)
    const q = query.trim()
    if (!q) {
      items = []
      selectedIndex = 0
      return
    }
    debounceTimer = setTimeout(async () => {
      const merged: PickerItem[] = []

      // Search cards
      try {
        const cardResults = await SearchCards(q, 8) || []
        for (const r of cardResults) {
          const ctx = (r.ProjectContext || '').trim()
          merged.push({
            type: 'card',
            label: r.Title,
            subtitle: ctx,
            badge: r.Type,
            badgeColor: getCardTypeColor(r.Type),
            link: `bruv:card:${r.CardID}`,
          })
        }
      } catch { /* ignore */ }

      // Filter cached projects
      const lower = q.toLowerCase()
      const projMatches = projectCache.filter(p => p.label.toLowerCase().includes(lower)).slice(0, 5)
      merged.push(...projMatches)

      items = merged
      selectedIndex = 0
    }, 150)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedIndex = Math.min(selectedIndex + 1, items.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      selectedIndex = Math.max(selectedIndex - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (items.length > 0) {
        selectItem(items[selectedIndex])
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  function selectItem(item: PickerItem) {
    const markdown = `[${item.label}](${item.link})`
    onSelect(markdown)
  }
</script>

{#if visible && anchor}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="mention-backdrop" onmousedown={(e) => { if (e.target === e.currentTarget) onClose() }}></div>
  <div class="mention-picker" style="top: {anchor.top}px; left: {anchor.left}px">
    <div class="mention-search">
      <input
        bind:this={inputEl}
        type="text"
        bind:value={query}
        oninput={handleInput}
        onkeydown={handleKeydown}
        placeholder={t('mention.placeholder')}
        class="mention-input"
      />
    </div>
    {#if items.length > 0}
      <div class="mention-results" role="listbox">
        {#each items as item, i}
          <button
            class="mention-result"
            class:active={i === selectedIndex}
            role="option"
            aria-selected={i === selectedIndex}
            onmouseenter={() => selectedIndex = i}
            onclick={() => selectItem(item)}
          >
            <span class="mention-badge" style="background: {item.badgeColor}">{item.badge}</span>
            <span class="mention-text">
              <span class="mention-label">{item.label}</span>
              {#if item.subtitle}<span class="mention-subtitle">{item.subtitle}</span>{/if}
            </span>
          </button>
        {/each}
      </div>
    {:else if query.trim()}
      <div class="mention-empty">{t('mention.no_results')}</div>
    {/if}
  </div>
{/if}

<style>
  .mention-backdrop {
    position: fixed;
    inset: 0;
    z-index: 199;
  }

  .mention-picker {
    position: fixed;
    z-index: 200;
    width: 340px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 24px var(--shadow-lg);
    overflow: hidden;
  }

  .mention-search {
    padding: 0.5rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .mention-input {
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.8rem;
    outline: none;
    box-sizing: border-box;
  }
  .mention-input:focus { border-color: var(--accent); }

  .mention-results {
    max-height: 240px;
    overflow-y: auto;
  }

  .mention-result {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.45rem 0.6rem;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .mention-result:last-child { border-bottom: none; }
  .mention-result:hover, .mention-result.active { background: var(--bg-elevated); }

  .mention-badge {
    font-size: 0.6rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    color: #fff;
    flex-shrink: 0;
  }

  .mention-text {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    flex: 1;
    min-width: 0;
  }

  .mention-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mention-subtitle {
    font-size: 0.65rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mention-empty {
    padding: 0.6rem;
    font-size: 0.8rem;
    color: var(--text-muted);
    text-align: center;
  }
</style>
