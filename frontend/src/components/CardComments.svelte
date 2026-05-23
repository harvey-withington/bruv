<script lang="ts">
  import { onMount } from 'svelte'
  import { Pencil, Trash2, Check, X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import {
    ListCardComments,
    AddCardComment,
    UpdateCardComment,
    DeleteCardComment,
  } from '@shared/api'
  import type { CardComment } from '@shared/types'

  let { cardId, count = $bindable(0) }: { cardId: string; count?: number } = $props()

  let comments = $state<CardComment[]>([])
  let draft = $state('')
  let posting = $state(false)
  let editingId = $state<string | null>(null)
  let editDraft = $state('')

  $effect(() => {
    count = comments.length
  })

  onMount(load)

  // Reload when the card changes (modal stays mounted across cards).
  $effect(() => {
    if (cardId) load()
  })

  async function load() {
    if (!cardId) return
    try {
      comments = (await ListCardComments(cardId)) || []
    } catch (e) {
      console.error('load comments', e)
      showToast(t('comments.load_failed'), 'error')
    }
  }

  async function post() {
    const text = draft.trim()
    if (!text || posting) return
    posting = true
    try {
      const created = await AddCardComment(cardId, '', text)
      comments = [...comments, created]
      draft = ''
    } catch (e) {
      console.error('post comment', e)
      showToast(t('comments.post_failed'), 'error')
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
    const text = editDraft.trim()
    if (!text) return
    try {
      const updated = await UpdateCardComment(cardId, c.id, text)
      comments = comments.map(x => x.id === c.id ? updated : x)
      cancelEdit()
    } catch (e) {
      console.error('edit comment', e)
      showToast(t('comments.edit_failed'), 'error')
    }
  }

  async function remove(c: CardComment) {
    const ok = await showConfirm(t('comments.confirm_delete'))
    if (!ok) return
    try {
      await DeleteCardComment(cardId, c.id)
      comments = comments.filter(x => x.id !== c.id)
    } catch (e) {
      console.error('delete comment', e)
      showToast(t('comments.delete_failed'), 'error')
    }
  }

  // Ctrl/Cmd+Enter inside the comment composer must NOT bubble — the parent
  // CardDetail modal treats it as "save and close", which would steal focus
  // and dismiss the card before the comment is posted.
  function handleComposerKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()
      post()
    }
  }

  function handleEditKeydown(e: KeyboardEvent, c: CardComment) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      e.stopPropagation()
      saveEdit(c)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      cancelEdit()
    }
  }

  function formatTs(iso: string): string {
    if (!iso) return ''
    try {
      const d = new Date(iso)
      return d.toLocaleString()
    } catch {
      return iso
    }
  }

  function wasEdited(c: CardComment): boolean {
    return !!(c.updated_at && c.created_at && c.updated_at !== c.created_at)
  }
</script>

<section class="comments-section">

  {#if comments.length === 0}
    <p class="comments-empty">{t('comments.empty')}</p>
  {:else}
    <ul class="comments-list">
      {#each comments as c (c.id)}
        <li class="comment action-reveal-parent">
          <div class="comment-meta">
            <span class="comment-author">{c.author}</span>
            <span class="comment-time">{formatTs(c.created_at)}</span>
            {#if wasEdited(c)}
              <span class="comment-edited">{t('comments.edited_suffix')}</span>
            {/if}
            {#if editingId !== c.id}
              <div class="comment-actions action-reveal">
                <button class="comment-action-btn" title={t('comments.edit')} onclick={() => startEdit(c)}>
                  <Pencil size={12} />
                </button>
                <button class="comment-action-btn action-reveal--danger" title={t('comments.delete')} onclick={() => remove(c)}>
                  <Trash2 size={12} />
                </button>
              </div>
            {/if}
          </div>
          {#if editingId === c.id}
            <textarea
              class="comment-edit-input"
              bind:value={editDraft}
              onkeydown={(e) => handleEditKeydown(e, c)}
              rows="3"
            ></textarea>
            <div class="comment-edit-actions">
              <button class="comment-edit-cancel" onclick={cancelEdit} title={t('comments.cancel')}>
                <X size={12} /> {t('comments.cancel')}
              </button>
              <button class="comment-edit-save" onclick={() => saveEdit(c)} title={t('comments.save')}>
                <Check size={12} /> {t('comments.save')}
              </button>
            </div>
          {:else}
            <p class="comment-body">{c.text}</p>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <div class="comments-composer">
    <textarea
      class="comment-input"
      bind:value={draft}
      onkeydown={handleComposerKeydown}
      placeholder={t('comments.add_placeholder')}
      rows="2"
      disabled={posting}
    ></textarea>
    <div class="comments-composer-footer">
      <span class="comments-hint">{t('comments.add_hint')}</span>
      <button class="comments-post" onclick={post} disabled={posting || !draft.trim()}>
        {t('comments.post')}
      </button>
    </div>
  </div>
</section>

<style>
  .comments-section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.35rem 0;
    height: 100%;
    min-height: 0;
  }


  .comments-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0;
    flex: 1;
  }

  .comments-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    flex: 1;
    overflow-y: auto;
    padding-right: 0.25rem;
  }

  .comment {
    background: var(--bg-elevated);
    border-radius: 6px;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--border);
  }

  .comment-meta {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-bottom: 0.25rem;
  }
  .comment-author {
    font-weight: 600;
    color: var(--text-body);
  }
  .comment-time { font-size: 0.7rem; color: var(--text-faint); }
  .comment-edited { font-size: 0.7rem; color: var(--text-faint); font-style: italic; }

  .comment-actions {
    margin-left: auto;
    display: flex;
    gap: 0.15rem;
  }
  .comment-action-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.15rem;
    display: flex;
    align-items: center;
    border-radius: 3px;
  }
  .comment-action-btn:hover { color: var(--text-primary); background: var(--bg-surface); }
  .comment-action-btn.action-reveal--danger:hover { color: var(--danger, #e53935); }

  .comment-body {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-primary);
    white-space: pre-wrap;
    word-wrap: break-word;
  }

  .comment-edit-input {
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--accent);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
  }

  .comment-edit-actions {
    display: flex;
    gap: 0.3rem;
    justify-content: flex-end;
    margin-top: 0.3rem;
  }
  .comment-edit-cancel,
  .comment-edit-save {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.55rem;
    border-radius: 4px;
    font-size: 0.75rem;
    cursor: pointer;
    border: 1px solid var(--border);
    background: var(--bg-surface);
    color: var(--text-body);
    font-family: inherit;
  }
  .comment-edit-save {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .comment-edit-save:hover { filter: brightness(1.1); }
  .comment-edit-cancel:hover { background: var(--bg-elevated); }

  .comments-composer {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .comment-input {
    width: 100%;
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
  }
  .comment-input:focus { border-color: var(--accent); }
  .comment-input:disabled { opacity: 0.6; }

  .comments-composer-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .comments-hint {
    font-size: 0.7rem;
    color: var(--text-faint);
  }
  .comments-post {
    padding: 0.3rem 0.8rem;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    border: none;
    font-size: 0.8rem;
    font-family: inherit;
    cursor: pointer;
  }
  .comments-post:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .comments-post:not(:disabled):hover { filter: brightness(1.1); }
</style>
