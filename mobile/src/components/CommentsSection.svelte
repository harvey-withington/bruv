<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Send, Trash2, Pencil, Check, X } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { renderMarkdown } from '@shared/markdown'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import type { CardComment } from '@shared/types'

  // Card comments. Self-contained: lists, adds, edits, deletes via the
  // existing RPCs. The list refreshes on add/edit/delete from the user
  // here; SSE-driven external updates aren't wired (no comment-specific
  // event topic) — the user can re-open the card to refresh.

  let { cardId }: { cardId: string } = $props()

  let comments = $state<CardComment[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)

  let draft = $state('')
  let posting = $state(false)
  let textareaEl: HTMLTextAreaElement | null = $state(null)

  let editingId = $state<string | null>(null)
  let editDraft = $state('')

  let confirmingDelete = $state<string | null>(null)

  async function reload() {
    try {
      comments = (await repoRPC<CardComment[]>('ListCardComments', [cardId])) ?? []
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('comments.err_load')
    } finally {
      loading = false
    }
  }

  onMount(reload)

  async function add() {
    const text = draft.trim()
    if (!text || posting) return
    posting = true
    errorMsg = null
    try {
      // Author is empty — backend fills in the user's profile name
      // (or "anonymous" if profile isn't set). Mobile doesn't have a
      // profile editor yet so we don't attempt to override.
      const created = await repoRPC<CardComment>('AddCardComment', [cardId, '', text])
      if (created) comments = [...comments, created]
      draft = ''
      autoGrow()
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('comments.err_post')
    } finally {
      posting = false
    }
  }

  function startEdit(c: CardComment) {
    editingId = c.id
    editDraft = c.text
  }
  function cancelEdit() {
    editingId = null
    editDraft = ''
  }
  async function saveEdit(c: CardComment) {
    const next = editDraft.trim()
    if (!next || next === c.text) {
      cancelEdit()
      return
    }
    try {
      const updated = await repoRPC<CardComment>('UpdateCardComment', [cardId, c.id, next])
      if (updated) comments = comments.map((x) => (x.id === c.id ? updated : x))
      cancelEdit()
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('comments.err_save')
    }
  }

  async function performDelete(id: string) {
    confirmingDelete = null
    try {
      await repoRPC('DeleteCardComment', [cardId, id])
      comments = comments.filter((x) => x.id !== id)
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('comments.err_delete')
    }
  }

  function autoGrow() {
    if (!textareaEl) return
    textareaEl.style.height = 'auto'
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, 160)}px`
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      void add()
    }
  }

  function formatTime(iso: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    return d.toLocaleString(undefined, {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    })
  }
</script>

<section class="comments">
  <h3 class="section-title">{t('comments.title')}{comments.length > 0 ? ` (${comments.length})` : ''}</h3>

  {#if loading}
    <p class="status">{t('common.loading')}</p>
  {:else if errorMsg && comments.length === 0}
    <p class="error">{errorMsg}</p>
  {/if}

  {#if comments.length > 0}
    <ul class="list">
      {#each comments as c (c.id)}
        <li class="comment">
          {#if editingId === c.id}
            <textarea
              class="edit-area"
              bind:value={editDraft}
              rows="3"
              onkeydown={(e) => {
                if (e.key === 'Escape') { e.preventDefault(); cancelEdit() }
                if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); void saveEdit(c) }
              }}
              ></textarea>
            <div class="edit-actions">
              <button type="button" class="ghost-btn" onclick={cancelEdit} aria-label={t('common.cancel')}>
                <X size={14} />
              </button>
              <button type="button" class="primary-btn" onclick={() => saveEdit(c)} aria-label={t('comments.save')}>
                <Check size={14} />
              </button>
            </div>
          {:else}
            <div class="meta">
              <span class="author">{c.author || t('comments.anonymous')}</span>
              <span class="time">{formatTime(c.created_at)}</span>
              <span class="meta-spacer"></span>
              <button type="button" class="ghost-btn" onclick={() => startEdit(c)} aria-label={t('comments.edit')}>
                <Pencil size={12} />
              </button>
              <button type="button" class="ghost-btn danger" onclick={() => (confirmingDelete = c.id)} aria-label={t('comments.delete')}>
                <Trash2 size={12} />
              </button>
            </div>
            <div class="body">{@html renderMarkdown(c.text)}</div>
          {/if}
        </li>
      {/each}
    </ul>
  {:else if !loading}
    <p class="empty">{t('comments.empty')}</p>
  {/if}

  <div class="composer">
    <textarea
      bind:this={textareaEl}
      bind:value={draft}
      oninput={autoGrow}
      onkeydown={handleKey}
      placeholder={t('comments.placeholder')}
      rows="2"
      disabled={posting}
    ></textarea>
    <button
      type="button"
      class="send-btn"
      onclick={add}
      disabled={!draft.trim() || posting}
      aria-label={t('comments.add')}
    >
      <Send size={16} />
    </button>
  </div>

  {#if errorMsg && comments.length > 0}
    <p class="error inline">{errorMsg}</p>
  {/if}
</section>

{#if confirmingDelete}
  <ConfirmDialog
    title={t('comments.delete_title')}
    body={t('comments.delete_body')}
    confirmLabel={t('comments.delete')}
    destructive
    onConfirm={() => performDelete(confirmingDelete!)}
    onCancel={() => (confirmingDelete = null)}
  />
{/if}

<style>
  .comments {
    border-top: 1px solid var(--border);
    padding-top: 1.25rem;
    margin-bottom: 1.5rem;
  }
  .section-title {
    margin: 0 0 0.6rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .list {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .comment {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0.65rem 0.85rem;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-bottom: 0.35rem;
  }
  .author {
    font-weight: 600;
    color: var(--text);
  }
  .time {
    color: var(--text-faint);
  }
  .meta-spacer {
    flex: 1;
  }
  .body {
    font-size: 0.92rem;
    line-height: 1.45;
    color: var(--text);
  }
  .body :global(p) {
    margin: 0 0 0.4rem;
  }
  .body :global(p:last-child) {
    margin-bottom: 0;
  }
  .body :global(a) {
    color: var(--accent);
  }
  .body :global(code) {
    background: var(--bg);
    padding: 0.05rem 0.25rem;
    border-radius: 3px;
    font-size: 0.85em;
  }

  .ghost-btn {
    background: transparent;
    border: 1px solid transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.3rem 0.4rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ghost-btn:hover,
  .ghost-btn:focus-visible {
    color: var(--text);
    background: var(--bg);
    outline: none;
  }
  .ghost-btn.danger:hover,
  .ghost-btn.danger:focus-visible {
    color: #ef4444;
  }
  .primary-btn {
    background: var(--accent);
    border: none;
    color: #18181b;
    cursor: pointer;
    padding: 0.3rem 0.5rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .edit-area {
    width: 100%;
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    padding: 0.4rem 0.55rem;
    resize: vertical;
    min-height: 4rem;
  }
  .edit-area:focus {
    outline: none;
  }
  .edit-actions {
    display: flex;
    gap: 0.4rem;
    justify-content: flex-end;
    margin-top: 0.35rem;
  }

  .composer {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
  }
  .composer textarea {
    flex: 1;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    font: inherit;
    font-size: 0.92rem;
    line-height: 1.4;
    padding: 0.55rem 0.7rem;
    resize: none;
    min-height: 48px;
    max-height: 160px;
    overflow-y: auto;
  }
  .composer textarea:focus {
    outline: none;
    border-color: var(--accent);
  }
  .send-btn {
    background: var(--accent);
    border: none;
    color: #18181b;
    border-radius: 10px;
    width: 44px;
    height: 44px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .send-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
    font-style: italic;
  }
  .status {
    color: var(--text-muted);
    font-size: 0.85rem;
    margin: 0 0 0.75rem;
  }
  .error {
    margin: 0 0 0.75rem;
    padding: 0.5rem 0.75rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 6px;
    color: #fca5a5;
    font-size: 0.85rem;
  }
  .error.inline {
    margin-top: 0.5rem;
    margin-bottom: 0;
  }
</style>
