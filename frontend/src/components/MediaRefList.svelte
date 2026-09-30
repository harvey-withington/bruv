<script lang="ts">
  // A slide media field's literal value, edited as a list: one row per
  // gallery URL / attachment ref, removed per item. Image fields hold many
  // (newline-joined — see lib/slides/mediaValue.ts), video fields one. The
  // old single-line input merged gallery URLs (inputs strip line breaks)
  // and the attachment chip's ✕ wiped every image.
  import { t } from '../lib/i18n.svelte'
  import { clickOutside } from '../lib/actions'
  import { pushKeyLayer } from '../lib/keyLayer'
  import { splitMediaValue, joinMediaValue, pastedMediaItems } from '../lib/slides/mediaValue'
  import { parseAttachmentRef } from '@shared/attachmentRefs'
  import { Paperclip, Plus, X } from 'lucide-svelte'

  type AttachOption = { ref: string; name: string; fromLinked: boolean }
  type Row = { id: string; text: string }

  let { value, multiple, attachmentOptions, refDisplayName, onChange }: {
    value: string
    /** Image fields take a gallery; video fields a single item. */
    multiple: boolean
    attachmentOptions: AttachOption[]
    refDisplayName: (ref: string) => string
    onChange: (value: string) => void
  } = $props()

  const newRow = (text = ''): Row => ({ id: `m-${crypto.randomUUID().slice(0, 8)}`, text })

  // Local rows (stable ids) so a row being typed in may be blank without
  // vanishing; the stored value is re-joined from them on every change.
  let rows = $state<Row[]>([])
  let lastEmitted: string | null = null
  let attachOpen = $state(false)

  // Re-seed only on an EXTERNAL value change (not our own echo).
  $effect(() => {
    if (value === lastEmitted) return
    const items = splitMediaValue(value)
    rows = items.length > 0 ? items.map((s) => newRow(s)) : [newRow()]
    lastEmitted = value
  })

  const canAdd = $derived(multiple || rows.every((r) => !r.text.trim()))

  function emit(): void {
    lastEmitted = joinMediaValue(rows.map((r) => r.text))
    onChange(lastEmitted)
  }

  function setRow(id: string, text: string): void {
    rows = rows.map((r) => (r.id === id ? { ...r, text } : r))
    emit()
  }

  function removeRow(id: string): void {
    const next = rows.filter((r) => r.id !== id)
    rows = next.length > 0 ? next : [newRow()]
    emit()
  }

  function addRow(): void {
    rows = [...rows, newRow()]
  }

  // Pasting several lines (a copied gallery) becomes one row per line
  // instead of being squashed into a single input.
  function handlePaste(e: ClipboardEvent, id: string): void {
    const items = pastedMediaItems(e.clipboardData?.getData('text') ?? '')
    if (items.length < 2) return
    e.preventDefault()
    const idx = rows.findIndex((r) => r.id === id)
    const kept = rows[idx]?.text.trim() ? [rows[idx]] : []
    const added = (multiple ? items : items.slice(0, 1)).map((s) => newRow(s))
    rows = [...rows.slice(0, idx), ...kept, ...added, ...rows.slice(idx + 1)]
    emit()
  }

  function pick(ref: string): void {
    attachOpen = false
    if (!multiple) {
      rows = [newRow(ref)]
    } else {
      const filled = rows.filter((r) => r.text.trim())
      if (filled.some((r) => r.text.trim() === ref)) return
      rows = [...filled, newRow(ref)]
    }
    emit()
  }

  // The attachment dropdown owns Escape while open (key-layer stack, above
  // the slide editor's layer — UI-CONVENTIONS §8.1).
  $effect(() => {
    if (!attachOpen) return
    const layer = pushKeyLayer({ onEscape: () => { attachOpen = false } })
    return () => layer.remove()
  })
</script>

<div class="media-list" use:clickOutside={{ onOutsideClick: () => { attachOpen = false } }}>
  {#each rows as row (row.id)}
    {#if parseAttachmentRef(row.text.trim())}
      <div class="attach-chip">
        <Paperclip size={12} />
        <span class="attach-name">{refDisplayName(row.text.trim())}</span>
        <button class="chip-x" type="button" onclick={() => removeRow(row.id)} title={t('common.remove')} aria-label={t('common.remove')}><X size={12} /></button>
      </div>
    {:else}
      <div class="media-row">
        <input
          class="field-input mono"
          value={row.text}
          oninput={(e) => setRow(row.id, e.currentTarget.value)}
          onpaste={(e) => handlePaste(e, row.id)}
          placeholder={t('slide.media_placeholder')}
        />
        {#if rows.length > 1 || row.text}
          <button class="chip-x" type="button" onclick={() => removeRow(row.id)} title={t('common.remove')} aria-label={t('common.remove')}><X size={12} /></button>
        {/if}
      </div>
    {/if}
  {/each}

  <div class="media-actions">
    {#if multiple && canAdd}
      <button class="attach-toggle" type="button" onclick={addRow}>
        <Plus size={11} /> {t('slide.media_add_url')}
      </button>
    {/if}
    {#if attachmentOptions.length > 0}
      <button class="attach-toggle" type="button" onclick={() => (attachOpen = !attachOpen)}>
        <Paperclip size={11} /> {t('slide.pick_attachment')}
      </button>
    {:else}
      <span class="field-hint">{t('slide.media_hint')}</span>
    {/if}
  </div>
  {#if attachOpen}
    <div class="block-picker">
      {#each attachmentOptions as opt (opt.ref)}
        <button type="button" onclick={() => pick(opt.ref)}>
          {opt.name}{opt.fromLinked ? ` ${t('slide.from_linked_card')}` : ''}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .media-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .media-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .media-actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .field-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-primary);
    font-size: 12px;
    width: 100%;
    box-sizing: border-box;
    font-family: ui-monospace, Consolas, monospace;
  }
  .field-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .field-hint {
    font-size: 10px;
    color: var(--text-muted);
    font-style: italic;
  }
  .attach-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }
  .attach-name {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .attach-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 11px;
    cursor: pointer;
    padding: 2px 0;
  }
  .attach-toggle:hover,
  .attach-toggle:focus-visible {
    color: var(--text-primary);
  }
  .chip-x {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    padding: 0;
  }
  .chip-x:hover,
  .chip-x:focus-visible {
    color: var(--danger);
  }
  .block-picker {
    display: flex;
    flex-direction: column;
    gap: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 4px;
    background: var(--bg-elevated);
  }
  .block-picker button {
    text-align: left;
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 12px;
    padding: 4px 6px;
    border-radius: 4px;
    cursor: pointer;
  }
  .block-picker button:hover,
  .block-picker button:focus-visible {
    background: var(--bg-hover);
  }
</style>
