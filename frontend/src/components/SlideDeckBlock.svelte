<script lang="ts">
  import type { SlideDeckValue, Slide, WailsWindow } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { Plus, GripVertical, Pencil, Copy, Trash2, Presentation, Clock, Play, ChevronLeft, ChevronRight } from 'lucide-svelte'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'
  import { resolveContentType, DEFAULT_CONTENT_TYPE_ID } from '@shared/slideContentTypes'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { SignPresentURL } from '@shared/api'
  import SlideEditorDialog from './SlideEditorDialog.svelte'

  let {
    value,
    cardId,
    onUpdate,
  }: {
    value: SlideDeckValue
    cardId: string
    onUpdate: (val: SlideDeckValue) => void
  } = $props()

  const slides = $derived<Slide[]>(value?.slides ?? [])

  let editingSlideId = $state<string | null>(null)
  const editingSlide = $derived<Slide | null>(
    editingSlideId ? slides.find((s) => s.id === editingSlideId) ?? null : null,
  )

  // A compact label for the row: the first non-empty field value of the
  // slide's content type, else the content-type name, else "Untitled".
  function slideLabel(slide: Slide): string {
    const ct = resolveContentType(slide.contentTypeId)
    if (ct) {
      for (const f of ct.fields) {
        const v = slide.values?.[f.key]
        if (v && v.trim()) return v.trim()
      }
      return t('slide.ct.' + slide.contentTypeId)
    }
    return t('slide.untitled')
  }

  function contentTypeName(slide: Slide): string {
    return resolveContentType(slide.contentTypeId) ? t('slide.ct.' + slide.contentTypeId) : slide.contentTypeId
  }

  function newSlideId(): string {
    return `sld-${crypto.randomUUID().slice(0, 8)}`
  }

  function commit(nextSlides: Slide[]): void {
    const idx = value?.currentIndex ?? 0
    const currentIndex = idx >= nextSlides.length ? 0 : Math.max(0, idx)
    onUpdate({ ...value, slides: nextSlides, currentIndex })
  }

  function addSlide(): void {
    const slide: Slide = { id: newSlideId(), contentTypeId: DEFAULT_CONTENT_TYPE_ID, values: {} }
    commit([...slides, slide])
    editingSlideId = slide.id
  }

  function duplicateSlide(s: Slide): void {
    const copy: Slide = { ...s, id: newSlideId() }
    const idx = slides.findIndex((x) => x.id === s.id)
    const next = [...slides]
    next.splice(idx + 1, 0, copy)
    commit(next)
  }

  async function deleteSlide(s: Slide): Promise<void> {
    const ok = await showConfirm(t('slide.delete_confirm', { title: slideLabel(s) }))
    if (!ok) return
    commit(slides.filter((x) => x.id !== s.id))
  }

  function saveSlide(updated: Slide): void {
    commit(slides.map((s) => (s.id === updated.id ? updated : s)))
    editingSlideId = null
  }

  // --- live position (the v1 presenter control surface is this block) ---
  // Bumping currentIndex rides the normal UpdateCardBlocks save path; the
  // /present output page polls the resolved card and follows within ~1.5s.
  const currentIndex = $derived(
    Math.min(Math.max(value?.currentIndex ?? 0, 0), Math.max(slides.length - 1, 0)),
  )
  function goTo(idx: number): void {
    if (idx < 0 || idx >= slides.length || idx === currentIndex) return
    onUpdate({ ...value, slides, currentIndex: idx })
  }

  // Present: mint the signed output-page URL, copy it (that's what gets
  // pasted into an OBS Browser Source), and open it for a quick look.
  let presenting = $state(false)
  async function present(): Promise<void> {
    if (presenting) return
    presenting = true
    try {
      const url = await SignPresentURL(cardId)
      try {
        await navigator.clipboard.writeText(url)
        showToast(t('slide.present_copied'), 'success')
      } catch {
        // Clipboard can be denied — opening still works.
        showToast(t('slide.present_opened'), 'success')
      }
      const w = window as WailsWindow
      if (w.runtime?.BrowserOpenURL) {
        w.runtime.BrowserOpenURL(url)
      } else {
        window.open(url, '_blank', 'noopener')
      }
    } catch {
      showToast(t('slide.present_failed'), 'error')
    } finally {
      presenting = false
    }
  }

  // --- reorder (grip DnD) ---
  let draggingId = $state<string | null>(null)
  let dropBeforeId = $state<string | typeof DROP_END | null>(null)

  function handleDragStart(e: DragEvent, id: string): void {
    draggingId = id
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', id)
    }
  }
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
      const next = slides[idx + 1]
      candidate = next ? next.id : DROP_END
    }
    dropBeforeId = wouldReorder(slides, draggingId, candidate, 'move') ? candidate : null
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
    const reordered = computeReorder(slides, draggingId, dropBeforeId, { mode: 'move' })
    handleDragEnd()
    if (reordered !== slides) commit(reordered)
  }
</script>

<div class="deck">
  {#if slides.length === 0}
    <div class="deck-empty">
      <p class="muted">{t('slide.empty')}</p>
    </div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <ul class="slide-list" role="list" ondrop={handleDrop} ondragover={(e) => { if (draggingId !== null) e.preventDefault() }}>
      {#each slides as slide, i (slide.id)}
        {#if draggingId !== null && dropBeforeId === slide.id}
          <div class="drop-indicator"></div>
        {/if}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <li
          class="slide-row"
          class:dragging={draggingId === slide.id}
          class:current={i === currentIndex}
          role="listitem"
          ondragover={(e) => handleDragOver(e, slide.id, i)}
        >
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <span
            class="drag-handle"
            draggable={true}
            ondragstart={(e) => handleDragStart(e, slide.id)}
            ondragend={handleDragEnd}
            role="button"
            tabindex="-1"
            aria-label={t('slide.reorder')}
            title={t('slide.reorder')}><GripVertical size={14} /></span>

          <button class="slide-open" type="button" onclick={() => (editingSlideId = slide.id)} title={t('slide.edit')}>
            <span class="thumb">
              {#if slide.thumbnail}
                <img src={slide.thumbnail} alt="" />
              {:else}
                <Presentation size={16} />
              {/if}
            </span>
            <span class="slide-title">{slideLabel(slide)}</span>
            <span class="ct-badge">{contentTypeName(slide)}</span>
            {#if slide.durationSec}
              <span class="duration"><Clock size={10} /> {slide.durationSec}s</span>
            {/if}
          </button>

          <div class="slide-actions">
            <button class="icon-btn" type="button" onclick={() => (editingSlideId = slide.id)} title={t('slide.edit')} aria-label={t('slide.edit')}>
              <Pencil size={13} />
            </button>
            <button class="icon-btn" type="button" onclick={() => duplicateSlide(slide)} title={t('common.duplicate')} aria-label={t('common.duplicate')}>
              <Copy size={13} />
            </button>
            <button class="icon-btn danger" type="button" onclick={() => deleteSlide(slide)} title={t('common.delete')} aria-label={t('common.delete')}>
              <Trash2 size={13} />
            </button>
          </div>
        </li>
      {/each}
      {#if draggingId !== null && dropBeforeId === DROP_END}
        <div class="drop-indicator"></div>
      {/if}
    </ul>
  {/if}

  <div class="deck-actions">
    <button class="add-slide" type="button" onclick={addSlide}>
      <Plus size={14} /> {t('slide.add')}
    </button>
    {#if slides.length > 0}
      <button class="present-btn" type="button" onclick={present} disabled={presenting} title={t('slide.present_tip')}>
        <Play size={13} /> {t('slide.present')}
      </button>
      <div class="nav-group" title={t('slide.nav_tip')}>
        <button class="icon-btn" type="button" onclick={() => goTo(currentIndex - 1)} disabled={currentIndex === 0} aria-label={t('slide.prev')}>
          <ChevronLeft size={14} />
        </button>
        <span class="nav-pos">{currentIndex + 1} / {slides.length}</span>
        <button class="icon-btn" type="button" onclick={() => goTo(currentIndex + 1)} disabled={currentIndex >= slides.length - 1} aria-label={t('slide.next')}>
          <ChevronRight size={14} />
        </button>
      </div>
    {/if}
  </div>
</div>

{#if editingSlide}
  <SlideEditorDialog
    slide={editingSlide}
    {cardId}
    onSave={saveSlide}
    onClose={() => (editingSlideId = null)}
  />
{/if}

<style>
  .deck {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .deck-empty {
    padding: 10px 0;
  }
  .muted {
    color: var(--text-muted);
    font-size: 12px;
    margin: 0;
  }
  .slide-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .slide-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 6px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .slide-row.dragging {
    opacity: 0.4;
  }
  .slide-row.current {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 6%, var(--bg));
  }
  .drop-indicator {
    height: 2px;
    background: var(--accent);
    border-radius: 1px;
    margin: 1px 0;
  }
  .drag-handle {
    display: inline-flex;
    align-items: center;
    color: var(--text-faint);
    cursor: grab;
    flex-shrink: 0;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .slide-row:hover .drag-handle,
  .drag-handle:focus-visible {
    opacity: 1;
  }
  .drag-handle:active {
    cursor: grabbing;
  }
  .slide-open {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    background: none;
    border: none;
    padding: 2px;
    cursor: pointer;
    color: var(--text-primary);
    text-align: left;
  }
  .thumb {
    width: 40px;
    height: 40px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-elevated);
    border-radius: 4px;
    color: var(--text-muted);
    overflow: hidden;
  }
  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .slide-title {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ct-badge {
    flex-shrink: 0;
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--text-muted);
    background: var(--bg-elevated);
    padding: 1px 6px;
    border-radius: 4px;
  }
  .duration {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-size: 10px;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .slide-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
    opacity: 0;
    transition: opacity var(--duration-fast) var(--ease-out);
  }
  .slide-row:hover .slide-actions,
  .slide-row:focus-within .slide-actions {
    opacity: 1;
  }
  .icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 3px;
    border-radius: 4px;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .icon-btn:hover {
    color: var(--text-primary);
    background: var(--bg-hover);
  }
  .icon-btn.danger:hover {
    color: var(--danger);
  }
  .deck-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .add-slide,
  .present-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text-muted);
    font-size: 12px;
    padding: 4px 10px;
    cursor: pointer;
  }
  .add-slide:hover {
    color: var(--text-primary);
    background: var(--bg-hover);
  }
  .present-btn {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  }
  .present-btn:hover {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }
  .present-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .nav-group {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    margin-left: auto;
  }
  .nav-group .icon-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .nav-pos {
    font-size: 11px;
    color: var(--text-muted);
    min-width: 3.2em;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
</style>
