<script lang="ts">
  import { ListCardComments } from '@shared/api'
  import { ChevronDown, ChevronRight, Paperclip, MessageSquare } from 'lucide-svelte'
  import CardAttachments from './CardAttachments.svelte'
  import CardComments from './CardComments.svelte'
  import BlockPicker from './BlockPicker.svelte'
  import { t } from '../lib/i18n.svelte'
  import type { Block, Card } from '@shared/types'

  // The attachments/comments strip pinned between the card body and
  // footer. Owns its tab + collapse state (always starts collapsed on
  // every card open — nothing persists) and the comment-count badge.
  // The add-block button rides on this header row, so BlockPicker
  // renders here; what "add" means stays with the parent via onAddBlock.

  let { cardId, card, onAttachmentsUpdated, onAddBlock }: {
    cardId: string
    card: Card
    /** CardAttachments returns the updated card after upload/remove. */
    onAttachmentsUpdated: (card: Card) => void
    onAddBlock: (type: Block['type'], label: string) => void
  } = $props()

  let collapsed = $state(true)
  // Re-collapse when the dialog instance is reused for another card —
  // switching cards counts as a fresh open.
  $effect(() => {
    void cardId
    collapsed = true
  })

  let activeTab = $state<'attachments' | 'comments'>('attachments')

  // Eager count for the badge — CardComments only reports (bind:count)
  // while the comments tab is open, but the badge shows regardless.
  // Keyed on the card reference so every reload (incl. silent live-
  // update refetches) refreshes the count.
  let commentCount = $state(0)
  $effect(() => {
    void card
    ListCardComments(cardId)
      .then(comments => { commentCount = (comments || []).length })
      .catch(e => console.error('load comments count', e))
  })
</script>

<div class="card-meta-tabs-pinned" class:collapsed>
  <div class="meta-tabs-header">
    <div class="meta-tabs-buttons">
      <button
        class="meta-collapse-btn"
        onclick={() => collapsed = !collapsed}
        title={collapsed ? t('tooltip.expand_block') : t('tooltip.collapse_block')}
      >
        {#if collapsed}
          <ChevronRight size={14} />
        {:else}
          <ChevronDown size={14} />
        {/if}
      </button>

      <button
        class="meta-tab-btn"
        class:active={activeTab === 'attachments'}
        onclick={() => { activeTab = 'attachments'; collapsed = false }}
      >
        <Paperclip size={13} />
        <span>{t('attachment.title')}</span>
        {#if card.file_attachments?.length > 0}
          <span class="meta-count">{card.file_attachments.length}</span>
        {/if}
      </button>

      <button
        class="meta-tab-btn"
        class:active={activeTab === 'comments'}
        onclick={() => { activeTab = 'comments'; collapsed = false }}
      >
        <MessageSquare size={13} />
        <span>{t('comments.title')}</span>
        {#if commentCount > 0}
          <span class="meta-count">{commentCount}</span>
        {/if}
      </button>
    </div>

    <BlockPicker onAdd={onAddBlock} />
  </div>

  {#if !collapsed}
    <div class="meta-tab-content">
      {#if activeTab === 'attachments'}
        <CardAttachments
          {cardId}
          attachments={card.file_attachments || []}
          onCardUpdated={onAttachmentsUpdated}
        />
      {:else if activeTab === 'comments'}
        <CardComments
          {cardId}
          bind:count={commentCount}
        />
      {/if}
    </div>
  {/if}
</div>

<style>
  .card-meta-tabs-pinned {
    flex-shrink: 0;
    padding: 0.5rem 1.25rem 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
    background: var(--bg-surface);
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    box-shadow: 0 -4px 12px var(--shadow);
    position: relative;
    z-index: 1;
  }
  .card-meta-tabs-pinned:has(:global(.preview-overlay)) {
    z-index: 99999;
  }
  .card-meta-tabs-pinned.collapsed {
    padding-bottom: 0.5rem;
  }

  .meta-tabs-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border-muted);
    position: relative;
  }

  .meta-collapse-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.25rem;
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border-radius: 4px;
    margin-right: 0.15rem;
    transition: background var(--duration-fast), color var(--duration-fast);
  }
  .meta-collapse-btn:hover {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }

  .meta-tabs-buttons {
    display: flex;
    gap: 0.25rem;
  }

  .meta-tab-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 0.5rem 0.6rem;
    display: flex;
    align-items: center;
    gap: 0.3rem;
    position: relative;
    transition: color var(--duration-fast);
  }

  .meta-tab-btn::after {
    content: '';
    position: absolute;
    bottom: -1px;
    left: 0;
    right: 0;
    height: 2px;
    background: transparent;
    transition: background var(--duration-fast);
  }

  .meta-tab-btn:hover {
    color: var(--text-primary);
  }

  .meta-tab-btn.active {
    color: var(--accent);
  }

  .meta-tab-btn.active::after {
    background: var(--accent);
  }

  .meta-count {
    font-size: 0.65rem;
    font-weight: 600;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0 0.3rem;
    color: var(--text-muted);
    margin-left: 0.15rem;
  }

  .meta-tab-btn.active .meta-count {
    border-color: var(--accent);
    color: var(--accent);
  }

  .meta-tab-content {
    height: 12rem;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
</style>
