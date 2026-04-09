<script lang="ts">
  import { search, boardSearch, nav, dnd, getTagColor, cardTypes, board } from '../lib/store.svelte'
  import { renderInline } from '../lib/markdown'
  import { t } from '../lib/i18n.svelte'
  import { getCardTypeColor, getCardTypeTextColor } from '../lib/cardTypes'
  import { TriggerAgent, CancelAgent } from '../lib/api'
  import { showToast } from '../lib/toast.svelte'
  import { Timer, Play, Square } from 'lucide-svelte'

  type CardData = {
    id: string
    type: string
    title: string
    tags: string[]
    due_date: string | null
    checklist_total: number
    checklist_done: number
  }

  let { card, categoryId, onclick }: { card: CardData; categoryId: string; onclick?: () => void } = $props()

  const draggable = $derived(categoryId !== '__recent__')
  let hasAgent = $derived(!!board.agentCardIds[card.id])
  let isRunning = $derived(!!board.runningAgentIds[card.id])

  function handleDragStart(e: DragEvent) {
    if (!draggable || !e.dataTransfer) return
    e.dataTransfer.effectAllowed = 'copyMove'
    e.dataTransfer.setData('text/plain', card.id)
    dnd.dragging = { type: 'card', cardId: card.id, fromCategoryId: categoryId, cardType: card.type || '' }
  }

  function handleDragEnd() {
    dnd.dragging = null
    dnd.overCategoryId = null
    dnd.overCardIndex = null
  }

  async function handleAgentAction(e: MouseEvent) {
    e.stopPropagation()
    e.preventDefault()
    try {
      if (isRunning) {
        await CancelAgent(card.id)
        showToast(t('agent.cancelled'), 'info')
      } else {
        await TriggerAgent(card.id)
        showToast(t('agent.triggered'), 'success')
      }
    } catch (err) {
      showToast(isRunning ? t('agent.cancel_failed') : t('agent.trigger_failed'), 'error')
    }
  }

</script>

<button
  class="card-item"
  class:search-highlight={nav.inboxMode ? search.matchingIds.has(card.id) : boardSearch.matchingIds.has(card.id)}
  class:search-collapsed={nav.inboxMode
    ? (search.query.trim() && search.matchingIds.size > 0 && !search.matchingIds.has(card.id))
    : (boardSearch.query.trim() && boardSearch.matchingIds.size > 0 && !boardSearch.matchingIds.has(card.id))}
  class:dragging={dnd.dragging?.type === 'card' && dnd.dragging.cardId === card.id}
  class:no-drag={!draggable}
  draggable={draggable}
  ondragstart={handleDragStart}
  ondragend={handleDragEnd}
  onclick={onclick}
>
  {#if hasAgent}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <span
      class="agent-indicator"
      class:running={isRunning}
      role="button"
      tabindex="-1"
      title={isRunning ? t('agent.cancel') : t('agent.run_now')}
      onclick={handleAgentAction}
      onkeydown={(e) => { if (e.key === 'Enter') handleAgentAction(e as any) }}
    >
      {#if isRunning}
        <Square size={10} />
      {:else}
        <Timer size={14} class="icon-bot" />
        <Play size={12} class="icon-play" />
      {/if}
    </span>
  {/if}
  <div class="card-header">
    <span class="type-badge" style="background: {getCardTypeColor(card.type, cardTypes.list)}; color: {getCardTypeTextColor(card.type)}">{cardTypes.list.find(t => t.id === card.type)?.label || card.type || t('card.type_none')}</span>
    {#if card.due_date}
      <span class="due-date">📅 {card.due_date.slice(0, 10)}</span>
    {/if}
  </div>

  <p class="card-title" title={card.title}>{@html renderInline(card.title)}</p>

  <div class="card-footer">
    {#if card.checklist_total > 0}
      <span class="checklist-count" class:all-done={card.checklist_done === card.checklist_total}>
        ✓ {card.checklist_done}/{card.checklist_total}
      </span>
    {/if}

    {#if card.tags.length > 0}
      <div class="tags">
        {#each card.tags.slice(0, 3) as tag}
          <span class="tag" style:background={getTagColor(tag)}>{tag}</span>
        {/each}
        {#if card.tags.length > 3}
          <span class="tag tag-more">+{card.tags.length - 3}</span>
        {/if}
      </div>
    {/if}
  </div>
</button>

<style>
  .card-item {
    position: relative;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    cursor: grab;
    text-align: left;
    width: 100%;
    transition: border-color 0.15s, box-shadow 0.15s, opacity 0.15s;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .card-item.no-drag {
    cursor: default;
  }

  .card-item.dragging {
    opacity: 0.4;
  }

  .card-item:hover {
    border-color: var(--border-hover);
    box-shadow: 0 2px 8px var(--shadow);
  }

  .card-item.search-collapsed {
    opacity: 0.35;
    gap: 0;
  }

  .card-item.search-collapsed .card-header,
  .card-item.search-collapsed .card-footer {
    display: none;
  }

  .card-item.search-highlight {
    border-color: var(--accent-light);
    box-shadow: 0 0 8px var(--accent-glow-1), 0 0 20px var(--accent-glow-2), 0 0 40px var(--accent-glow-3);
  }

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .type-badge {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    color: #fff;
    line-height: 1.4;
  }

  .due-date {
    font-size: 0.7rem;
    color: var(--text-secondary);
  }

  .agent-indicator {
    position: absolute;
    top: 5px;
    right: 5px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--accent) 15%, transparent);
    color: var(--accent);
    z-index: 1;
    cursor: pointer;
    border: none;
    transition: background 0.15s, transform 0.1s;
  }
  .agent-indicator:hover {
    background: color-mix(in srgb, var(--accent) 35%, transparent);
    transform: scale(1.15);
  }

  /* Idle: bot visible, play hidden — swap on hover */
  .agent-indicator :global(.icon-play) { display: none; }
  .agent-indicator:not(.running):hover :global(.icon-bot) { display: none; }
  .agent-indicator:not(.running):hover :global(.icon-play) { display: block; }

  /* Running: neon AI gradient glow animation */
  .agent-indicator.running {
    background: linear-gradient(135deg, #6366f1, #06b6d4, #a855f7, #6366f1);
    background-size: 300% 300%;
    animation: agent-neon 2s ease infinite;
    color: white;
    box-shadow: 0 0 6px rgba(99, 102, 241, 0.5), 0 0 12px rgba(168, 85, 247, 0.3);
  }
  .agent-indicator.running:hover {
    background: #eb5a46;
    animation: none;
    box-shadow: 0 0 6px rgba(235, 90, 70, 0.5);
    transform: scale(1.15);
  }

  @keyframes agent-neon {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
  }

  .card-title {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-strong);
    line-height: 1.3;
    font-weight: 400;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-footer {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    min-height: 1.25rem;
  }

  .checklist-count {
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  .checklist-count.all-done {
    color: var(--success);
  }

  .tags {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }

  .tag {
    font-size: 0.6rem;
    padding: 0.05rem 0.35rem;
    border-radius: 3px;
    background: var(--border);
    color: #fff;
  }

  .tag-more {
    background: transparent;
    color: var(--text-muted);
  }
</style>
