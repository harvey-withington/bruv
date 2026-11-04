<script lang="ts">
  // The UNCLI chat's review banner (in its header-extra slot): the edits
  // its Suggest mode staged on the chat's history, "N edits waiting for
  // review", opening to a checklist that applies the ticked ones and
  // rejects the rest (shared/pendingEdits). Hidden while nothing waits.
  import { untrack } from 'svelte'
  import { ListChecks } from 'lucide-svelte'
  import { errorDetail } from '@shared/errors'
  import { failedEditCount, pendingEditGroups } from '@shared/pendingEdits'
  import type { ChatFile, ChatMessage, PendingEdit } from './types'
  import { t } from '../../lib/i18n.svelte'

  let {
    load,
    apply,
    reloadKey,
    onerror,
  }: {
    /** The chat's history (LoadChatHistory / LoadProjectChatHistory). */
    load: () => Promise<ChatFile>
    /** Apply(Project)PendingEdits: accepts acceptIDs, rejects the rest. */
    apply: (messageId: string, acceptIDs: string[]) => Promise<ChatFile>
    /** Reloads when it changes (a turn finished). */
    reloadKey: unknown
    onerror: (message: string) => void
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
      onerror(t('uncli.review_load_failed', { detail: errorDetail(e) }))
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
      if (failed) onerror(t('uncli.review_some_failed', { count: failed }))
      open = false
    } catch (e) {
      onerror(t('uncli.review_apply_failed', { detail: errorDetail(e) }))
    } finally {
      busy = false
      await reload()
    }
  }
</script>

{#if count > 0}
  <div class="review">
    <button type="button" class="review-toggle" aria-expanded={open} onclick={() => (open = !open)}>
      <ListChecks size={14} />
      {t('uncli.review_waiting', { count })}
    </button>
    {#if open}
      <ul class="review-list">
        {#each groups as g (g.messageId)}
          {#each g.edits as e (e.id)}
            <li>
              <label>
                <input type="checkbox" bind:checked={checked[e.id]} disabled={busy} />
                <span>{e.label}</span>
              </label>
            </li>
          {/each}
        {/each}
      </ul>
      <div class="review-actions">
        <button type="button" class="action" onclick={() => resolve(false)} disabled={busy}>{t('uncli.review_reject_all')}</button>
        <button type="button" class="action primary" onclick={() => resolve(true)} disabled={busy || !groups.some(g => g.edits.some(e => checked[e.id]))}>
          {t('uncli.review_apply')}
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .review {
    margin: 0.5rem 0.85rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--warn-border);
    border-radius: 8px;
    background: var(--bg-elev-1);
    font-size: 0.85rem;
    color: var(--text);
  }
  .review-toggle {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    width: 100%;
    min-height: 36px;
    padding: 0;
    border: none;
    background: none;
    color: var(--warn-border);
    font: inherit;
    font-weight: 600;
    text-align: left;
    cursor: pointer;
  }
  .review-list {
    list-style: none;
    margin: 0.4rem 0 0;
    padding: 0;
    max-height: 40vh;
    overflow-y: auto;
  }
  .review-list label {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.35rem 0;
  }
  .review-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }
  .action {
    min-height: 36px;
    padding: 0.4rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
  }
  .action.primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--on-accent);
  }
  .action:disabled {
    opacity: 0.5;
  }
</style>
