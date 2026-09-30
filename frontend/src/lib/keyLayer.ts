// keyLayer — one ordered stack of keyboard layers (UI-CONVENTIONS §8.1).
//
// Every overlay that sits ABOVE a closable container (ConfirmDialog, card
// dropdowns, pickers, previews, the slide editor…) pushes a layer while it
// is mounted. A single CAPTURE-phase window listener routes Escape and
// Enter to the TOPMOST layer only, and a handled key is consumed
// (preventDefault + stopPropagation) — so the card dialog's bubble-phase
// `<svelte:window onkeydown>` never sees it and can't close (or, on a fresh
// card, delete) itself underneath the overlay.
//
// Why a stack instead of per-overlay capture listeners: several capture
// listeners on `window` all fire (stopPropagation can't stop siblings on
// the same node), in REGISTRATION order — the lowest layer first. The stack
// makes "topmost owns the key" explicit and order-proof.
//
//   Escape             → top layer's onEscape (always consumed)
//   Ctrl/Cmd+Enter     → top layer's onEnter if it has one, else swallowed
//                        (the chord must never reach the card underneath:
//                        it would commit-all + close it)
//   Enter              → top layer's onEnter if it has one, else untouched
//                        (default activation / the focused field's own
//                        handler keeps working)
//
// A layer can decline a key (`enabled: false`, or onEscape/onEnter
// returning false) — it then falls through to the next layer down, and
// past the stack to the normal bubble-phase listeners.

export interface KeyLayerParams {
  /** Escape pressed while this is the top layer. Return false to decline. */
  onEscape: () => void | boolean
  /**
   * Enter (plain or with Ctrl/Cmd) while this is the top layer. Return
   * false to decline. Omit to leave plain Enter alone and swallow the
   * Ctrl/Cmd+Enter chord.
   */
  onEnter?: (e: KeyboardEvent) => void | boolean
  /** Temporarily stop owning keys without unmounting (default true). */
  enabled?: boolean
}

interface Layer {
  params: KeyLayerParams
}

const layers: Layer[] = []

function consume(e: KeyboardEvent): void {
  e.preventDefault()
  e.stopPropagation()
}

function handleLayer(layer: Layer, e: KeyboardEvent): boolean {
  const { params } = layer
  if (params.enabled === false) return false
  if (e.key === 'Escape') {
    if (params.onEscape() === false) return false
    consume(e)
    return true
  }
  if (e.key === 'Enter') {
    const chord = e.ctrlKey || e.metaKey
    if (params.onEnter) {
      if (params.onEnter(e) === false) return false
      consume(e)
      return true
    }
    if (chord) {
      consume(e)
      return true
    }
  }
  return false
}

function onWindowKeydownCapture(e: KeyboardEvent): void {
  if (e.isComposing) return // IME candidate selection — not ours
  if (e.key !== 'Escape' && e.key !== 'Enter') return
  for (let i = layers.length - 1; i >= 0; i--) {
    if (handleLayer(layers[i], e)) return
  }
}

/** Push a layer; returns the function that removes it. */
export function pushKeyLayer(params: KeyLayerParams): { update(p: KeyLayerParams): void; remove(): void } {
  const layer: Layer = { params }
  if (layers.length === 0) window.addEventListener('keydown', onWindowKeydownCapture, true)
  layers.push(layer)
  return {
    update(p) { layer.params = p },
    remove() {
      const idx = layers.indexOf(layer)
      if (idx === -1) return
      layers.splice(idx, 1)
      if (layers.length === 0) window.removeEventListener('keydown', onWindowKeydownCapture, true)
    },
  }
}

/**
 * Svelte action form: the layer lives exactly as long as the element.
 * Attach it to the overlay/dropdown that only mounts while open.
 *
 *   {#if open}<div class="dropdown-menu" use:keyLayer={{ onEscape: () => open = false }}>…{/if}
 */
export function keyLayer(_node: HTMLElement, params: KeyLayerParams) {
  const handle = pushKeyLayer(params)
  return {
    update(p: KeyLayerParams) { handle.update(p) },
    destroy() { handle.remove() },
  }
}

/** True while any overlay layer is open (global shortcuts stand down). */
export function hasKeyLayer(): boolean {
  return layers.some(l => l.params.enabled !== false)
}

// Modal containers (the card dialog) aren't layers — they handle their own
// keys bubble-phase — but while one is open, single-letter global shortcuts
// (`p`, `w`, `/`, `?`) must not fire behind it.
let openModals = 0

/** Svelte action: counts the element as an open modal while mounted. */
export function modalOpen(_node: HTMLElement) {
  openModals++
  return { destroy() { openModals = Math.max(0, openModals - 1) } }
}

/** Global single-key shortcuts should be ignored right now. */
export function globalShortcutsBlocked(): boolean {
  return openModals > 0 || hasKeyLayer()
}
