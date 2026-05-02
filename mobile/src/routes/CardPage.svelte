<script lang="ts">
  import { onMount } from 'svelte'
  import { repoRPC } from '../lib/auth'
  import { navigate } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'
  import { Trash2, MapPin, Plus, X } from 'lucide-svelte'
  import EditableText from '../components/EditableText.svelte'
  import TagsEditor from '../components/TagsEditor.svelte'
  import BlockView from '../components/BlockView.svelte'
  import ConfirmDialog from '../components/ConfirmDialog.svelte'
  import PinPicker from '../components/PinPicker.svelte'
  import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from '@shared/cardTypes'
  import { renderMarkdown } from '@shared/markdown'
  import { repoMeta, loadProjectTags, projectKey as makeProjectKey } from '../lib/repoMeta.svelte'
  import type { Card, CardPin } from '@shared/types'

  // Card view + basic edit. Phase 1 scope:
  //   - title  (editable inline)
  //   - tags   (editable chip list)
  //   - blocks (rendered read-only via BlockView)
  //   - type, due_date  (display only — date pickers + type pickers
  //     are their own UX questions, not v1 critical)
  //
  // Block editing, comments, attachments, pin/unpin, agent config —
  // all later passes. Mobile-first, but mobile-not-everything.

  let { id }: { id: string } = $props()

  let card = $state<Card | null>(null)
  let projectKey = $state<string | undefined>(undefined)
  let pins = $state<CardPin[]>([])
  let pinPickerOpen = $state(false)
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)
  let saveError = $state<string | null>(null)

  onMount(async () => {
    try {
      card = await repoRPC<Card>('GetCard', [id])
      // Resolve the card's primary pin so we can load that project's
      // tag definitions for accurate chip colours. Orphaned cards (no
      // pin) fall back to the global tag colour map. Best-effort —
      // failures here just mean grey tags.
      try {
        const loc = await repoRPC<{ brandSlug: string; streamSlug: string; projectSlug: string }>(
          'GetCardLocation',
          [id],
        )
        if (loc?.brandSlug && loc.streamSlug && loc.projectSlug) {
          projectKey = makeProjectKey(loc.brandSlug, loc.streamSlug, loc.projectSlug)
          loadProjectTags(loc.brandSlug, loc.streamSlug, loc.projectSlug)
        }
      } catch {
        /* unpinned card or RPC error — global tag colours only */
      }
      // Pin breadcrumbs power the "Pinned in" section + per-pin unpin.
      // Orphan cards just get an empty array.
      try {
        pins = (await repoRPC<CardPin[]>('GetCardPinBreadcrumbs', [id])) ?? []
      } catch {
        pins = []
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('card.err_load')
    } finally {
      loading = false
    }
  })

  // --- Pin / unpin ---

  async function refreshPins() {
    try {
      pins = (await repoRPC<CardPin[]>('GetCardPinBreadcrumbs', [id])) ?? []
    } catch {
      /* leave the previous list visible on transient errors */
    }
  }

  async function pinTo(sel: { project: { id: string }; category: { id: string } }) {
    if (!card) return
    pinPickerOpen = false
    saveError = null
    try {
      await repoRPC('PinCard', [card.id, sel.project.id, sel.category.id])
      await refreshPins()
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('card.err_pin')
    }
  }

  async function unpinAt(pin: CardPin) {
    if (!card) return
    saveError = null
    // Optimistic remove — restore on error so the UI reflects reality.
    const previous = pins
    pins = pins.filter((p) => p !== pin)
    try {
      await repoRPC('UnpinCard', [card.id, pin.projectId, pin.categoryId])
    } catch (err) {
      pins = previous
      saveError = err instanceof Error ? err.message : t('card.err_unpin')
    }
  }

  // --- Edit handlers ---
  //
  // Each handler optimistically updates local state, calls the RPC,
  // and reverts on failure with a visible error. Keeps the UI snappy
  // while still surfacing real save problems.

  async function saveTitle(next: string) {
    if (!card) return
    const previous = card.title
    card.title = next
    saveError = null
    try {
      await repoRPC('UpdateCardTitle', [card.id, next])
    } catch (err) {
      card.title = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  async function saveTags(next: string[]) {
    if (!card) return
    const previous = card.tags
    card.tags = next
    saveError = null
    try {
      await repoRPC('UpdateCardTags', [card.id, next])
    } catch (err) {
      card.tags = previous
      saveError = err instanceof Error ? err.message : t('card.err_save')
    }
  }

  function formatDueDate(due: string | null): string {
    if (!due) return ''
    const d = new Date(due)
    if (Number.isNaN(d.getTime())) return due
    return d.toLocaleDateString()
  }

  // --- Delete ---

  let confirmingDelete = $state(false)

  async function deleteCard() {
    if (!card) return
    try {
      await repoRPC<void>('DeleteCard', [card.id])
      // Pop the route stack so the user lands wherever they came from
      // (Browse / Project / Inbox) rather than seeing the now-stale
      // detail page briefly. Falls back to / if there's no history
      // (e.g. they hit the card via deep link).
      if (window.history.length > 1) {
        history.back()
      } else {
        navigate('/')
      }
    } catch (err) {
      saveError = err instanceof Error ? err.message : t('card.err_delete')
      confirmingDelete = false
    }
  }
</script>

<header class="topbar">
  <button type="button" class="back" onclick={() => history.back()}>
    <span aria-hidden="true">‹</span> {t('common.back')}
  </button>
  <span class="topbar-title" title={card?.title ?? ''}>
    {card?.title ?? t('common.loading')}
  </span>
  <span class="spacer"></span>
</header>

<main style:view-transition-name={`card-${id}`}>
  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg}
    <p class="error">{errorMsg}</p>
  {:else if card}
    <section class="meta">
      <EditableText
        value={card.title}
        placeholder={t('card.untitled')}
        ariaLabel={t('card.edit_title')}
        className="title-field"
        onSave={saveTitle}
      />

      <div class="meta-row">
        {#if card.type}
          <span
            class="type-badge"
            style:background={getCardTypeColor(card.type, repoMeta.cardTypes)}
            style:color={getCardTypeTextColor(card.type)}
          >
            {getCardTypeLabel(card.type, repoMeta.cardTypes)}
          </span>
        {/if}
        {#if card.due_date}
          <span class="due">{t('card.due')} {formatDueDate(card.due_date)}</span>
        {/if}
      </div>

      <TagsEditor tags={card.tags ?? []} {projectKey} onChange={saveTags} />

      <section class="pins">
        <h3 class="pins-label">
          <MapPin size={12} />
          {t('card.pinned_in')}
        </h3>
        {#if pins.length === 0}
          <p class="pins-empty">{t('card.no_pins')}</p>
        {:else}
          <ul class="pin-list">
            {#each pins as pin (pin.projectId + ':' + pin.categoryId)}
              <li class="pin">
                <span class="pin-text">{pin.breadcrumb}</span>
                <button
                  type="button"
                  class="pin-remove"
                  onclick={() => unpinAt(pin)}
                  aria-label={t('card.unpin')}
                  title={t('card.unpin')}
                >
                  <X size={12} />
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <button type="button" class="pin-add" onclick={() => (pinPickerOpen = true)}>
          <Plus size={12} />
          {t('card.pin_to')}
        </button>
      </section>

      {#if saveError}
        <div class="save-error" role="alert">{saveError}</div>
      {/if}
    </section>

    {#if card.description}
      <section class="description">
        <h3 class="section-title">{t('card.description')}</h3>
        <!-- shared/markdown.ts: marked + custom link renderer; output
             is trusted markdown HTML, safe to inject. Single newlines
             become <br> via marked's `breaks: true`, so multi-line
             text from a share preserves its structure. -->
        <div class="prose">{@html renderMarkdown(card.description)}</div>
      </section>
    {/if}

    {#if card.blocks?.length}
      <section class="blocks">
        {#each card.blocks as block (block.id)}
          <BlockView {block} />
        {/each}
      </section>
    {:else if !card.description}
      <p class="no-blocks">{t('card.no_blocks')}</p>
    {/if}

    <footer class="actions">
      <button type="button" class="ghost" onclick={() => navigate('/')}>
        {t('card.back_to_browse')}
      </button>
      <button type="button" class="danger-link" onclick={() => (confirmingDelete = true)}>
        <Trash2 size={14} />
        {t('card.delete')}
      </button>
    </footer>
  {/if}
</main>

{#if confirmingDelete && card}
  <ConfirmDialog
    title={t('card.delete_confirm_title')}
    body={t('card.delete_confirm_body', { title: card.title || t('card.untitled') })}
    confirmLabel={t('card.delete')}
    destructive
    onConfirm={deleteCard}
    onCancel={() => (confirmingDelete = false)}
  />
{/if}

{#if pinPickerOpen}
  <PinPicker onSelect={pinTo} onClose={() => (pinPickerOpen = false)} />
{/if}

<style>
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .topbar-title {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text);
    text-align: center;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60vw;
  }

  .back {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    justify-self: start;
  }

  .back:hover,
  .back:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  main {
    padding: 1rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .meta {
    margin-bottom: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  /* The EditableText sets width:100% on its input/button — these rules
     just make the title visually larger when not editing. */
  .meta :global(.title-field) {
    font-size: 1.4rem;
    font-weight: 600;
    padding: 0.4rem 0.5rem;
  }

  .meta-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }

  .type-badge {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    /* background + color set inline via style: bindings */
    padding: 0.2rem 0.55rem;
    border-radius: 4px;
    font-weight: 500;
  }

  .due {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .pins {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .pins-label {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }

  .pins-empty {
    margin: 0;
    color: var(--text-faint);
    font-size: 0.85rem;
    font-style: italic;
  }

  .pin-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .pin {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.55rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .pin-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }

  .pin-remove {
    background: transparent;
    border: none;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.15rem;
    border-radius: 4px;
    display: inline-flex;
    flex-shrink: 0;
  }
  .pin-remove:hover,
  .pin-remove:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.1);
    outline: none;
  }

  .pin-add {
    align-self: flex-start;
    background: transparent;
    border: 1px dashed var(--border);
    color: var(--text-muted);
    padding: 0.35rem 0.7rem;
    border-radius: 6px;
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
  }
  .pin-add:hover,
  .pin-add:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .save-error {
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    font-size: 0.8rem;
  }

  .description {
    border-top: 1px solid var(--border);
    padding-top: 1.25rem;
    margin-bottom: 1.5rem;
  }

  .section-title {
    margin: 0 0 0.5rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .prose {
    font-size: 0.95rem;
    line-height: 1.55;
    color: var(--text);
  }
  .prose :global(p) {
    margin: 0 0 0.75rem;
  }
  .prose :global(p:last-child) {
    margin-bottom: 0;
  }
  .prose :global(a) {
    color: var(--accent);
    word-break: break-word;
  }
  .prose :global(code) {
    background: var(--bg-elev-1);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .prose :global(pre) {
    background: var(--bg-elev-1);
    padding: 0.65rem;
    border-radius: 6px;
    overflow-x: auto;
  }
  .prose :global(pre code) {
    background: transparent;
    padding: 0;
  }
  .prose :global(blockquote) {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }
  .prose :global(ul),
  .prose :global(ol) {
    padding-left: 1.25rem;
    margin: 0.5rem 0;
  }

  .blocks {
    border-top: 1px solid var(--border);
    padding-top: 1.25rem;
    margin-bottom: 2rem;
  }

  .no-blocks {
    color: var(--text-faint);
    font-size: 0.85rem;
    font-style: italic;
    text-align: center;
    margin: 1.5rem 0;
  }

  .actions {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1.5rem;
  }

  .ghost {
    background: transparent;
    color: var(--text-muted);
    border: 1px solid var(--border);
    padding: 0.55rem 1rem;
    border-radius: 6px;
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .ghost:hover,
  .ghost:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }

  .danger-link {
    background: transparent;
    border: none;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .danger-link:hover,
  .danger-link:focus-visible {
    color: #ef4444;
    background: rgba(239, 68, 68, 0.08);
    outline: none;
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
