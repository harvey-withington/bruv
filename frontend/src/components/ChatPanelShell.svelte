<script lang="ts">
  // The card chat's docked panel: its width, the splitter against the
  // card's main column, and the slide in/out. Whichever chat engine runs
  // (ChatSection, UncliChat) renders inside it. Hosted: a parent
  // container (SidePanel) owns all of that, and the panel just fills it.
  import type { Snippet } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { keyboardResize } from '../lib/actions'

  let {
    visible,
    hosted = false,
    mainWidth = $bindable(720),
    splitterDragging = $bindable(false),
    children,
  }: {
    visible: boolean
    hosted?: boolean
    mainWidth?: number
    splitterDragging?: boolean
    children: Snippet
  } = $props()

  // Slide the panel's width out to 0 on unmount, mirroring the CSS
  // `chat-panel-in` keyframe used on mount (240ms, the same easing), so
  // closing plays as the true reverse of opening. It measures the width
  // the user dragged to at transition start, and animates width +
  // opacity only: a transform would break the scroll container inside
  // on WebView2. CardDetail keeps the modal wide for the same duration
  // (CHAT_OUT_DURATION) so the dialog doesn't snap narrow first.
  const easing = (p: number) => 1 - Math.pow(1 - p, 4) // ≈ cubic-bezier(0.16, 1, 0.3, 1)
  function slideOutWidth(node: HTMLElement, opts: { duration?: number } = {}) {
    const width = node.clientWidth
    return {
      duration: opts.duration ?? 240,
      easing,
      css: (p: number) => `width: ${width * p}px; opacity: ${p}; overflow: hidden; min-width: 0;`,
    }
  }

  // Resizable width (splitter between main and chat).
  const CHAT_WIDTH_KEY = 'bruv:chatPanelWidth'
  const MIN_PANEL = 300
  let chatWidth = $state(Number(localStorage.getItem(CHAT_WIDTH_KEY)) || 380)
  let resizing = $state(false)

  /** Moves the main|chat boundary by `delta` px from the given widths,
   *  unless that would squeeze either side below MIN_PANEL. */
  function moveSplitter(fromMain: number, fromChat: number, delta: number) {
    if (fromMain + delta >= MIN_PANEL && fromChat - delta >= MIN_PANEL) {
      mainWidth = fromMain + delta
      chatWidth = fromChat - delta
    }
  }
  function saveSplit() {
    localStorage.setItem(CHAT_WIDTH_KEY, String(chatWidth))
    localStorage.setItem('bruv:mainPanelWidth', String(mainWidth))
  }

  /** Splitter: redistributes space between main and chat, outer edges
   *  stay fixed. Typed as MouseEvent because Svelte 5's `onpointerdown`
   *  attribute type is MouseEventHandler; PointerEvent extends it at
   *  runtime. Pointer events give touch/stylus support for free. */
  function onSplitterDown(e: MouseEvent) {
    e.preventDefault()
    resizing = true
    splitterDragging = true
    const startX = e.clientX
    const startChat = chatWidth
    const startMain = mainWidth
    const target = e.currentTarget as HTMLElement | null
    const pointerId = (e as PointerEvent).pointerId
    target?.setPointerCapture?.(pointerId)

    function onMove(ev: PointerEvent) {
      moveSplitter(startMain, startChat, ev.clientX - startX)
    }
    function onUp() {
      resizing = false
      splitterDragging = false
      saveSplit()
      try { target?.releasePointerCapture?.(pointerId) } catch { /* already released */ }
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }
</script>

{#if visible}
  <div class="chat-panel" class:resizing class:hosted style={hosted ? undefined : `width: ${chatWidth}px; --chat-width: ${chatWidth}px;`} out:slideOutWidth={{ duration: 220 }}>
    {#if !hosted}
      <div
        class="chat-resize-handle"
        class:active={resizing}
        role="separator"
        aria-orientation="vertical"
        aria-label={t('splitter.card_chat')}
        aria-valuenow={Math.round(chatWidth)}
        aria-valuemin={MIN_PANEL}
        onpointerdown={onSplitterDown}
        use:keyboardResize={{ axis: 'x', onMove: (d) => moveSplitter(mainWidth, chatWidth, d), onCommit: saveSplit }}
      ></div>
    {/if}
    <!-- Inner wrapper is pinned to the final width, so children don't
         reflow while the outer .chat-panel animates from 0 → chatWidth.
         The overflow on the right is clipped by .modal's overflow:hidden. -->
    <div class="chat-panel-inner" style={hosted ? undefined : `width: ${chatWidth}px;`}>
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .chat-panel {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border-muted);
    background: var(--bg-surface);
    position: relative;
    /* Clip the fixed-width .chat-panel-inner child while the outer
       panel width animates; the chat's own scroll container inside is
       independent. */
    overflow: hidden;
    /* The width grows from 0 on mount, so the panel emerges from the
       card body's right edge. Width (layout), not transform: transforms
       create a stacking context that breaks the scroll container inside
       on WebView2. `--chat-width` mirrors the inline width so the
       keyframe needn't hard-code it; `backwards` avoids a 1-frame flash
       at full width, then hands back to the inline style so dragging
       keeps working. */
    animation: chat-panel-in 240ms cubic-bezier(0.16, 1, 0.3, 1) backwards;
  }

  @keyframes chat-panel-in {
    from {
      width: 0;
      opacity: 0;
    }
    to {
      width: var(--chat-width);
      opacity: 1;
    }
  }

  /* Pinned to the panel's natural width by an inline style, so the
     chat inside doesn't reflow on every animation frame. */
  .chat-panel-inner {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }

  .chat-panel.resizing {
    user-select: none;
  }

  /* Hosted: a SidePanel owns geometry + animations; fill it. The border
     comes from the host, and the entrance animation would double up
     with the host's. Background matches the sibling WorkspacePanel tab. */
  .chat-panel.hosted {
    width: 100%;
    height: 100%;
    border-left: none;
    animation: none;
    background: var(--bg-base);
  }
  .chat-panel.hosted .chat-panel-inner {
    width: 100%;
  }

  .chat-resize-handle {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 5px;
    z-index: 5;
    cursor: col-resize;
    transition: background 0.15s;
  }
  .chat-resize-handle:hover,
  .chat-resize-handle.active {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent-glow-1);
  }
</style>
