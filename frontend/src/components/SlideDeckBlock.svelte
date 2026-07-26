<script lang="ts">
  import type { SlideDeckValue, Slide, BlockLiveState, WailsWindow } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { Plus, GripVertical, Pencil, Copy, Trash2, Presentation, Clock, Play, Pause, Square, MonitorPlay, ChevronLeft, ChevronRight, Link2, ExternalLink, Maximize2, Minimize2 } from 'lucide-svelte'
  import { computeReorder, wouldReorder, DROP_END } from '../lib/reorder'
  import { resolveContentType, DEFAULT_CONTENT_TYPE_ID } from '@shared/slideContentTypes'
  import { slideDisplayLabel } from '@shared/slideLabel'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { GetCard, SignPresentURL, SetBlockLiveState, GetBlockLiveState, ListPresentingCards, SetPresenting } from '@shared/api'
  import { onEvent } from '../lib/events'
  import SlideEditorDialog from './SlideEditorDialog.svelte'

  let {
    value,
    cardId,
    blockId,
    onUpdate,
  }: {
    value: SlideDeckValue
    cardId: string
    blockId: string
    onUpdate: (val: SlideDeckValue) => void
  } = $props()

  const slides = $derived<Slide[]>(value?.slides ?? [])

  let editingSlideId = $state<string | null>(null)
  const editingSlide = $derived<Slide | null>(
    editingSlideId ? slides.find((s) => s.id === editingSlideId) ?? null : null,
  )

  // Row labels follow the linked card's LIVE title unless the slide carries
  // an explicit title override (see shared/slideLabel.ts for the priority
  // order). Titles are fetched once per linked card and refreshed on
  // card:updated, so renaming a clipped card renames its slide row.
  let linkedTitles = $state<Record<string, string>>({})
  const requestedTitleIds = new Set<string>()
  function loadLinkedTitle(id: string): void {
    // A failed load (e.g. the linked card was deleted) just leaves the
    // label on its value/content-type fallback — nothing to surface.
    GetCard(id)
      .then((c) => {
        linkedTitles[id] = c.title
      })
      .catch(() => {})
  }
  $effect(() => {
    for (const s of slides) {
      if (s.cardId && !requestedTitleIds.has(s.cardId)) {
        requestedTitleIds.add(s.cardId)
        loadLinkedTitle(s.cardId)
      }
    }
  })
  $effect(() => {
    return onEvent<{ cardID?: string }>('card:updated', (ev) => {
      if (ev?.cardID && requestedTitleIds.has(ev.cardID)) loadLinkedTitle(ev.cardID)
    })
  })

  function slideLabel(slide: Slide): string {
    const label = slideDisplayLabel(slide, slide.cardId ? linkedTitles[slide.cardId] : undefined)
    if (label) return label
    return resolveContentType(slide.contentTypeId) ? t('slide.ct.' + slide.contentTypeId) : t('slide.untitled')
  }

  function contentTypeName(slide: Slide): string {
    return resolveContentType(slide.contentTypeId) ? t('slide.ct.' + slide.contentTypeId) : slide.contentTypeId
  }

  function newSlideId(): string {
    return `sld-${crypto.randomUUID().slice(0, 8)}`
  }

  function commit(nextSlides: Slide[]): void {
    const next: SlideDeckValue = { ...value, slides: nextSlides }
    delete next.currentIndex // legacy persisted position — live state owns it now
    onUpdate(next)
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
  // The position is block LIVE state: server-held in memory, broadcast as
  // `block:live`, never persisted — advancing a slide is session state, not
  // a card edit, so it must not write the card / log activity / bump
  // UpdatedAt. The /present page receives it via the PresentCardJSON
  // overlay, so presenting "from slide N" is free: wherever this console
  // last pointed is where the output page starts.
  let liveIndex = $state(0)
  const currentIndex = $derived(
    Math.min(Math.max(liveIndex, 0), Math.max(slides.length - 1, 0)),
  )

  $effect(() => {
    let alive = true
    // A failed/absent read just means "start at slide 1" — nothing to surface.
    GetBlockLiveState(cardId, blockId)
      .then((s) => {
        if (alive && typeof s?.currentIndex === 'number') liveIndex = s.currentIndex
      })
      .catch(() => {})
    const unsub = onEvent<{ cardID?: string; blockID?: string; state?: BlockLiveState }>('block:live', (ev) => {
      if (ev.cardID !== cardId || ev.blockID !== blockId) return
      if (typeof ev.state?.currentIndex === 'number') liveIndex = ev.state.currentIndex
      // Mirror the video transport across consoles: a state without a
      // videoAction (e.g. plain navigation) means paused, not fullscreen.
      videoPlaying = ev.state?.videoAction === 'play'
      videoFull = ev.state?.videoFull === true
    })
    return () => {
      alive = false
      unsub()
    }
  })

  async function goTo(idx: number): Promise<void> {
    if (idx < 0 || idx >= slides.length || idx === currentIndex) return
    const prev = liveIndex
    liveIndex = idx // optimistic; the block:live echo confirms
    videoPlaying = false // navigation clears video command + fullscreen (state is replaced wholesale)
    videoFull = false
    try {
      await SetBlockLiveState(cardId, blockId, { currentIndex: idx })
    } catch {
      liveIndex = prev
      showToast(t('slide.nav_failed'), 'error')
    }
  }

  // --- video transport (presenter-fired playback) ---
  // Videos never autoplay on the present page — a streamer sets the slide
  // up verbally, then fires it from here. Each press bumps videoSeq so the
  // page can distinguish new commands; navigation resets to paused.
  const currentHasVideo = $derived.by(() => {
    const s = slides[currentIndex]
    return !!(s && (s.values?.video || s.bindings?.video))
  })
  let videoSeq = $state(0)
  let videoPlaying = $state(false)
  let videoFull = $state(false)

  async function toggleVideo(): Promise<void> {
    const next = !videoPlaying
    videoPlaying = next
    videoSeq += 1
    try {
      await SetBlockLiveState(cardId, blockId, {
        currentIndex,
        videoSeq,
        videoAction: next ? 'play' : 'pause',
        videoFull,
      })
    } catch {
      videoPlaying = !next
      showToast(t('slide.nav_failed'), 'error')
    }
  }

  // Fullscreen toggle keeps videoSeq UNCHANGED — enlarging must not
  // restart or re-fire playback, only relayout the output page.
  async function toggleVideoFull(): Promise<void> {
    const next = !videoFull
    videoFull = next
    try {
      await SetBlockLiveState(cardId, blockId, {
        currentIndex,
        videoSeq,
        videoAction: videoPlaying ? 'play' : 'pause',
        videoFull: next,
      })
    } catch {
      videoFull = !next
      showToast(t('slide.nav_failed'), 'error')
    }
  }

  // Live "presentation ongoing" state: the server flags a card while its
  // /present-data endpoint is being polled (OBS or a tab), with
  // present:active/idle transition events. The Present button restyles and
  // relabels while live — otherwise nothing on the card says it's on air.
  let presentingLive = $state(false)
  $effect(() => {
    let alive = true
    ListPresentingCards()
      .then((ids) => {
        if (alive) presentingLive = (ids ?? []).includes(cardId)
      })
      .catch(() => {})
    const unsubActive = onEvent<{ cardID?: string }>('present:active', (ev) => {
      if (ev.cardID === cardId) presentingLive = true
    })
    const unsubIdle = onEvent<{ cardID?: string }>('present:idle', (ev) => {
      if (ev.cardID === cardId) presentingLive = false
    })
    return () => {
      alive = false
      unsubActive()
      unsubIdle()
    }
  })

  // Present is a START/STOP toggle on the server-side presentation gate:
  // output pages only receive deck content while it's open, so stopping
  // genuinely stops broadcasting even though the signed URL stays valid
  // (pages show a waiting state and resume on restart — OBS scenes can be
  // set up before going live). Opening the page is its own button.
  let toggling = $state(false)
  async function togglePresent(): Promise<void> {
    if (toggling) return
    toggling = true
    const next = !presentingLive
    presentingLive = next // optimistic; the present:active/idle echo confirms
    try {
      await SetPresenting(cardId, next)
    } catch {
      presentingLive = !next
      showToast(t('slide.present_toggle_failed'), 'error')
    } finally {
      toggling = false
    }
  }

  let opening = $state(false)
  async function openPresent(): Promise<void> {
    if (opening) return
    opening = true
    try {
      const url = await SignPresentURL(cardId)
      const w = window as WailsWindow
      if (w.runtime?.BrowserOpenURL) {
        w.runtime.BrowserOpenURL(url)
      } else {
        window.open(url, '_blank', 'noopener')
      }
    } catch {
      showToast(t('slide.present_failed'), 'error')
    } finally {
      opening = false
    }
  }

  let copying = $state(false)
  async function copyObsUrl(): Promise<void> {
    if (copying) return
    copying = true
    try {
      const url = await SignPresentURL(cardId)
      await navigator.clipboard.writeText(url)
      showToast(t('slide.present_copied'), 'success')
    } catch {
      showToast(t('slide.copy_failed'), 'error')
    } finally {
      copying = false
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

  <!-- Toolbar: authoring on the left, slide transport centered, presenter
       controls (video, Present, OBS link, future fullscreen…) on the right. -->
  <div class="deck-actions" class:has-slides={slides.length > 0}>
    <div class="actions-left">
      <button class="add-slide" type="button" onclick={addSlide}>
        <Plus size={14} /> {t('slide.add')}
      </button>
      {#if slides.length > 0}
        <!-- Present stays a full labelled button (and on the left) so it
             can't be missed; the right group is icon-only controls. -->
        <button
          class="present-btn"
          class:live={presentingLive}
          type="button"
          onclick={togglePresent}
          disabled={toggling}
          title={presentingLive ? t('slide.presenting_tip') : t('slide.present_tip')}
        >
          {#if presentingLive}<Square size={12} /> {t('slide.stop_presenting')}{:else}<Play size={13} /> {t('slide.present')}{/if}
        </button>
      {/if}
    </div>
    {#if slides.length > 0}
      <div class="nav-group" title={t('slide.nav_tip')}>
        <button class="icon-btn" type="button" onclick={() => goTo(currentIndex - 1)} disabled={currentIndex === 0} aria-label={t('slide.prev')}>
          <ChevronLeft size={14} />
        </button>
        <span class="nav-pos">{currentIndex + 1} / {slides.length}</span>
        <button class="icon-btn" type="button" onclick={() => goTo(currentIndex + 1)} disabled={currentIndex >= slides.length - 1} aria-label={t('slide.next')}>
          <ChevronRight size={14} />
        </button>
      </div>
      <div class="actions-right">
        {#if currentHasVideo}
          <button
            class="icon-btn video-btn"
            class:playing={videoPlaying}
            type="button"
            onclick={toggleVideo}
            title={videoPlaying ? t('slide.pause_video') : t('slide.play_video')}
            aria-label={videoPlaying ? t('slide.pause_video') : t('slide.play_video')}
          >
            {#if videoPlaying}<Pause size={13} />{:else}<MonitorPlay size={14} />{/if}
          </button>
          <button
            class="icon-btn video-btn"
            class:playing={videoFull}
            type="button"
            onclick={toggleVideoFull}
            title={videoFull ? t('slide.video_full_exit') : t('slide.video_full')}
            aria-label={videoFull ? t('slide.video_full_exit') : t('slide.video_full')}
          >
            {#if videoFull}<Minimize2 size={13} />{:else}<Maximize2 size={13} />{/if}
          </button>
        {/if}
        <button class="icon-btn" type="button" onclick={openPresent} disabled={opening} title={t('slide.open_present_tip')} aria-label={t('slide.open_present_tip')}>
          <ExternalLink size={14} />
        </button>
        <button class="icon-btn" type="button" onclick={copyObsUrl} disabled={copying} title={t('slide.copy_obs_tip')} aria-label={t('slide.copy_obs_tip')}>
          <Link2 size={14} />
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
  .deck-actions.has-slides {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
  }
  .actions-left {
    display: flex;
    align-items: center;
    gap: 6px;
    justify-content: flex-start;
  }
  .actions-right {
    display: flex;
    align-items: center;
    gap: 4px;
    justify-content: flex-end;
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
  .present-btn.live {
    color: #fff;
    border-color: transparent;
    background: var(--agent-running-gradient);
    background-size: 300% 300%;
    animation: present-live 2s ease infinite;
    box-shadow: var(--agent-running-glow);
  }
  @keyframes present-live {
    0% { background-position: 0% 50%; }
    50% { background-position: 100% 50%; }
    100% { background-position: 0% 50%; }
  }
  .nav-group {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .nav-group .icon-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .video-btn {
    color: var(--accent);
  }
  .video-btn.playing {
    color: var(--warning-text, var(--accent));
  }
  .nav-pos {
    font-size: 11px;
    color: var(--text-muted);
    min-width: 3.2em;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
</style>
