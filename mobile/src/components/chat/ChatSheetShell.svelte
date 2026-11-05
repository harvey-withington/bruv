<script lang="ts">
  // The AI chat's bottom sheet: backdrop, the sheet itself, a header to
  // swipe it closed by, hardware Back closing it, and its capture-phase
  // keys, and room for the on-screen keyboard: the sheet shrinks above it
  // (fitVisualViewport) instead of the page sliding up. Whichever chat
  // engine runs (ChatSheet, UncliChatSheet) renders inside. Header
  // buttons passed as `actions` get the header's .icon-btn look.
  import { onMount, type Snippet } from 'svelte'
  import { X } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import { fitVisualViewport } from '../../lib/actions/fitVisualViewport'

  let {
    title,
    note = '',
    onClose,
    onKeydown,
    navigatingAway = () => false,
    actions,
    children,
  }: {
    title: string
    /** Faint text after the title (a message count). */
    note?: string
    onClose: () => void
    /** Keys for the sheet, on window in the capture phase so the page
     *  underneath (CardPage) never sees them. */
    onKeydown?: (e: KeyboardEvent) => void
    /** True when the sheet closes because a link navigated: that
     *  navigation consumes the sheet's history entry, so closing mustn't
     *  go back as well (the navigatedAway pattern from PromoteCardSheet). */
    navigatingAway?: () => boolean
    actions?: Snippet
    children: Snippet
  } = $props()

  // Swipe-to-dismiss: a drag down the header past 40% of the viewport
  // closes the sheet.
  let dragStartY = 0
  let dragCurrentY = 0
  let dragging = $state(false)
  let translateY = $state(0)

  function onHeaderPointerDown(e: PointerEvent) {
    // A press on a header button isn't a drag: capturing the pointer
    // would re-target its click to the header, and X would never fire.
    if ((e.target as HTMLElement | null)?.closest('button')) return
    dragStartY = e.clientY
    dragCurrentY = e.clientY
    dragging = true
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function onHeaderPointerMove(e: PointerEvent) {
    if (!dragging) return
    dragCurrentY = e.clientY
    translateY = Math.max(0, dragCurrentY - dragStartY)
  }

  function onHeaderPointerUp() {
    if (!dragging) return
    dragging = false
    const vh = window.innerHeight
    if (dragCurrentY - dragStartY > vh * 0.4) {
      translateY = vh
      // Close once the slide-out has settled.
      setTimeout(onClose, 180)
    } else {
      translateY = 0
    }
  }

  onMount(() => {
    // A synthetic history entry, so hardware Back closes the sheet
    // without leaving the page underneath. Any other close pops it.
    history.pushState({ chat: true }, '')
    const onPop = () => onClose()
    const keys = (e: KeyboardEvent) => onKeydown?.(e)
    window.addEventListener('popstate', onPop)
    window.addEventListener('keydown', keys, true)
    return () => {
      window.removeEventListener('popstate', onPop)
      window.removeEventListener('keydown', keys, true)
      if (!navigatingAway() && history.state?.chat) history.back()
    }
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}></div>

<div
  class="sheet"
  use:fitVisualViewport
  role="dialog"
  aria-label={title}
  style:transform={translateY > 0 ? `translateY(${translateY}px)` : undefined}
  style:transition={dragging ? 'none' : undefined}
>
  <div
    class="header"
    role="presentation"
    onpointerdown={onHeaderPointerDown}
    onpointermove={onHeaderPointerMove}
    onpointerup={onHeaderPointerUp}
    onpointercancel={onHeaderPointerUp}
  >
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">{title}{#if note}<span class="note">{note}</span>{/if}</span>
    {@render actions?.()}
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('chat.dismiss')}>
      <X size={18} />
    </button>
  </div>
  {@render children()}
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 200ms ease forwards;
  }

  /* Sits on the keyboard and shrinks to what's left visible, rather than
     the page sliding up under it (fitVisualViewport). Closed keyboard:
     90% of the screen as before. */
  .sheet {
    position: fixed;
    left: 0;
    right: 0;
    bottom: var(--keyboard-inset, 0px);
    height: min(90vh, calc(var(--visible-height, 100vh) - 0.75rem));
    background: var(--bg);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-up 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-bottom: env(safe-area-inset-bottom);
    transition: transform 180ms cubic-bezier(0.16, 1, 0.3, 1);
  }

  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem 0.6rem;
    border-bottom: 1px solid var(--border);
    cursor: grab;
    touch-action: none;
  }
  .header:active { cursor: grabbing; }
  .grabber {
    position: absolute;
    top: 6px;
    left: 50%;
    transform: translateX(-50%);
    width: 36px;
    height: 4px;
    border-radius: 2px;
    background: var(--text-faint);
    opacity: 0.5;
  }
  .title {
    flex: 1;
    margin-top: 0.4rem;
    font-weight: 600;
    color: var(--text);
    font-size: 0.95rem;
  }
  /* Spaced by margin, not a text space: Svelte trims an element's
     leading whitespace, which ran "Chat" and "UNCLI" together. Muted,
     not faint, so it reads on both themes. */
  .note { margin-left: 0.35em; color: var(--text-muted); font-weight: 400; }
  .header :global(.icon-btn) {
    margin-top: 0.4rem;
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 36px;
    min-height: 36px;
  }
  .header :global(.icon-btn:hover),
  .header :global(.icon-btn:focus-visible) {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes slide-up {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .backdrop,
    .sheet {
      animation: fade-in 120ms ease forwards;
    }
    .sheet { transition: none; }
  }
</style>
