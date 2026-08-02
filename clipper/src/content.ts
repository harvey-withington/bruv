// Content script — runs on every page (registry gates what's capturable).
// Remembers the last right-clicked element, resolves it to the platform's
// capture unit via the plugin, extracts a ClipResult, and hands it to the
// background worker. Also renders the in-page toast.

import type {
  CaptureDialogRequest,
  CaptureDialogResponse,
  ClipExtractedMessage,
  ClipPageRequestMessage,
  ClipPageResponse,
  ClipRequestMessage,
  DialogAliveMessage,
  OpenOptionsMessage,
  ToastMessage,
} from './lib/types'
import { pluginForUrl } from './lib/plugins/registry'
import { showCaptureDialog } from './lib/captureDialog'

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
  const out: ClipExtractedMessage = {
    type: 'BRUV_EXTRACTED',
    clip,
    includeInDeck: msg.includeInDeck,
    withOptions: msg.withOptions,
  }
  void chrome.runtime.sendMessage(out)
}

// The worker asks for the user's capture choices. It waits on this answer,
// so ping it while the dialog is open: an inbound message resets the
// service worker's idle timer, which would otherwise expire mid-decision.
const KEEPALIVE_MS = 20_000

async function handleOptionsRequest(request: CaptureDialogRequest): Promise<CaptureDialogResponse> {
  // The "Clipping…" toast is now a lie — the dialog is the status.
  document.querySelector('.bruv-clip-toast')?.remove()
  const ping = setInterval(() => {
    const alive: DialogAliveMessage = { type: 'BRUV_DIALOG_ALIVE' }
    void chrome.runtime.sendMessage<DialogAliveMessage, void>(alive).catch(() => undefined)
  }, KEEPALIVE_MS)
  try {
    const outcome = await showCaptureDialog(request)
    if (outcome.openOptions) {
      // Only the worker can open the options page.
      const open: OpenOptionsMessage = { type: 'BRUV_OPEN_OPTIONS' }
      void chrome.runtime.sendMessage<OpenOptionsMessage, void>(open).catch(() => undefined)
    }
    if (outcome.ok) showToast(chrome.i18n.getMessage('toast_clipping'), true)
    return { ok: outcome.ok, choices: outcome.choices }
  } finally {
    clearInterval(ping)
  }
}

// Completion capture: no click target, no toast. The background worker
// opened this tab from a pending clip card and will close it again the
// moment we answer, so the page is never a place the user is looking at —
// failures are reported in the popup, not here.
function handleClipPageRequest(): ClipPageResponse {
  const plugin = pluginForUrl(location.href)
  const unit = plugin?.resolvePageUnit?.(document) ?? null
  if (!unit || !plugin) return { clip: null }
  return { clip: plugin.extract(unit, document) }
}

chrome.runtime.onMessage.addListener(
  (
    message: ClipRequestMessage | ToastMessage | ClipPageRequestMessage | CaptureDialogRequest,
    _sender,
    sendResponse: (response: ClipPageResponse | CaptureDialogResponse) => void,
  ) => {
    if (message?.type === 'BRUV_CLIP') handleClipRequest(message)
    else if (message?.type === 'BRUV_TOAST') showToast(message.text, message.ok)
    else if (message?.type === 'BRUV_CLIP_PAGE') {
      sendResponse(handleClipPageRequest())
      return true
    } else if (message?.type === 'BRUV_OPTIONS') {
      void handleOptionsRequest(message).then(sendResponse)
      return true
    }
  },
)
