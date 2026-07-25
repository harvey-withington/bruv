// Content script — runs on every page (registry gates what's capturable).
// Remembers the last right-clicked element, resolves it to the platform's
// capture unit via the plugin, extracts a ClipResult, and hands it to the
// background worker. Also renders the in-page toast.

import type { ClipExtractedMessage, ClipRequestMessage, ToastMessage } from './lib/types'
import { pluginForUrl } from './lib/plugins/registry'

let lastContextTarget: Element | null = null

document.addEventListener(
  'contextmenu',
  (e) => {
    lastContextTarget = e.target instanceof Element ? e.target : null
  },
  { capture: true, passive: true },
)

function showToast(text: string, ok: boolean): void {
  document.querySelector('.bruv-clip-toast')?.remove()
  const el = document.createElement('div')
  el.className = `bruv-clip-toast ${ok ? 'bruv-ok' : 'bruv-err'}`
  el.textContent = text
  document.documentElement.appendChild(el)
  setTimeout(() => el.remove(), 3200)
}

function handleClipRequest(msg: ClipRequestMessage): void {
  const plugin = pluginForUrl(location.href)
  if (!plugin) {
    showToast(chrome.i18n.getMessage('toast_no_plugin'), false)
    return
  }

  // Prefer the right-clicked element; fall back to the selection's anchor so
  // "highlight then clip" works even when the click landed elsewhere.
  const selection = window.getSelection()
  const selectionEl =
    selection && selection.rangeCount > 0 && selection.anchorNode
      ? selection.anchorNode instanceof Element
        ? selection.anchorNode
        : selection.anchorNode.parentElement
      : null
  const start = lastContextTarget ?? selectionEl
  const unit = start ? plugin.resolveCaptureUnit(start, document) : null
  if (!unit) {
    showToast(chrome.i18n.getMessage('toast_no_unit'), false)
    return
  }

  const clip = plugin.extract(unit, document)
  if (!clip) {
    showToast(chrome.i18n.getMessage('toast_extract_failed'), false)
    return
  }

  // A highlighted passage inside the capture unit overrides the full text —
  // clipping half a long post is a deliberate act.
  const selText = selection?.toString().trim()
  if (selText && unit.contains(selectionEl)) {
    clip.text = selText
  }

  showToast(chrome.i18n.getMessage('toast_clipping'), true)
  const out: ClipExtractedMessage = { type: 'BRUV_EXTRACTED', clip, includeInDeck: msg.includeInDeck }
  void chrome.runtime.sendMessage(out)
}

chrome.runtime.onMessage.addListener((message: ClipRequestMessage | ToastMessage) => {
  if (message?.type === 'BRUV_CLIP') handleClipRequest(message)
  else if (message?.type === 'BRUV_TOAST') showToast(message.text, message.ok)
})
