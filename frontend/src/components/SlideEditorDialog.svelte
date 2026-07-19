<script lang="ts">
  import type { Slide, SlideFieldDef, SlideFieldType, Card, SearchResult, Block } from '@shared/types'
  import { untrack } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { X, Link2 } from 'lucide-svelte'
  import { portal, focusTrap } from '../lib/actions'
  import { GetCard, SearchCards } from '@shared/api'
  import { showToast } from '../lib/toast.svelte'
  import { SLIDE_CONTENT_TYPES, resolveContentType } from '@shared/slideContentTypes'
  import { templatesForContentType, resolveSlideTemplate } from '@shared/slideTemplates'
  import { isBlockCompatible, resolveBlockValueForField } from '@shared/slideBindings'
  import SlideRenderer from '../lib/slides/SlideRenderer.svelte'

  let {
    slide,
    cardId: _cardId,
    onSave,
    onClose,
  }: {
    slide: Slide
    cardId: string
    onSave: (updated: Slide) => void
    onClose: () => void
  } = $props()

  const initial = untrack(() => $state.snapshot(slide)) as Slide
  let contentTypeId = $state(initial.contentTypeId || 'title')
  let templateId = $state<string | undefined>(initial.templateId)
  let values = $state<Record<string, string>>({ ...(initial.values ?? {}) })
  let bindings = $state<Record<string, string>>({ ...(initial.bindings ?? {}) })
  let linkedCardId = $state<string | undefined>(initial.cardId)
  let durationInput = $state<number>(initial.durationSec ?? 0)
  let notes = $state<string>(initial.notes ?? '')

  let linkedCard = $state<Card | null>(null)
  let cardQuery = $state('')
  let cardResults = $state<SearchResult[]>([])
  let openPickerField = $state<string | null>(null)

  const contentType = $derived(resolveContentType(contentTypeId))
  const fields = $derived<SlideFieldDef[]>(contentType?.fields ?? [])
  const templates = $derived(templatesForContentType(contentTypeId))
  const activeTemplateId = $derived(resolveSlideTemplate(templateId, contentTypeId).id)

  // Fetch the linked card's blocks whenever the link changes (stale-guarded).
  let loadSeq = 0
  $effect(() => {
    const id = linkedCardId
    if (!id) {
      linkedCard = null
      return
    }
    const seq = ++loadSeq
    GetCard(id)
      .then((c) => {
        if (seq === loadSeq) linkedCard = c
      })
      .catch(() => {
        if (seq === loadSeq) {
          linkedCard = null
          showToast(t('slide.link_load_failed'), 'error')
        }
      })
  })

  let searchSeq = 0
  async function onCardSearch(): Promise<void> {
    const query = cardQuery.trim()
    if (query.length < 2) {
      cardResults = []
      return
    }
    const seq = ++searchSeq
    try {
      const res = await SearchCards(query, 8)
      if (seq === searchSeq) cardResults = res
    } catch {
      if (seq === searchSeq) cardResults = []
    }
  }
  function linkCard(r: SearchResult): void {
    linkedCardId = r.CardID
    cardQuery = ''
    cardResults = []
  }
  function unlinkCard(): void {
    linkedCardId = undefined
    linkedCard = null
    bindings = {}
  }

  function compatibleBlocks(fieldType: SlideFieldType): Block[] {
    if (!linkedCard) return []
    return linkedCard.blocks.filter((b) => isBlockCompatible(b.type, fieldType))
  }
  function bindField(key: string, blockId: string): void {
    bindings = { ...bindings, [key]: blockId }
    openPickerField = null
  }
  function unbindField(key: string): void {
    const next = { ...bindings }
    delete next[key]
    bindings = next
  }
  function boundBlockLabel(key: string): string {
    const b = linkedCard?.blocks.find((x) => x.id === bindings[key])
    return b ? b.label || b.type : bindings[key]
  }
  function resolveField(_s: Slide, fieldKey: string): string | undefined {
    const id = bindings[fieldKey]
    if (!id || !linkedCard) return undefined
    const block = linkedCard.blocks.find((b) => b.id === id)
    if (!block) return undefined
    const ft = fields.find((f) => f.key === fieldKey)?.type ?? 'text'
    return resolveBlockValueForField(block, ft)
  }

  function setContentType(id: string): void {
    contentTypeId = id
    if (templateId && !templatesForContentType(id).some((tpl) => tpl.id === templateId)) {
      templateId = undefined
    }
  }

  const previewSlide = $derived<Slide>({
    id: initial.id,
    contentTypeId,
    templateId,
    cardId: linkedCardId,
    values,
    bindings,
    durationSec: durationInput > 0 ? durationInput : undefined,
  })

  function save(): void {
    const cleanValues: Record<string, string> = {}
    const cleanBindings: Record<string, string> = {}
    for (const f of fields) {
      const v = (values[f.key] ?? '').trim()
      if (v) cleanValues[f.key] = v
      if (bindings[f.key]) cleanBindings[f.key] = bindings[f.key]
    }
    const d: Slide = { id: initial.id, contentTypeId, values: cleanValues }
    if (templateId) d.templateId = templateId
    if (linkedCardId) d.cardId = linkedCardId
    if (Object.keys(cleanBindings).length) d.bindings = cleanBindings
    if (durationInput > 0) d.durationSec = durationInput
    if (notes.trim()) d.notes = notes.trim()
    if (initial.thumbnail) d.thumbnail = initial.thumbnail
    onSave(d)
  }

  function handleKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-modal="true" tabindex="-1" aria-label={t('slide.editor_title')} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('slide.editor_title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <div class="editor-grid">
        <div class="form">
          <div class="field">
            <span class="field-label">{t('slide.content_type')}</span>
            <div class="seg-picker">
              {#each SLIDE_CONTENT_TYPES as ct (ct.id)}
                <button class="seg-btn" class:active={contentTypeId === ct.id} type="button" onclick={() => setContentType(ct.id)}>{ct.name}</button>
              {/each}
            </div>
          </div>

          <div class="field">
            <span class="field-label">{t('slide.linked_card')}</span>
            {#if linkedCardId}
              <div class="linked-chip">
                <span class="linked-name">{linkedCard?.title || linkedCardId}</span>
                <button class="chip-x" type="button" onclick={unlinkCard} title={t('slide.unlink_card')} aria-label={t('slide.unlink_card')}><X size={12} /></button>
              </div>
              <span class="field-hint">{t('slide.linked_card_hint')}</span>
            {:else}
              <input class="field-input" bind:value={cardQuery} oninput={onCardSearch} placeholder={t('slide.link_card_search')} />
              {#if cardResults.length > 0}
                <ul class="card-results">
                  {#each cardResults as r (r.CardID)}
                    <li><button type="button" onclick={() => linkCard(r)}>{r.Title || t('card.untitled')}</button></li>
                  {/each}
                </ul>
              {/if}
            {/if}
          </div>

          {#each fields as field (field.key)}
            <div class="field">
              <div class="field-head">
                <span class="field-label">{field.label}</span>
                {#if linkedCardId}
                  {#if bindings[field.key]}
                    <button class="link-btn active" type="button" onclick={() => unbindField(field.key)} title={t('slide.unbind')}>
                      <Link2 size={11} /> {boundBlockLabel(field.key)} <X size={10} />
                    </button>
                  {:else}
                    <button class="link-btn" type="button" onclick={() => (openPickerField = openPickerField === field.key ? null : field.key)} title={t('slide.bind')} aria-label={t('slide.bind')}>
                      <Link2 size={11} />
                    </button>
                  {/if}
                {/if}
              </div>

              {#if bindings[field.key]}
                <div class="bound-value">{resolveField(previewSlide, field.key) || '—'}</div>
              {:else}
                {#if openPickerField === field.key}
                  <div class="block-picker">
                    {#each compatibleBlocks(field.type) as b (b.id)}
                      <button type="button" onclick={() => bindField(field.key, b.id)}>{b.label || b.type}</button>
                    {/each}
                    {#if compatibleBlocks(field.type).length === 0}
                      <span class="picker-empty">{t('slide.no_compatible_blocks')}</span>
                    {/if}
                  </div>
                {/if}
                {#if field.type === 'longtext'}
                  <textarea class="field-input" rows="3" value={values[field.key] ?? ''} oninput={(e) => (values[field.key] = e.currentTarget.value)} placeholder={field.label}></textarea>
                {:else}
                  <input
                    class="field-input"
                    class:mono={field.type === 'image' || field.type === 'video'}
                    value={values[field.key] ?? ''}
                    oninput={(e) => (values[field.key] = e.currentTarget.value)}
                    placeholder={field.type === 'image' || field.type === 'video' ? t('slide.media_placeholder') : field.label}
                  />
                {/if}
                {#if field.type === 'image' || field.type === 'video'}
                  <span class="field-hint">{t('slide.media_hint')}</span>
                {/if}
              {/if}
            </div>
          {/each}

          {#if templates.length > 1}
            <div class="field">
              <span class="field-label">{t('slide.template')}</span>
              <div class="seg-picker">
                {#each templates as tpl (tpl.id)}
                  <button class="seg-btn" class:active={activeTemplateId === tpl.id} type="button" onclick={() => (templateId = tpl.id)}>{tpl.name}</button>
                {/each}
              </div>
            </div>
          {/if}

          <label class="field">
            <span class="field-label">{t('slide.duration')}</span>
            <input class="field-input" type="number" min="0" bind:value={durationInput} placeholder="0" />
            <span class="field-hint">{t('slide.duration_hint')}</span>
          </label>

          <label class="field">
            <span class="field-label">{t('slide.notes')}</span>
            <textarea class="field-input" rows="2" bind:value={notes} placeholder={t('slide.notes_placeholder')}></textarea>
          </label>
        </div>

        <div class="preview-col">
          <span class="field-label">{t('slide.preview')}</span>
          <div class="preview-frame">
            <SlideRenderer slide={previewSlide} {resolveField} />
          </div>
          <p class="field-hint">{t('slide.preview_hint')}</p>
        </div>
      </div>
    </div>

    <div class="dialog-footer">
      <button class="btn btn-ghost" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn btn-primary" onclick={save}>{t('common.save')}</button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 220;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: min(1000px, 94vw);
    height: min(85vh, 720px);
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
  }
  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 2px;
    line-height: 1;
  }
  .close-btn:hover {
    color: var(--text-primary);
  }
  .dialog-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem 1.1rem;
    min-height: 0;
  }
  .editor-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
    gap: 1.5rem;
    height: 100%;
  }
  @media (max-width: 780px) {
    .editor-grid {
      grid-template-columns: 1fr;
    }
  }
  .form {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .field-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }
  .field-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    box-sizing: border-box;
    font-family: inherit;
  }
  .field-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .field-input.mono {
    font-family: ui-monospace, Consolas, monospace;
    font-size: 12px;
  }
  .field-hint {
    font-size: 10px;
    color: var(--text-muted);
    font-style: italic;
  }
  .seg-picker {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .seg-btn {
    padding: 5px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-muted);
    font-size: 12px;
    cursor: pointer;
  }
  .seg-btn:hover {
    color: var(--text-primary);
    background: var(--bg-hover);
  }
  .seg-btn.active {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }
  .link-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text-muted);
    font-size: 10px;
    cursor: pointer;
    max-width: 60%;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .link-btn:hover {
    color: var(--text-primary);
    border-color: var(--border-muted);
  }
  .link-btn.active {
    color: var(--accent);
    border-color: var(--accent);
  }
  .bound-value {
    padding: 6px 10px;
    border: 1px dashed var(--accent);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--accent) 8%, transparent);
    color: var(--text-primary);
    font-size: 13px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 5rem;
    overflow-y: auto;
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
  .block-picker button:hover {
    background: var(--bg-hover);
  }
  .picker-empty {
    font-size: 11px;
    color: var(--text-muted);
    padding: 4px 6px;
  }
  .linked-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }
  .linked-name {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chip-x {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    padding: 0;
  }
  .chip-x:hover {
    color: var(--danger);
  }
  .card-results {
    list-style: none;
    margin: 2px 0 0;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    max-height: 10rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .card-results button {
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 12px;
    padding: 5px 6px;
    border-radius: 4px;
    cursor: pointer;
  }
  .card-results button:hover {
    background: var(--bg-hover);
  }
  .preview-col {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-height: 0;
  }
  .preview-frame {
    aspect-ratio: 16 / 9;
    width: 100%;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    background: #0b0b12;
  }
  .dialog-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 0.75rem 1.1rem;
    border-top: 1px solid var(--border-muted);
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 14px;
    font-size: 12px;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn:hover {
    background: var(--bg-hover);
  }
  .btn-primary {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }
  .btn-primary:hover {
    filter: brightness(1.1);
  }
  .btn-ghost {
    background: transparent;
  }
</style>
