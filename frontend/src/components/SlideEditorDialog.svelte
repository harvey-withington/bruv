<script lang="ts">
  import type { Slide, SlideFieldDef, SlideFieldType, Card, SearchResult, Block, Attachment } from '@shared/types'
  import { untrack } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { X } from 'lucide-svelte'
  import { portal, focusTrap, clickOutside } from '../lib/actions'
  import { GetCard, SearchCards, RecentCards, SignAttachmentURL } from '@shared/api'
  import { showToast } from '../lib/toast.svelte'
  import { SLIDE_CONTENT_TYPES, resolveContentType } from '@shared/slideContentTypes'
  import { templatesForContentType, resolveSlideTemplate } from '@shared/slideTemplates'
  import { isBlockCompatible, resolveBlockValueForField } from '@shared/slideBindings'
  import SlideRenderer from '../lib/slides/SlideRenderer.svelte'
  import BoundField from './BoundField.svelte'

  let {
    slide,
    cardId,
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
  let hostCard = $state<Card | null>(null)
  let cardQuery = $state('')
  let cardResults = $state<SearchResult[]>([])

  // Signed preview URLs for "attachment:<cardID>/<attID>" media refs, keyed
  // by the ref string. Filled lazily by the effect below.
  let signedRefUrls = $state<Record<string, string>>({})

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

  // Load the host card once — its attachments feed the media-field picker.
  $effect(() => {
    let alive = true
    GetCard(cardId)
      .then((c) => {
        if (alive) hostCard = c
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  })

  // Media attachments available to pick: host card's first, then the linked
  // card's. The stored value is "attachment:<ownerCardID>/<attID>" so the
  // present resolver can sign it server-side.
  type AttachOption = { ref: string; name: string; fromLinked: boolean }
  function attachmentOptions(fieldType: SlideFieldType): AttachOption[] {
    const wantVideo = fieldType === 'video'
    const collect = (c: Card | null, fromLinked: boolean): AttachOption[] =>
      (c?.file_attachments ?? [])
        .filter((a: Attachment) => (wantVideo ? a.mime.startsWith('video/') : a.mime.startsWith('image/')))
        .map((a: Attachment) => ({ ref: `attachment:${c!.id}/${a.id}`, name: a.name, fromLinked }))
    return [...collect(hostCard, false), ...collect(linkedCard, true)]
  }
  function pickAttachment(fieldKey: string, ref: string): void {
    values[fieldKey] = ref
  }
  function refDisplayName(ref: string): string {
    const all = [...(hostCard?.file_attachments ?? []), ...(linkedCard?.file_attachments ?? [])]
    const attID = ref.split('/').pop() ?? ''
    return all.find((a) => a.id === attID)?.name ?? ref
  }

  // Sign preview URLs for any attachment refs in the current values.
  $effect(() => {
    const refs = Object.values(values).filter((v) => v.startsWith('attachment:') && !(v in signedRefUrls))
    for (const ref of refs) {
      const rest = ref.slice('attachment:'.length)
      const slash = rest.indexOf('/')
      if (slash <= 0) continue
      SignAttachmentURL(rest.slice(0, slash), rest.slice(slash + 1))
        .then((url) => {
          signedRefUrls = { ...signedRefUrls, [ref]: url }
        })
        .catch(() => {})
    }
  })
  function resolveMediaUrl(value: string): string | undefined {
    return value.startsWith('attachment:') ? signedRefUrls[value] : undefined
  }

  let searchSeq = 0
  async function onCardSearch(): Promise<void> {
    const query = cardQuery.trim()
    const seq = ++searchSeq
    // Empty/short query → recent cards, so the picker is never a blank box.
    // The card being edited is excluded — linking a slide to its own card
    // is a no-op the picker shouldn't offer.
    try {
      const res = query.length < 2 ? await RecentCards(8) : await SearchCards(query, 8)
      if (seq === searchSeq) cardResults = (res ?? []).filter((r) => r.CardID !== cardId)
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

  // Escape handling uses a CAPTURE-phase window listener (registered before
  // CardDetail's bubble-phase one fires) so Esc in this second-level dialog
  // never falls through and closes the card underneath — the exact data-loss
  // path Harvey hit. stopPropagation() shields the bubble phase; layering is
  // dropdown-first: an open dropdown consumes the Esc, the dialog only closes
  // on a "bare" one. (Same capture-shield pattern as mobile's ChatSheet; the
  // deferred overlay-stack refactor in TODO would replace all of these.)
  let dialogEl = $state<HTMLElement | null>(null)

  function handleCaptureKeydown(e: KeyboardEvent): void {
    if (e.key !== 'Escape') return
    e.preventDefault()
    e.stopPropagation()
    if (cardResults.length > 0) {
      cardResults = []
      return
    }
    // A BoundField dropdown is open — its own capture listener (registered
    // later, so it runs after this one) closes it. Just don't close the
    // dialog on this press.
    if (dialogEl?.querySelector('.block-picker')) return
    onClose()
  }

  $effect(() => {
    window.addEventListener('keydown', handleCaptureKeydown, true)
    return () => window.removeEventListener('keydown', handleCaptureKeydown, true)
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={(e) => { if (e.target === e.currentTarget) onClose() }}>
  <div class="dialog" role="dialog" aria-modal="true" tabindex="-1" aria-label={t('slide.editor_title')} use:focusTrap bind:this={dialogEl}>
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
                <button class="seg-btn" class:active={contentTypeId === ct.id} type="button" onclick={() => setContentType(ct.id)}>{t('slide.ct.' + ct.id)}</button>
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
              <div class="card-search" use:clickOutside={{ onOutsideClick: () => (cardResults = []) }}>
                <input class="field-input" bind:value={cardQuery} oninput={onCardSearch} onfocus={onCardSearch} placeholder={t('slide.link_card_search')} />
                {#if cardResults.length > 0}
                  <ul class="card-results">
                    {#each cardResults as r (r.CardID)}
                      <li><button type="button" onclick={() => linkCard(r)}>{r.Title || t('card.untitled')}</button></li>
                    {/each}
                  </ul>
                {/if}
              </div>
            {/if}
          </div>

          {#each fields as field (field.key)}
            <BoundField
              {field}
              value={values[field.key] ?? ''}
              isLinked={!!linkedCardId}
              binding={bindings[field.key]}
              boundLabel={boundBlockLabel(field.key)}
              boundPreview={resolveField(previewSlide, field.key) || '—'}
              compatibleBlocks={compatibleBlocks(field.type)}
              attachmentOptions={attachmentOptions(field.type)}
              {refDisplayName}
              onInput={(v) => (values[field.key] = v)}
              onBind={(blockId) => bindField(field.key, blockId)}
              onUnbind={() => unbindField(field.key)}
              onPickAttachment={(ref) => pickAttachment(field.key, ref)}
              onClearAttachment={() => (values[field.key] = '')}
            />
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
            <SlideRenderer slide={previewSlide} {resolveField} {resolveMediaUrl} />
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
  .card-search {
    display: flex;
    flex-direction: column;
    gap: 4px;
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
