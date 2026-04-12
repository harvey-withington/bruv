<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { ICON_CATEGORIES } from '../lib/icons'
  import DynamicIcon from './DynamicIcon.svelte'
  import ColorPicker from './ColorPicker.svelte'
  import { X, Upload, Droplet, ImagePlus, RotateCcw } from 'lucide-svelte'
  import { focusOnMount } from '../lib/actions'
  import { showToast } from '../lib/toast.svelte'
  import {
    loadImageFromFile,
    loadImageFromUrl,
    bakeIconFromImage,
    parseIconValue,
    withIconColor,
    ImageIconError,
    ACCEPTED_IMAGE_TYPES,
    type IconEditorTransform,
  } from '../lib/imageIcon'

  let {
    value = '',
    onSelect,
    onClose,
  }: {
    value?: string
    onSelect: (icon: string) => void
    onClose: () => void
  } = $props()

  const EDITOR_PREVIEW_SIZE = 180
  type Tab = 'icon' | 'image'

  // ---- Initial state derived from incoming value ------------------------------------
  const initialParsed = (() => parseIconValue(value))()
  const initialTab: Tab = initialParsed.inner.startsWith('data:') ? 'image' : 'icon'
  const initialIconName = initialParsed.inner.startsWith('data:') ? '' : initialParsed.inner

  let activeTab = $state<Tab>(initialTab)
  // svelte-ignore state_referenced_locally
  let pickedColor = $state<string | null>(initialParsed.color)
  let showColorPicker = $state(false)

  // ---- Built-in icon tab state ------------------------------------------------------
  let search = $state('')
  // svelte-ignore state_referenced_locally
  let selectedIconName = $state(initialIconName)

  const filteredCategories = $derived.by(() => {
    if (!search) return ICON_CATEGORIES
    const s = search.toLowerCase()
    const result: Record<string, string[]> = {}
    for (const [cat, icons] of Object.entries(ICON_CATEGORIES)) {
      const filtered = icons.filter(icon => icon.includes(s) || cat.toLowerCase().includes(s))
      if (filtered.length > 0) result[cat] = filtered
    }
    return result
  })

  // ---- Custom image tab state -------------------------------------------------------
  let fileInput: HTMLInputElement | null = $state(null)
  let editorImg = $state<HTMLImageElement | null>(null)
  let editorScale = $state(1)
  let editorOffsetX = $state(0)
  let editorOffsetY = $state(0)
  let processing = $state(false)
  let dragOver = $state(false)

  // Drag-to-pan inside the editor
  let panning = $state(false)
  let panStartX = 0
  let panStartY = 0
  let panOriginOffsetX = 0
  let panOriginOffsetY = 0

  // Pre-load existing data URL into the editor when opening on the image tab.
  // We can only get the (already-baked) 64×64, but it's enough to re-position
  // the existing icon if the user wants to tweak.
  $effect(() => {
    if (initialTab !== 'image') return
    if (!initialParsed.inner.startsWith('data:')) return
    let cancelled = false
    loadImageFromUrl(initialParsed.inner)
      .then(img => { if (!cancelled) editorImg = img })
      .catch(() => { /* silent — user can re-upload */ })
    return () => { cancelled = true }
  })

  // Live preview value (used by the in-grid colour previews and editor preview)
  const previewColor = $derived(pickedColor ?? '')

  // ---- File loading ------------------------------------------------------------------
  async function handleFileList(files: FileList | File[] | null | undefined) {
    if (!files || files.length === 0) return
    const file = (files as FileList)[0]
    if (!file) return
    processing = true
    try {
      const img = await loadImageFromFile(file)
      editorImg = img
      resetEditor()
    } catch (err) {
      reportImageError(err)
    } finally {
      processing = false
    }
  }

  function reportImageError(err: unknown) {
    if (err instanceof ImageIconError) {
      const key =
        err.code === 'too-large' ? 'icon.upload_too_large'
        : err.code === 'unsupported' ? 'icon.upload_unsupported'
        : 'icon.upload_failed'
      showToast(t(key), 'error')
    } else {
      showToast(t('icon.upload_failed'), 'error')
    }
  }

  function resetEditor() {
    editorScale = 1
    editorOffsetX = 0
    editorOffsetY = 0
  }

  function clearEditor() {
    editorImg = null
    resetEditor()
  }

  function openFileDialog() {
    fileInput?.click()
  }

  function handleFileChange(e: Event) {
    const input = e.target as HTMLInputElement
    handleFileList(input.files)
    input.value = ''
  }

  // ---- Drag/drop --------------------------------------------------------------------
  function onDragEnter(e: DragEvent) {
    if (!hasFiles(e)) return
    e.preventDefault()
    dragOver = true
  }
  function onDragOver(e: DragEvent) {
    if (!hasFiles(e)) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
    dragOver = true
  }
  function onDragLeave(e: DragEvent) {
    // Only clear when leaving the drop zone, not transitioning between children.
    if (e.currentTarget === e.target) dragOver = false
  }
  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragOver = false
    handleFileList(e.dataTransfer?.files)
  }
  function hasFiles(e: DragEvent): boolean {
    return Array.from(e.dataTransfer?.types ?? []).includes('Files')
  }

  // ---- Paste -----------------------------------------------------------------------
  function onPaste(e: ClipboardEvent) {
    if (activeTab !== 'image') return
    const items = e.clipboardData?.items
    if (!items) return
    for (const item of items) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile()
        if (file) {
          e.preventDefault()
          handleFileList([file])
          return
        }
      }
    }
  }

  // ---- Editor pan -------------------------------------------------------------------
  function onPanStart(e: PointerEvent) {
    if (!editorImg) return
    panning = true
    panStartX = e.clientX
    panStartY = e.clientY
    panOriginOffsetX = editorOffsetX
    panOriginOffsetY = editorOffsetY
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }
  function onPanMove(e: PointerEvent) {
    if (!panning) return
    editorOffsetX = panOriginOffsetX + (e.clientX - panStartX)
    editorOffsetY = panOriginOffsetY + (e.clientY - panStartY)
  }
  function onPanEnd(e: PointerEvent) {
    if (!panning) return
    panning = false
    ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
  }

  // ---- Editor display maths (mirrored in bakeIconFromImage) -------------------------
  const editorBaseScale = $derived(
    editorImg ? Math.min(EDITOR_PREVIEW_SIZE / editorImg.width, EDITOR_PREVIEW_SIZE / editorImg.height) : 1
  )
  const editorImgStyle = $derived.by(() => {
    if (!editorImg) return ''
    const w = editorImg.width * editorBaseScale * editorScale
    const h = editorImg.height * editorBaseScale * editorScale
    const left = EDITOR_PREVIEW_SIZE / 2 + editorOffsetX - w / 2
    const top = EDITOR_PREVIEW_SIZE / 2 + editorOffsetY - h / 2
    return `width:${w}px;height:${h}px;left:${left}px;top:${top}px;`
  })

  // ---- Selection / Apply ------------------------------------------------------------
  const canApply = $derived(
    (activeTab === 'icon' && !!selectedIconName) ||
    (activeTab === 'image' && !!editorImg)
  )

  function applyAndClose() {
    if (!canApply) return
    if (activeTab === 'icon') {
      onSelect(withIconColor(selectedIconName, pickedColor))
    } else if (editorImg) {
      const transform: IconEditorTransform = {
        previewSize: EDITOR_PREVIEW_SIZE,
        scale: editorScale,
        offsetX: editorOffsetX,
        offsetY: editorOffsetY,
      }
      const dataUrl = bakeIconFromImage(editorImg, transform)
      onSelect(withIconColor(dataUrl, pickedColor))
    }
    onClose()
  }

  function clearIcon() {
    onSelect('')
    onClose()
  }

  function selectIcon(name: string) {
    selectedIconName = name
  }

  function dblClickIcon(name: string) {
    selectedIconName = name
    activeTab = 'icon'
    applyAndClose()
  }

  // ---- Color picker -----------------------------------------------------------------
  function openColorPicker() {
    showColorPicker = true
  }
  function handleColorApply(c: string) {
    pickedColor = c
  }
  function clearColor() {
    pickedColor = null
  }

  // ---- Dialog-level keyboard --------------------------------------------------------
  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('icon-picker-backdrop')) {
      onClose()
    }
  }
  function handleKeydown(e: KeyboardEvent) {
    if (showColorPicker) return // child handles its own keys
    if (e.key === 'Escape') {
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      applyAndClose()
    } else if (activeTab === 'icon' && (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      handleGridArrow(e)
    }
  }

  /**
   * Move keyboard focus through the icon grid using arrow keys.
   *
   * The grid uses CSS `auto-fill` so column count is dynamic. Rather than
   * computing it, we use `document.elementFromPoint()` to find the cell
   * visually adjacent to the currently-focused one. This naturally handles
   * non-uniform layouts (e.g. crossing category boundaries with different
   * trailing-row widths).
   *
   * Triggered from the search field too: pressing Down from the search input
   * focuses the first cell in the grid, so users can flow from typing to
   * picking without reaching for the mouse.
   */
  function handleGridArrow(e: KeyboardEvent) {
    const active = document.activeElement as HTMLElement | null
    const isSearch = active?.classList.contains('ip-search')
    const isCell = active?.classList.contains('ip-icon-cell')
    if (!isSearch && !isCell) return

    // Down from search → focus first visible cell.
    if (isSearch) {
      if (e.key !== 'ArrowDown') return
      const first = document.querySelector('.ip-icon-cell') as HTMLButtonElement | null
      if (!first) return
      e.preventDefault()
      first.focus()
      first.scrollIntoView({ block: 'nearest' })
      return
    }

    // From a cell — find the visually adjacent cell.
    e.preventDefault()
    const cells = Array.from(document.querySelectorAll('.ip-icon-cell')) as HTMLButtonElement[]
    if (cells.length === 0) return
    const idx = cells.indexOf(active as HTMLButtonElement)
    if (idx === -1) return

    let next: HTMLButtonElement | null = null
    if (e.key === 'ArrowLeft') {
      next = cells[Math.max(0, idx - 1)]
    } else if (e.key === 'ArrowRight') {
      next = cells[Math.min(cells.length - 1, idx + 1)]
    } else {
      // Up / Down — probe geometrically with elementFromPoint.
      const rect = active!.getBoundingClientRect()
      const probeX = rect.left + rect.width / 2
      const probeY = e.key === 'ArrowDown'
        ? rect.bottom + rect.height / 2
        : rect.top - rect.height / 2
      const hit = document.elementFromPoint(probeX, probeY)
      const cell = hit?.closest('.ip-icon-cell') as HTMLButtonElement | null
      if (cell) {
        next = cell
      } else if (e.key === 'ArrowDown') {
        // Fell off the bottom of the grid — just go to last cell.
        next = cells[cells.length - 1]
      } else {
        // Fell off the top — go back to search input.
        const search = document.querySelector('.ip-search') as HTMLInputElement | null
        search?.focus()
        return
      }
    }
    if (next && next !== active) {
      next.focus()
      next.scrollIntoView({ block: 'nearest' })
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} onpaste={onPaste} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="icon-picker-backdrop" role="presentation" onclick={handleBackdropClick} out:fade={{ duration: 150 }}>
  <div class="icon-picker" role="dialog" aria-modal="true" aria-label={t('icon.pick')}>
    <!-- Tabs -->
    <div class="ip-tabs" role="tablist">
      <button
        class="ip-tab"
        class:active={activeTab === 'icon'}
        role="tab"
        aria-selected={activeTab === 'icon'}
        onclick={() => (activeTab = 'icon')}
      >
        {t('icon.tab_builtin')}
      </button>
      <button
        class="ip-tab"
        class:active={activeTab === 'image'}
        role="tab"
        aria-selected={activeTab === 'image'}
        onclick={() => (activeTab = 'image')}
      >
        {t('icon.tab_image')}
      </button>
      <div class="ip-tabs-spacer"></div>
      <button class="ip-close" onclick={onClose} title={t('common.close')}><X size={16} /></button>
    </div>

    <!-- Tab content -->
    {#if activeTab === 'icon'}
      <div class="ip-tab-body">
        <div class="ip-search-row">
          <input
            type="text"
            class="ip-search"
            placeholder={t('icon.search')}
            bind:value={search}
            use:focusOnMount
          />
        </div>
        <div class="ip-icon-grid-wrap" style={previewColor ? `color:${previewColor}` : ''}>
          {#each Object.entries(filteredCategories) as [category, icons]}
            <div class="ip-category">
              <div class="ip-category-label">{category}</div>
              <div class="ip-icon-grid">
                {#each icons as icon}
                  <button
                    class="ip-icon-cell"
                    class:selected={icon === selectedIconName}
                    onclick={() => selectIcon(icon)}
                    ondblclick={() => dblClickIcon(icon)}
                    title={icon}
                  >
                    <DynamicIcon name={icon} size={18} />
                  </button>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="ip-tab-body">
        {#if !editorImg}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="ip-dropzone"
            class:dragover={dragOver}
            ondragenter={onDragEnter}
            ondragover={onDragOver}
            ondragleave={onDragLeave}
            ondrop={onDrop}
            onclick={openFileDialog}
          >
            <ImagePlus size={36} />
            <p class="ip-dropzone-title">{t('icon.dropzone_title')}</p>
            <p class="ip-dropzone-hint">{t('icon.dropzone_hint')}</p>
            {#if processing}
              <p class="ip-dropzone-processing">{t('app.loading')}</p>
            {/if}
          </div>
        {:else}
          <div class="ip-editor">
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="ip-editor-frame"
              class:tinted={!!pickedColor}
              style="--frame-size:{EDITOR_PREVIEW_SIZE}px;{pickedColor ? `color:${pickedColor};` : ''}"
              onpointerdown={onPanStart}
              onpointermove={onPanMove}
              onpointerup={onPanEnd}
              onpointercancel={onPanEnd}
            >
              {#if pickedColor}
                <div
                  class="ip-editor-mask"
                  style="{editorImgStyle} -webkit-mask-image:url('{editorImg.src}'); mask-image:url('{editorImg.src}');"
                ></div>
              {:else}
                <img
                  class="ip-editor-img"
                  src={editorImg.src}
                  alt=""
                  draggable="false"
                  style={editorImgStyle}
                />
              {/if}
            </div>
            <div class="ip-editor-controls">
              <label class="ip-scale">
                <span>{t('icon.scale')}</span>
                <input type="range" min="0.2" max="3" step="0.01" bind:value={editorScale} />
              </label>
              <div class="ip-editor-actions">
                <button class="ip-mini-btn" onclick={resetEditor} title={t('icon.reset_position')}>
                  <RotateCcw size={12} />
                  {t('icon.reset')}
                </button>
                <button class="ip-mini-btn" onclick={openFileDialog} title={t('icon.replace_image')}>
                  <Upload size={12} />
                  {t('icon.replace')}
                </button>
                <button class="ip-mini-btn danger" onclick={clearEditor}>
                  <X size={12} />
                  {t('common.clear')}
                </button>
              </div>
            </div>
            <p class="ip-editor-hint">{t('icon.editor_hint')}</p>
          </div>
        {/if}
        <input
          bind:this={fileInput}
          type="file"
          accept={ACCEPTED_IMAGE_TYPES}
          class="ip-hidden-file"
          onchange={handleFileChange}
        />
      </div>
    {/if}

    <!-- Shared toolbar: colour + clear current -->
    <div class="ip-toolbar">
      <div class="ip-color-group">
        <span class="ip-toolbar-label">{t('icon.color')}</span>
        <button
          class="ip-color"
          onclick={openColorPicker}
          title={pickedColor ? t('icon.color_change') : t('icon.color_pick')}
        >
          {#if pickedColor}
            <span class="ip-color-swatch" style="background:{pickedColor}"></span>
          {:else}
            <span class="ip-color-swatch ip-color-swatch-default" title={t('icon.color_default')}>
              <Droplet size={12} />
            </span>
          {/if}
        </button>
        {#if pickedColor}
          <button class="ip-color-clear" onclick={clearColor} title={t('icon.color_reset')}>
            <X size={12} />
          </button>
        {/if}
      </div>
      {#if value}
        <button class="ip-remove-btn" onclick={clearIcon}>{t('icon.clear')}</button>
      {/if}
    </div>

    <!-- Footer: Apply / Cancel -->
    <div class="ip-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn-primary" onclick={applyAndClose} disabled={!canApply}>
        {t('common.apply')}
      </button>
    </div>
  </div>
</div>

{#if showColorPicker}
  <ColorPicker
    initial={pickedColor ?? '#3b82f6'}
    onApply={handleColorApply}
    onClose={() => (showColorPicker = false)}
  />
{/if}

<style>
  .icon-picker-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.3);
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .icon-picker {
    width: 440px;
    max-height: 600px;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  /* Tabs */
  .ip-tabs {
    display: flex;
    align-items: stretch;
    border-bottom: 1px solid var(--border);
    padding: 0 6px;
    gap: 2px;
  }
  .ip-tab {
    background: none;
    border: none;
    color: var(--text-muted);
    padding: 12px 14px;
    font-size: 0.82rem;
    font-weight: 500;
    cursor: pointer;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }
  .ip-tab:hover {
    color: var(--text-primary);
  }
  .ip-tab.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }
  .ip-tabs-spacer {
    flex: 1;
  }
  .ip-close {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 8px;
    border-radius: 4px;
    align-self: center;
  }
  .ip-close:hover {
    color: var(--text-primary);
  }

  .ip-tab-body {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding: 12px;
    overflow: hidden;
  }

  /* Built-in icon tab */
  .ip-search-row {
    margin-bottom: 10px;
  }
  .ip-search {
    width: 100%;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-main);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    box-sizing: border-box;
  }
  .ip-search:focus {
    border-color: var(--accent);
  }
  .ip-icon-grid-wrap {
    flex: 1;
    overflow-y: auto;
    color: var(--text-primary);
  }
  .ip-category {
    margin-bottom: 12px;
  }
  .ip-category-label {
    font-size: 0.7em;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 6px;
  }
  .ip-icon-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(36px, 1fr));
    gap: 4px;
  }
  .ip-icon-cell {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 6px;
    border: 1px solid transparent;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .ip-icon-cell:hover {
    background: var(--bg-hover);
  }
  .ip-icon-cell.selected {
    background: var(--accent-bg);
    color: var(--accent);
    border-color: var(--accent);
  }

  /* Custom image tab */
  .ip-dropzone {
    flex: 1;
    border: 2px dashed var(--border);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 24px;
    color: var(--text-muted);
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s, color 0.15s;
  }
  .ip-dropzone:hover,
  .ip-dropzone.dragover {
    border-color: var(--accent);
    background: var(--accent-bg);
    color: var(--accent);
  }
  .ip-dropzone-title {
    margin: 0;
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .ip-dropzone-hint {
    margin: 0;
    font-size: 0.78rem;
    color: var(--text-muted);
    text-align: center;
  }
  .ip-dropzone-processing {
    margin: 0;
    font-size: 0.78rem;
    color: var(--accent);
  }
  .ip-hidden-file {
    display: none;
  }
  .ip-editor {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }
  .ip-editor-frame {
    width: var(--frame-size);
    height: var(--frame-size);
    position: relative;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: 8px;
    background:
      repeating-conic-gradient(var(--bg-hover) 0% 25%, var(--bg-main) 0% 50%) 50% / 20px 20px;
    cursor: grab;
    touch-action: none;
  }
  .ip-editor-frame:active {
    cursor: grabbing;
  }
  .ip-editor-img {
    position: absolute;
    user-select: none;
    pointer-events: none;
  }
  .ip-editor-mask {
    position: absolute;
    background-color: currentColor;
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: center;
    mask-position: center;
    -webkit-mask-size: contain;
    mask-size: contain;
    pointer-events: none;
  }
  .ip-editor-controls {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .ip-scale {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .ip-scale input[type='range'] {
    flex: 1;
    accent-color: var(--accent);
  }
  .ip-editor-actions {
    display: flex;
    gap: 6px;
    justify-content: center;
  }
  .ip-mini-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    font-size: 0.72rem;
    background: var(--bg-main);
    color: var(--text-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    cursor: pointer;
  }
  .ip-mini-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
  .ip-mini-btn.danger:hover {
    color: var(--danger);
    border-color: var(--danger);
  }
  .ip-editor-hint {
    margin: 0;
    font-size: 0.7rem;
    color: var(--text-muted);
    text-align: center;
  }

  /* Toolbar (colour) */
  .ip-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 12px;
    border-top: 1px solid var(--border);
    background: var(--bg-main);
  }
  .ip-color-group {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .ip-toolbar-label {
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .ip-color {
    background: none;
    border: 1px solid var(--border);
    cursor: pointer;
    padding: 3px;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ip-color:hover {
    border-color: var(--accent);
  }
  .ip-color-swatch {
    width: 18px;
    height: 18px;
    border-radius: 3px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .ip-color-swatch-default {
    background: repeating-conic-gradient(var(--bg-hover) 0% 25%, var(--bg-main) 0% 50%) 50% / 8px 8px;
    color: var(--text-muted);
  }
  .ip-color-clear {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 2px;
    border-radius: 4px;
    display: inline-flex;
  }
  .ip-color-clear:hover {
    color: var(--danger);
  }
  .ip-remove-btn {
    padding: 4px 8px;
    font-size: 0.75rem;
    background: none;
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
  }
  .ip-remove-btn:hover {
    color: var(--danger);
    border-color: var(--danger);
  }

  /* Footer */
  .ip-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }
  .btn-secondary,
  .btn-primary {
    padding: 6px 14px;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
    border: 1px solid var(--border);
  }
  .btn-secondary {
    background: var(--bg-main);
    color: var(--text-primary);
  }
  .btn-secondary:hover {
    background: var(--bg-hover);
  }
  .btn-primary {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .btn-primary:hover:not(:disabled) {
    filter: brightness(1.08);
  }
  .btn-primary:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
