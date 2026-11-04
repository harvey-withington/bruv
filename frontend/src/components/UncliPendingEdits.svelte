<script lang="ts">
  // The UNCLI chat's review banner (in its header-extra slot): the edits
  // its Suggest mode staged on the chat's history, "N edits waiting for
  // review", opening to a checklist that applies the ticked ones and
  // rejects the rest (shared/pendingEdits). Hidden while nothing waits.
  import { untrack } from 'svelte'
  import { ListChecks } from 'lucide-svelte'
  import type { ChatHistory, ChatMessage, PendingEdit } from '@shared/types'
  import { errorDetail } from '@shared/errors'
  import { failedEditCount, pendingEditGroups } from '@shared/pendingEdits'
  import { showToast } from '../lib/toast.svelte'
  import { t } from '../lib/i18n.svelte'

  let {
    load,
    apply,
    reloadKey,
    onApplied,
  }: {
    /** The chat's history (LoadChatHistory / LoadProjectChatHistory). */
    load: () => Promise<ChatHistory>
    /** Apply(Project)PendingEdits: accepts acceptIDs, rejects the rest. */
    apply: (messageId: string, acceptIDs: string[]) => Promise<ChatHistory>
    /** Reloads when it changes (a turn finished, the chat changed). */
    reloadKey: unknown
    /** Edits were applied: the card or board may need reloading. */
    onApplied?: () => void
  } = $props()

  let messages = $state<ChatMessage[]>([])
  let open = $state(false)
  let busy = $state(false)
  let checked = $state<Record<string, boolean>>({})

  const groups = $derived(pendingEditGroups<PendingEdit, ChatMessage>(messages))
  const count = $derived(groups.reduce((n, g) => n + g.edits.length, 0))

  async function reload() {
    try {
      messages = (await load())?.messages ?? []
      for (const g of groups) for (const e of g.edits) checked[e.id] ??= true
    } catch (e) {
      showToast(t('uncli.review_load_failed', { detail: errorDetail(e) }), 'error')
    }
  }
  $effect(() => {
    void reloadKey
    untrack(() => void reload())
  })

  async function resolve(acceptChecked: boolean) {
    busy = true
    const accepted: string[] = []
    try {
      for (const g of groups) {
        const ids = acceptChecked ? g.edits.filter(e => checked[e.id]).map(e => e.id) : []
        accepted.push(...ids)
        messages = (await apply(g.messageId, ids))?.messages ?? messages
      }
      const failed = failedEditCount<PendingEdit, ChatMessage>(messages, accepted)
      if (failed) showToast(t('error.edit_apply_some_failed', { count: failed }), 'error')
      if (accepted.length) onApplied?.()
      open = false
    } catch (e) {
      showToast(t('uncli.review_apply_failed', { detail: errorDetail(e) }), 'error')
    } finally {
      busy = false
      await reload()
    }
  }
</script>

{#if count > 0}
  <div class="review">
    <button type="button" class="review-toggle" aria-expanded={open} onclick={() => (open = !open)}>
      <ListChecks size={13} />
      {t('uncli.review_waiting', { count })}
    </button>
    {#if open}
      <ul class="review-list">
        {#each groups as g (g.messageId)}
          {#each g.edits as e (e.id)}
            <li>
              <label title={e.detail || e.label}>
                <input type="checkbox" bind:checked={checked[e.id]} disabled={busy} />
                <span>{e.label}</span>
              </label>
            </li>
          {/each}
        {/each}
      </ul>
      <div class="review-actions">
        <button type="button" class="btn" onclick={() => resolve(false)} disabled={busy}>{t('uncli.review_reject_all')}</button>
        <button type="button" class="btn primary" onclick={() => resolve(true)} disabled={busy || !groups.some(g => g.edits.some(e => checked[e.id]))}>
          {t('uncli.review_apply')}
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .review {
    margin: 0.4rem 0.75rem;
    padding: 0.4rem 0.6rem;
    border: 1px solid color-mix(in srgb, var(--warning) 50%, transparent);
    border-radius: 6px;
    background: var(--bg-elevated);
    font-size: 0.78rem;
    color: var(--text-body);
  }
  .review-toggle {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    width: 100%;
    padding: 0;
    border: none;
    background: none;
    color: var(--warning);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
  }
  .review-list {
    list-style: none;
    margin: 0.4rem 0 0;
    padding: 0;
    max-height: 12rem;
    overflow-y: auto;
  }
  .review-list label {
    display: flex;
    align-items: flex-start;
    gap: 0.4rem;
    padding: 0.15rem 0;
    cursor: pointer;
  }
  .review-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }
</style>
