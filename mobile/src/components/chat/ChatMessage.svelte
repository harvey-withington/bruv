<script lang="ts">
  import { Wrench, Check, X, MapPin, ListChecks } from 'lucide-svelte'
  import { renderMarkdown } from '@shared/markdown'
  import { t } from '../../lib/i18n.svelte'
  import type { ChatMessage, ToolAction, PendingEdit } from './types'

  let {
    msg,
    projectMode = false,
    onAcceptPin,
    onRejectPin,
    onApplyEdits,
    onOpenCard,
  }: {
    msg: ChatMessage
    projectMode?: boolean
    onAcceptPin?: (msgID: string) => void
    onRejectPin?: (msgID: string) => void
    /** Called with the subset of pending edits the user ticked. */
    onApplyEdits?: (msgID: string, acceptIDs: string[]) => void
    /** Called when the user taps "Open" on a created-card chip. */
    onOpenCard?: (cardID: string) => void
  } = $props()

  // Per-message checkbox state for the pending-edits review block.
  // Initialised to all-pending-checked when the message first renders.
  let checked = $state<Record<string, boolean>>({})

  $effect(() => {
    if (!msg.pending_edits?.length) return
    const init: Record<string, boolean> = {}
    for (const e of msg.pending_edits) {
      if (e.status === 'pending') init[e.id] = true
    }
    // Bail out when there's nothing to seed. Without this guard, a
    // message that has pending_edits but no entries in 'pending' status
    // (all already accepted/rejected) drives an infinite update loop:
    // init is `{}`, `checked` stays `{}` (Object.keys length 0), so the
    // gate below keeps writing `{}` to `checked` — and Svelte sees each
    // assignment as a new reference, re-fires the effect, and tips into
    // effect_update_depth_exceeded. That error then breaks event wiring
    // for the entire chat sheet, which is why the parent's clicks and
    // bind:value silently stop working.
    if (Object.keys(init).length === 0) return
    // Only seed once per message ID lifetime — preserve user choices
    // across re-renders triggered by streaming or new messages.
    if (Object.keys(checked).length === 0) {
      checked = init
    }
  })

  function pendingEdits(): PendingEdit[] {
    return msg.pending_edits?.filter((e) => e.status === 'pending') ?? []
  }

  function someChecked(): boolean {
    return pendingEdits().some((e) => checked[e.id])
  }

  function allChecked(): boolean {
    const list = pendingEdits()
    return list.length > 0 && list.every((e) => checked[e.id])
  }

  function toggleAll() {
    const value = !allChecked()
    const next = { ...checked }
    for (const e of pendingEdits()) next[e.id] = value
    checked = next
  }

  function previewEditValue(detail: string | undefined): string {
    if (!detail) return ''
    const oneLine = detail.replace(/\s+/g, ' ').trim()
    return oneLine.length <= 60 ? oneLine : `${oneLine.slice(0, 60)}…`
  }

  function distinctCardCount(): number {
    const ids = new Set<string>()
    for (const e of msg.pending_edits ?? []) {
      const cid = (e.input as Record<string, unknown> | undefined)?.card_id
      if (typeof cid === 'string' && cid !== '') ids.add(cid)
    }
    return ids.size
  }

  function toolLabel(action: ToolAction): string {
    const inp = (action.input ?? {}) as Record<string, unknown>
    switch (action.tool) {
      case 'set_title': return `Title: ${inp.title || '?'}`
      case 'set_due_date': return inp.due_date ? `Due: ${inp.due_date}` : 'Cleared due date'
      case 'set_card_type': return `Set type: ${inp.card_type || '?'}`
      case 'set_fields':
      case 'update_blocks': {
        const fields = (inp.fields || inp.blocks) as Record<string, unknown> | undefined
        const keys = fields ? Object.keys(fields) : []
        return `Updated: ${keys.join(', ') || '?'}`
      }
      case 'add_tags': return `Added tags: ${(inp.tags as string[] || []).join(', ')}`
      case 'add_field': return `Added field: ${inp.label || inp.key || '?'}`
      case 'suggest_pin': return `Suggested pin: ${action.result || '?'}`
      case 'create_card': return `Created card: ${inp.title || '?'}`
      case 'add_tags_to_cards':
        return `Tagged ${(inp.card_ids as string[] || []).length} cards: ${(inp.tags as string[] || []).join(', ')}`
      case 'move_card': return 'Moved card'
      case 'update_card': return action.result || 'Updated card'
      default: return action.tool
    }
  }

  function extractCardId(result: string): string | null {
    const match = result.match(/\(ID: ([^)]+)\)/)
    return match ? match[1] : null
  }

  function applyChecked() {
    const accepts = pendingEdits().filter((e) => checked[e.id]).map((e) => e.id)
    onApplyEdits?.(msg.id, accepts)
  }

  function formatTime(ts: string): string {
    try {
      const d = new Date(ts)
      return d.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return ''
    }
  }

  const hasPending = $derived(pendingEdits().length > 0)
</script>

<article class="msg msg-{msg.role}">
  <div class="content">{@html renderMarkdown(msg.content)}</div>

  {#if msg.tool_actions?.length}
    <div class="tools">
      {#each msg.tool_actions as a}
        <div class="tool">
          <Wrench size={11} />
          <span class="tool-label">{toolLabel(a)}</span>
          {#if a.tool === 'create_card' && extractCardId(a.result)}
            <button type="button" class="open-card" onclick={() => onOpenCard?.(extractCardId(a.result)!)}>
              {t('chat.open_card')}
            </button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if msg.pending_edits?.length}
    <div class="pending">
      <header class="pending-header">
        {#if hasPending}
          <input
            type="checkbox"
            class="cb"
            checked={allChecked()}
            indeterminate={someChecked() && !allChecked()}
            onchange={toggleAll}
            aria-label="Select all"
          />
        {:else}
          <ListChecks size={12} />
        {/if}
        <span class="pending-title">{t('chat.suggest_edits_review')}</span>
        {#if projectMode && hasPending}
          {@const n = distinctCardCount()}
          {#if n > 0}
            <span class="card-count">
              {n === 1 ? t('chat.suggest_one_card') : t('chat.suggest_n_cards', { count: n })}
            </span>
          {/if}
        {/if}
        {#if hasPending}
          <button type="button" class="apply" disabled={!someChecked()} onclick={applyChecked}>
            {t('chat.apply')}
          </button>
        {/if}
      </header>
      {#each msg.pending_edits as e (e.id)}
        <div
          class="edit-row"
          class:accepted={e.status === 'accepted'}
          class:rejected={e.status === 'rejected'}
          class:errored={e.status === 'pending' && e.detail?.startsWith('error:')}
        >
          {#if e.status === 'pending'}
            <input
              type="checkbox"
              class="cb"
              checked={checked[e.id] ?? false}
              onchange={() => (checked = { ...checked, [e.id]: !checked[e.id] })}
            />
          {:else if e.status === 'accepted'}
            <Check size={11} class="edit-resolved-ok" />
          {:else}
            <X size={11} class="edit-resolved-bad" />
          {/if}
          <span class="edit-label">{e.label}</span>
          {#if e.detail}
            <span class="edit-preview" title={e.detail}>{previewEditValue(e.detail)}</span>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if !projectMode && msg.pin_suggestion}
    <div
      class="pin"
      class:accepted={msg.pin_suggestion.status === 'accepted'}
      class:rejected={msg.pin_suggestion.status === 'rejected'}
    >
      <header class="pin-header">
        <MapPin size={12} />
        <span>{t('chat.pin_suggestion')}</span>
      </header>
      <p class="pin-breadcrumb">{msg.pin_suggestion.breadcrumb}</p>
      {#if msg.pin_suggestion.reason}
        <p class="pin-reason">{msg.pin_suggestion.reason}</p>
      {/if}
      {#if msg.pin_suggestion.status === 'pending'}
        <div class="pin-actions">
          <button type="button" class="pin-btn pin-accept" onclick={() => onAcceptPin?.(msg.id)}>
            <Check size={12} /> {t('chat.pin_accept')}
          </button>
          <button type="button" class="pin-btn pin-reject" onclick={() => onRejectPin?.(msg.id)}>
            <X size={12} /> {t('chat.pin_dismiss')}
          </button>
        </div>
      {:else if msg.pin_suggestion.status === 'accepted'}
        <p class="pin-resolved">{t('chat.pin_accepted')}</p>
      {:else}
        <p class="pin-resolved">{t('chat.pin_dismissed')}</p>
      {/if}
    </div>
  {/if}

  <span class="time">{formatTime(msg.timestamp)}</span>
</article>

<style>
  .msg {
    max-width: 92%;
    padding: 8px 12px;
    border-radius: 12px;
    font-size: 0.9rem;
    line-height: 1.45;
    word-wrap: break-word;
  }
  .msg-user {
    align-self: flex-end;
    background: var(--accent);
    color: #fff;
    border-bottom-right-radius: 4px;
  }
  .msg-assistant {
    align-self: flex-start;
    background: var(--bg-elev-1);
    color: var(--text);
    border: 1px solid var(--border);
    border-bottom-left-radius: 4px;
  }
  .msg-system {
    align-self: center;
    background: transparent;
    color: var(--text-faint);
    font-size: 0.78rem;
    font-style: italic;
  }
  .content :global(p) { margin: 0; }
  .content :global(p + p) { margin-top: 0.5rem; }
  .content :global(a) { color: inherit; text-decoration: underline; }
  .content :global(code) {
    background: rgba(0,0,0,0.2);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .content :global(pre) {
    background: rgba(0,0,0,0.2);
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    overflow-x: auto;
  }
  .content :global(pre code) { background: transparent; padding: 0; }
  .msg-user .content :global(a) {
    color: #fff;
    border-bottom: 1px solid rgba(255,255,255,0.5);
    text-decoration: none;
  }

  .time {
    display: block;
    font-size: 0.65rem;
    color: var(--text-faint);
    margin-top: 4px;
  }
  .msg-user .time { color: rgba(255,255,255,0.7); text-align: right; }

  .tools {
    margin-top: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .tool {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.72rem;
    color: var(--text-muted);
  }
  .tool-label { flex: 1; min-width: 0; }
  .open-card {
    background: transparent;
    border: none;
    color: var(--accent);
    font: inherit;
    font-size: 0.72rem;
    cursor: pointer;
    padding: 0 4px;
    text-decoration: underline;
  }

  .pending {
    margin-top: 6px;
    padding: 6px 8px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .pending-header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 2px;
  }
  .pending-title {
    flex: 1;
    font-size: 0.72rem;
    font-weight: 600;
    color: var(--accent);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .card-count {
    font-size: 0.72rem;
    color: var(--text-muted);
  }
  .cb {
    width: 16px;
    height: 16px;
    accent-color: var(--accent);
    flex-shrink: 0;
  }
  .apply {
    background: color-mix(in srgb, var(--accent) 12%, var(--bg));
    border: 1px solid var(--accent);
    color: var(--accent);
    border-radius: 6px;
    padding: 4px 10px;
    font: inherit;
    font-size: 0.75rem;
    font-weight: 500;
    cursor: pointer;
  }
  .apply:disabled { opacity: 0.4; cursor: default; }
  .edit-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 0;
    min-width: 0;
    font-size: 0.78rem;
  }
  .edit-row.accepted .edit-label,
  .edit-row.accepted .edit-preview { opacity: 0.6; }
  .edit-row.rejected .edit-label,
  .edit-row.rejected .edit-preview {
    text-decoration: line-through;
    opacity: 0.45;
  }
  .edit-row.errored {
    background: rgba(239, 68, 68, 0.08);
    border-radius: 4px;
    padding: 2px 4px;
  }
  .edit-row.errored .edit-preview { color: #ef4444; }
  :global(.edit-resolved-ok) { color: #22c55e; flex-shrink: 0; }
  :global(.edit-resolved-bad) { color: var(--text-muted); flex-shrink: 0; }
  .edit-label {
    flex: 1 1 auto;
    min-width: 0;
    font-weight: 500;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .edit-preview {
    flex: 0 1 auto;
    max-width: 45%;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pin {
    margin-top: 6px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
  }
  .pin.accepted {
    border-color: #22c55e;
    background: color-mix(in srgb, #22c55e 8%, var(--bg));
  }
  .pin.rejected { opacity: 0.55; }
  .pin-header {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--accent);
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .pin-breadcrumb {
    margin: 4px 0 0;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text);
  }
  .pin-reason {
    margin: 2px 0 0;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .pin-actions {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }
  .pin-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    color: var(--text);
    border-radius: 6px;
    padding: 6px 10px;
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .pin-accept:hover {
    color: #22c55e;
    border-color: #22c55e;
  }
  .pin-reject:hover {
    color: #ef4444;
    border-color: #ef4444;
  }
  .pin-resolved {
    margin: 6px 0 0;
    font-size: 0.78rem;
    color: var(--text-muted);
    font-style: italic;
  }
</style>
