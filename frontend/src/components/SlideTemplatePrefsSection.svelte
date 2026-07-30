<script lang="ts">
  // Settings → Slide Templates: vault-level Auto-matching preferences.
  // Rows are the templates that participate in Auto (those with a urlHint),
  // drag-reorderable to set multi-match priority (row order IS the priority,
  // BRUV-style: no grip icons — the row itself drags). Each row's URL
  // pattern is editable; an override that doesn't compile shows an inline
  // error and the built-in hint keeps matching (rendering can never break).
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { SLIDE_TEMPLATES } from '@shared/slideTemplates'
  import type { SlideTemplate } from '@shared/types'
  import { templatePrefs, loadTemplatePrefs, saveTemplatePrefs } from '../lib/templatePrefs.svelte'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'

  const AUTO_TEMPLATES: SlideTemplate[] = SLIDE_TEMPLATES.filter((tpl) => tpl.urlHint)

  // Working copy — synced from the store on mount, persisted on every
  // committed change (reorder / pattern blur). Keyed by template id.
  let order = $state<string[]>([])
  let overrides = $state<Record<string, string>>({})
  let invalidIds = $state<Record<string, boolean>>({})

  loadTemplatePrefs()
    .then(() => {
      const p = templatePrefs()
      overrides = { ...(p.urlOverrides ?? {}) }
      order = orderedIds(p.order ?? [])
      for (const id of Object.keys(overrides)) validate(id, overrides[id])
    })
    .catch(() => showToast(t('templates.prefs_load_failed'), 'error'))

  // Prefs order first (ignoring unknown ids), then the rest in registration
  // order — mirrors shared/slideTemplates.ts byPrefOrder.
  function orderedIds(prefOrder: string[]): string[] {
    const known = AUTO_TEMPLATES.map((tpl) => tpl.id)
    const listed = prefOrder.filter((id) => known.includes(id))
    return [...listed, ...known.filter((id) => !listed.includes(id))]
  }

  const rows = $derived(order.map((id) => AUTO_TEMPLATES.find((tpl) => tpl.id === id)).filter((tpl): tpl is SlideTemplate => !!tpl))

  function validate(id: string, source: string): void {
    if (!source.trim()) {
      invalidIds = { ...invalidIds, [id]: false }
      return
    }
    try {
      new RegExp(source, 'i')
      invalidIds = { ...invalidIds, [id]: false }
    } catch {
      invalidIds = { ...invalidIds, [id]: true }
    }
  }

  async function persist(): Promise<void> {
    const urlOverrides: Record<string, string> = {}
    for (const [id, src] of Object.entries(overrides)) {
      if (src.trim()) urlOverrides[id] = src.trim()
    }
    try {
      await saveTemplatePrefs({
        order: [...order],
        ...(Object.keys(urlOverrides).length ? { urlOverrides } : {}),
      })
    } catch {
      showToast(t('templates.prefs_save_failed'), 'error')
    }
  }

  function setOverride(id: string, source: string): void {
    overrides = { ...overrides, [id]: source }
    validate(id, source)
  }

  function resetOverride(id: string): void {
    const next = { ...overrides }
    delete next[id]
    overrides = next
    invalidIds = { ...invalidIds, [id]: false }
    void persist()
  }

  // Drag state — id-keyed (never index-keyed), same reference DnD shape as
  // OptionsEditorDialog / EditableChecklist.
  let draggingId = $state<string | null>(null)
  let dropBeforeId = $state<string | typeof DROP_END | null>(null)

  type OrderRow = { id: string }
  const orderRows = $derived<OrderRow[]>(order.map((id) => ({ id })))

  function handleDragOver(e: DragEvent, overId: string, idx: number): void {
    if (draggingId === null) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const midY = rect.top + rect.height / 2
    let candidate: string | typeof DROP_END
    if (e.clientY < midY) {
      candidate = overId
    } else {
      const next = orderRows[idx + 1]
      candidate = next ? next.id : DROP_END
    }
    dropBeforeId = wouldReorder(orderRows, draggingId, candidate, 'move') ? candidate : null
  }

  function handleDragEnd(): void {
    draggingId = null
    dropBeforeId = null
  }

  function handleDrop(e: DragEvent): void {
    e.preventDefault()
    if (draggingId === null || dropBeforeId === null) {
      handleDragEnd()
      return
    }
    const reordered = computeReorder(orderRows, draggingId, dropBeforeId, { mode: 'move' })
    handleDragEnd()
    if (reordered !== orderRows) {
      order = reordered.map((r) => r.id)
      void persist()
    }
  }
</script>

<p class="intro">{t('templates.prefs_intro')}</p>

<div class="tpl-list" role="list" ondrop={handleDrop} ondragover={(e) => e.preventDefault()}>
  {#each rows as tpl, idx (tpl.id)}
    <div
      class="tpl-row"
      class:dragging={draggingId === tpl.id}
      class:drop-before={dropBeforeId === tpl.id}
      role="listitem"
      draggable="true"
      ondragstart={() => (draggingId = tpl.id)}
      ondragover={(e) => handleDragOver(e, tpl.id, idx)}
      ondragend={handleDragEnd}
    >
      <div class="tpl-head">
        <span class="tpl-rank">{idx + 1}</span>
        <span class="tpl-name">{tpl.name}</span>
        {#if overrides[tpl.id]?.trim()}
          <button class="tpl-reset" type="button" onclick={() => resetOverride(tpl.id)}>{t('templates.pattern_reset')}</button>
        {/if}
      </div>
      <label class="tpl-pattern">
        <span class="pattern-label">{t('templates.pattern_label')}</span>
        <input
          type="text"
          spellcheck="false"
          value={overrides[tpl.id] ?? ''}
          placeholder={tpl.urlHint}
          oninput={(e) => setOverride(tpl.id, e.currentTarget.value)}
          onblur={() => void persist()}
        />
      </label>
      {#if invalidIds[tpl.id]}
        <p class="pattern-error">{t('templates.pattern_invalid')}</p>
      {/if}
    </div>
  {/each}
  {#if draggingId !== null && dropBeforeId === DROP_END}
    <div class="drop-end-line"></div>
  {/if}
</div>

<style>
  .intro {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin: 0 0 0.75rem;
    line-height: 1.45;
  }
  .tpl-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .tpl-row {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    background: var(--bg-elevated);
    cursor: grab;
  }
  .tpl-row.dragging {
    opacity: 0.5;
  }
  .tpl-row.drop-before {
    box-shadow: 0 -2px 0 0 var(--accent);
  }
  .drop-end-line {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
  }
  .tpl-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .tpl-rank {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--text-muted);
    min-width: 1.1rem;
  }
  .tpl-name {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-primary);
  }
  .tpl-reset {
    margin-left: auto;
    background: none;
    border: none;
    color: var(--accent);
    font-size: 0.75rem;
    cursor: pointer;
    padding: 0.15rem 0.3rem;
  }
  .tpl-pattern {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.45rem;
  }
  .pattern-label {
    font-size: 0.75rem;
    color: var(--text-secondary);
    white-space: nowrap;
  }
  .tpl-pattern input {
    flex: 1;
    font-family: var(--font-mono, monospace);
    font-size: 0.78rem;
    padding: 0.4rem 0.55rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-primary);
    color: var(--text-primary);
    outline: none;
    box-sizing: border-box;
    min-width: 0;
  }
  .tpl-pattern input:focus {
    border-color: var(--accent);
  }
  .pattern-error {
    margin: 0.35rem 0 0;
    font-size: 0.75rem;
    color: var(--danger, #e5484d);
  }
</style>
