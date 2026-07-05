<script lang="ts">
  import type { ComponentType, Snippet } from 'svelte'
  import { X, type Icon } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  // Generic right-hand side panel: one resizable, slide-animated container
  // hosting N tabs (VS Code-style bottom tab bar). Owns ALL geometry —
  // width, drag-to-resize, slide in/out — so hosted content (ChatSection in
  // hosted mode, WorkspacePanel) just fills 100%.
  //
  // Designed to stay the single seam for future layouts: a horizontal
  // split or a drag-drop panel scheme changes this component's internals
  // (render two panes instead of tab-switching), not its consumers — App
  // passes tabs + a content snippet keyed by tab id either way.

  export type SidePanelTab = { id: string; label: string; icon: ComponentType<Icon> }

  let {
    visible,
    tabs,
    activeTab = $bindable(''),
    onClose,
    children,
  }: {
    /** Controls the out animation: parent keeps the component mounted for
     *  the transition duration after setting visible=false (same DOM-lag
     *  pattern as the old project chat). */
    visible: boolean
    tabs: SidePanelTab[]
    activeTab: string
    onClose?: () => void
    /** Rendered once (not per tab) so inactive tab content keeps its DOM
     *  and state — wrap each tab's content in a pane hidden via CSS. */
    children: Snippet<[string]>
  } = $props()

  // --- Geometry (ported from ChatSection's shell) ---
  const WIDTH_KEY = 'bruv:sidePanelWidth'
  const MIN_WIDTH = 300
  const MAX_WIDTH = 860
  let width = $state(
    Number(localStorage.getItem(WIDTH_KEY)) ||
    Number(localStorage.getItem('bruv:chatPanelWidth')) || // migrate old chat width
    380
  )
  let resizing = $state(false)

  function onSplitterDown(e: MouseEvent) {
    e.preventDefault()
    resizing = true
    const startX = e.clientX
    const startW = width
    const pe = e as PointerEvent
    const target = e.currentTarget as HTMLElement | null
    const pointerId = pe.pointerId
    target?.setPointerCapture?.(pointerId)

    function onMove(ev: PointerEvent) {
      const next = startW - (ev.clientX - startX)
      width = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, next))
    }
    function onUp() {
      resizing = false
      localStorage.setItem(WIDTH_KEY, String(width))
      try { target?.releasePointerCapture?.(pointerId) } catch { /* already released */ }
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }

  // Width-collapse exit, mirroring the entrance keyframes (see ChatSection's
  // slideOutWidth for the reasoning: width not transform, WebView2 scroll).
  function slideOutWidth(node: HTMLElement, opts: { duration?: number } = {}) {
    const { duration = 220 } = opts
    const w = node.clientWidth
    return {
      duration,
      css: (t2: number) => `width: ${w * t2}px; opacity: ${t2}; overflow: hidden; min-width: 0;`,
    }
  }
</script>

{#if visible}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="side-panel" class:resizing style="width: {width}px; --sp-width: {width}px;" out:slideOutWidth={{ duration: 220 }}>
    <div class="sp-resize-handle" class:active={resizing} role="separator" tabindex="-1" onpointerdown={onSplitterDown}></div>
    <!-- Inner pinned to final width so content doesn't reflow during the
         width animation — same trick as .chat-panel-inner. -->
    <div class="side-panel-inner" style="width: {width}px;">
      <div class="sp-content">
        {@render children(activeTab)}
      </div>
      <div class="sp-tabbar" role="tablist">
        {#each tabs as tab (tab.id)}
          <button
            role="tab"
            aria-selected={activeTab === tab.id}
            class="sp-tab"
            class:active={activeTab === tab.id}
            onclick={() => activeTab = tab.id}
          >
            <tab.icon size={13} />
            <span>{tab.label}</span>
          </button>
        {/each}
        <button class="sp-close" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={14} /></button>
      </div>
    </div>
  </div>
{/if}

<style>
  .side-panel {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border-muted);
    background: var(--bg-base);
    position: relative;
    overflow: hidden;
    /* Width-based entrance, not transform — transforms create a stacking
       context that breaks inner scroll containers on WebView2. `backwards`
       prevents a 1-frame full-width flash, then the inline style resumes
       control so drag-to-resize keeps working. */
    animation: sp-panel-in 240ms cubic-bezier(0.16, 1, 0.3, 1) backwards;
  }

  @keyframes sp-panel-in {
    from { width: 0; opacity: 0; }
    to { width: var(--sp-width); opacity: 1; }
  }

  /* NOTE: no `animation: none` here — removing and re-adding the animation
     property (class toggle on drag end) RESTARTS it. The finished entrance
     animation is inert; leave it applied, exactly like .chat-panel does. */
  .side-panel.resizing {
    user-select: none;
  }

  .side-panel-inner {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }

  .sp-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }

  .sp-resize-handle {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 5px;
    z-index: 5;
    cursor: col-resize;
    transition: background 0.15s;
  }
  .sp-resize-handle:hover,
  .sp-resize-handle.active {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent-glow-1);
  }

  /* VS Code-style bottom tab bar */
  .sp-tabbar {
    display: flex;
    align-items: stretch;
    flex-shrink: 0;
    border-top: 1px solid var(--border-muted);
    background: var(--bg-base);
  }
  .sp-tab {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.4rem 0.75rem;
    border: none;
    border-top: 2px solid transparent;
    background: none;
    color: var(--text-muted);
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    transition: color 0.12s, border-color 0.12s;
  }
  .sp-tab:hover,
  .sp-tab:focus-visible {
    color: var(--text-primary);
  }
  .sp-tab.active {
    color: var(--accent);
    border-top-color: var(--accent);
  }
  .sp-close {
    margin-left: auto;
    display: flex;
    align-items: center;
    padding: 0.4rem 0.6rem;
    border: none;
    background: none;
    color: var(--text-faint);
    cursor: pointer;
    transition: color 0.12s;
  }
  .sp-close:hover,
  .sp-close:focus-visible {
    color: var(--text-primary);
  }
</style>
