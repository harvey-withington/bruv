// Background service worker — the platform-blind conductor. Owns context
// menus, receives extracted clips from the content script, runs plugin
// enrichment + media download, executes the pipeline (queueing on network
// failure), and reports the outcome back to the page as a toast.
//
// It also runs the COMPLETION flow: pending clips the server couldn't read
// (bot-walled platforms) are finished here by opening the source URL in a
// real logged-in tab, capturing it with the same DOM plugins, and posting
// the result to CompleteCapture. See lib/pending.ts.

import type {
  CaptureChoices,
  CaptureDialogRequest,
  CaptureDialogResponse,
  ClipExtractedMessage,
  ClipPageRequestMessage,
  ClipPageResponse,
  ClipRequestMessage,
  ClipResult,
  ClipperSettings,
  CompleteRequestMessage,
  CompleteResponse,
  DialogAliveMessage,
  OpenOptionsMessage,
  ToastMessage,
} from './lib/types'
import { loadSettings, isNetworkError, repoRPC } from './lib/api'
import { pluginById } from './lib/plugins/registry'
import { buildJob, executeJob } from './lib/clip'
import { enqueue, drainQueue, listQueue } from './lib/queue'
import { refreshPendingBadge } from './lib/pending'
import {
  buildDialogRequest,
  defaultChoices,
  fallbackPrefs,
  loadCapturePrefs,
  previewCapture,
  shouldAsk,
} from './lib/captureOptions'

const MENU_CLIP = 'bruv-clip'
const MENU_CLIP_DECK = 'bruv-clip-deck'
// Third menu item rather than a modifier key: Chrome's context-menu click
// data carries no modifier state (Firefox's does), so a Shift+click trigger
// would be undiscoverable AND unimplementable. This one says what it does.
const MENU_CLIP_OPTIONS = 'bruv-clip-options'

const ALARM_QUEUE = 'bruv-queue'
const ALARM_PENDING = 'bruv-pending'

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    const contexts: chrome.contextMenus.ContextType[] = ['page', 'selection', 'image', 'video', 'link']
    chrome.contextMenus.create({ id: MENU_CLIP, title: chrome.i18n.getMessage('menu_clip'), contexts })
    chrome.contextMenus.create({ id: MENU_CLIP_DECK, title: chrome.i18n.getMessage('menu_clip_deck'), contexts })
    chrome.contextMenus.create({ id: MENU_CLIP_OPTIONS, title: chrome.i18n.getMessage('menu_clip_options'), contexts })
  })
  chrome.alarms.create(ALARM_QUEUE, { periodInMinutes: 1 })
  chrome.alarms.create(ALARM_PENDING, { periodInMinutes: 30 })
  void refreshPendingBadge()
})

// The badge lives on the toolbar icon, which survives service-worker
// restarts — but the badge text doesn't, so re-stamp it on every browser
// start.
chrome.runtime.onStartup.addListener(() => {
  void refreshPendingBadge()
})

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (!tab?.id) return
  const id = info.menuItemId
  if (id !== MENU_CLIP && id !== MENU_CLIP_DECK && id !== MENU_CLIP_OPTIONS) return
  const msg: ClipRequestMessage = {
    type: 'BRUV_CLIP',
    includeInDeck: id === MENU_CLIP_DECK,
    withOptions: id === MENU_CLIP_OPTIONS,
  }
  void chrome.tabs.sendMessage(tab.id, msg)
})

function toast(tabId: number | undefined, text: string, ok: boolean): void {
  if (tabId == null) return
  const msg: ToastMessage = { type: 'BRUV_TOAST', text, ok }
  void chrome.tabs.sendMessage(tabId, msg)
}

chrome.runtime.onMessage.addListener(
  (
    message: ClipExtractedMessage | CompleteRequestMessage | DialogAliveMessage | OpenOptionsMessage,
    sender,
    sendResponse: (response: CompleteResponse) => void,
  ) => {
    if (message?.type === 'BRUV_EXTRACTED') {
      void handleExtracted(message, sender.tab?.id)
      return
    }
    if (message?.type === 'BRUV_DIALOG_ALIVE') {
      // Nothing to do — receiving it is the point (it resets this worker's
      // idle timer while a capture dialog waits on the user).
      return
    }
    if (message?.type === 'BRUV_OPEN_OPTIONS') {
      void chrome.runtime.openOptionsPage()
      return
    }
    if (message?.type === 'BRUV_COMPLETE') {
      void queueCompletion(message.cardID, message.url).then(sendResponse)
      return true
    }
  },
)

// askForChoices shows the in-page dialog and waits for the answer. Returns
// null when the dialog couldn't be shown at all (no content script in that
// tab, tab closed) — the caller then falls back to the vault's defaults
// rather than dropping a capture the user asked for.
async function askForChoices(tabId: number, request: CaptureDialogRequest): Promise<CaptureDialogResponse | null> {
  try {
    return await chrome.tabs.sendMessage<CaptureDialogRequest, CaptureDialogResponse>(tabId, request)
  } catch (err) {
    console.warn('capture dialog unavailable:', err)
    return null
  }
}

// resolveChoices is the options step in front of the pipeline: read the
// vault's defaults + the server's preview of this URL, decide whether the
// decision is the user's to make, and (when it is) ask them.
//
// Returns null when the user cancelled. Undefined choices mean "behave
// exactly as the extension did before options existed" — the path taken
// when the server can't be reached for prefs at all.
async function resolveChoices(
  settings: ClipperSettings,
  clip: ClipResult,
  message: ClipExtractedMessage,
  tabId: number | undefined,
): Promise<{ choices?: CaptureChoices } | null> {
  // Explicitly asking for the dialog must show one even when the vault's
  // prefs can't be read; the silent path stays byte-for-byte as before.
  const prefs = (await loadCapturePrefs(settings)) ?? (message.withOptions ? fallbackPrefs() : null)
  if (!prefs) return {}

  // A vault set to never ask doesn't need the server's preview: it exists to
  // fill a dialog that won't open, and resolving the post server-side is the
  // one slow step in a flow whose whole appeal is one click. Such a capture
  // uses whatever ladder the plugin itself resolved.
  const wantPreview = message.withOptions || (prefs.askMode ?? 'triggers') !== 'never'
  const preview = wantPreview ? await previewCapture(settings, clip.canonicalUrl, message.withOptions) : null

  const defaults = defaultChoices(clip, preview, prefs, message.includeInDeck)
  if (!message.withOptions && !shouldAsk(clip, preview, prefs)) return { choices: defaults }
  if (tabId == null) return { choices: defaults }

  const answer = await askForChoices(tabId, buildDialogRequest(clip, preview, prefs, settings, message.includeInDeck))
  if (!answer) return { choices: defaults }
  if (!answer.ok) return null
  return { choices: answer.choices ?? defaults }
}

async function handleExtracted(message: ClipExtractedMessage, tabId: number | undefined): Promise<void> {
  const settings = await loadSettings()
  if (!settings || !settings.repoID) {
    toast(tabId, chrome.i18n.getMessage('toast_not_paired'), false)
    return
  }
  if (message.includeInDeck && !settings.deckTarget) {
    toast(tabId, chrome.i18n.getMessage('toast_no_deck'), false)
    return
  }

  let clip = message.clip
  const plugin = pluginById(clip.platform)
  if (plugin?.enrich && clip.needsEnrichment) {
    // Enrichment first: the dialog can't offer a video quality ladder for a
    // video whose URL hasn't been resolved yet.
    clip = await plugin.enrich(clip)
  }

  const resolved = await resolveChoices(settings, clip, message, tabId)
  if (!resolved) {
    toast(tabId, chrome.i18n.getMessage('dialog_cancelled'), true)
    return
  }
  const choices = resolved.choices
  const includeInDeck = choices ? choices.includeInDeck : message.includeInDeck
  if (includeInDeck && !settings.deckTarget) {
    toast(tabId, chrome.i18n.getMessage('toast_no_deck'), false)
    return
  }

  const job = await buildJob(clip, includeInDeck, choices)
  try {
    const outcome = await executeJob(settings, job)
    if (outcome.pinFailed) {
      // The clip landed but the chosen pin destination bounced (stale
      // category from another pairing, accepted-types gate) — attention
      // styling on purpose: silently landing in the Inbox is the bug.
      toast(tabId, chrome.i18n.getMessage('toast_clipped_no_pin'), false)
    } else {
      toast(
        tabId,
        chrome.i18n.getMessage(outcome.slideAppended ? 'toast_clipped_deck' : 'toast_clipped'),
        true,
      )
    }
    void refreshPendingBadge()
  } catch (err) {
    if (isNetworkError(err)) {
      await enqueue(job)
      toast(tabId, chrome.i18n.getMessage('toast_queued'), true)
    } else {
      console.error('clip failed:', err)
      toast(tabId, chrome.i18n.getMessage('toast_failed'), false)
    }
  }
}

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === ALARM_PENDING) {
    void refreshPendingBadge()
    return
  }
  if (alarm.name !== ALARM_QUEUE) return
  void (async () => {
    const settings = await loadSettings()
    if (!settings) return
    if ((await listQueue()).length === 0) return
    await drainQueue(settings)
    void refreshPendingBadge()
  })()
})

// --- completion flow -------------------------------------------------------

// A pending card's page has to actually load and (on every one of these
// platforms) hydrate client-side before the DOM plugins can see anything,
// so the runner waits for the load event and then polls the content script.
const TAB_LOAD_TIMEOUT_MS = 20_000
const POLL_ATTEMPTS = 8
const POLL_INTERVAL_MS = 700

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// Completions run ONE AT A TIME, and here rather than in the popup: opening
// the source tab takes focus, which closes the popup — so "Complete all"
// dispatches every request up front and the chain below sees them through
// even once nothing is left to await the answers.
let completionChain: Promise<unknown> = Promise.resolve()

function queueCompletion(cardID: string, url: string): Promise<CompleteResponse> {
  const run = completionChain.then(() => completePendingClip(cardID, url))
  completionChain = run.catch(() => undefined)
  return run
}

// Resolves when the tab reports status 'complete', or when the timeout
// expires — a timeout is NOT fatal, since polling below can still succeed
// on a page whose load event never settles (media, ads, long-poll sockets).
function waitForTabLoad(tabId: number): Promise<void> {
  return new Promise((resolve) => {
    let settled = false
    const finish = (): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      chrome.tabs.onUpdated.removeListener(onUpdated)
      resolve()
    }
    const onUpdated = (id: number, info: chrome.tabs.TabChangeInfo): void => {
      if (id === tabId && info.status === 'complete') finish()
    }
    const timer = setTimeout(finish, TAB_LOAD_TIMEOUT_MS)
    chrome.tabs.onUpdated.addListener(onUpdated)
    // The tab may have finished loading before the listener attached.
    chrome.tabs.get(tabId).then((tab) => {
      if (tab.status === 'complete') finish()
    }, finish)
  })
}

// SPAs render their post after the load event (and sometimes after a second
// data round-trip), so a null answer is "not yet", not "not here".
async function pollForPageClip(tabId: number): Promise<ClipResult | null> {
  const request: ClipPageRequestMessage = { type: 'BRUV_CLIP_PAGE' }
  for (let attempt = 0; attempt < POLL_ATTEMPTS; attempt++) {
    if (attempt > 0) await delay(POLL_INTERVAL_MS)
    try {
      const res = await chrome.tabs.sendMessage<ClipPageRequestMessage, ClipPageResponse>(tabId, request)
      if (res?.clip) return res.clip
    } catch {
      // Content script not injected yet (or the page is still on about:blank)
      // — keep polling.
    }
  }
  return null
}

// completePendingClip finishes ONE pending clip card. It never mutates the
// card on failure: CompleteCapture is the only write, so a failed run leaves
// the pending card exactly as the server made it, ready to retry.
async function completePendingClip(cardID: string, url: string): Promise<CompleteResponse> {
  const settings = await loadSettings()
  if (!settings || !settings.repoID) {
    return { ok: false, error: chrome.i18n.getMessage('toast_not_paired') }
  }

  let tabId: number | undefined
  try {
    // Foreground tab on purpose: browsers throttle background tabs, and
    // several of these SPAs simply don't hydrate while hidden.
    const tab = await chrome.tabs.create({ url, active: true })
    tabId = tab.id
    if (tabId == null) throw new Error(chrome.i18n.getMessage('popup_pending_error_no_tab'))

    await waitForTabLoad(tabId)
    let clip = await pollForPageClip(tabId)
    if (!clip) throw new Error(chrome.i18n.getMessage('popup_pending_error_no_clip'))

    const plugin = pluginById(clip.platform)
    if (plugin?.enrich && clip.needsEnrichment) {
      clip = await plugin.enrich(clip)
    }

    // includeInDeck is false: the pending card's slide already exists and is
    // bound to its blocks — CompleteCapture fills the blocks, never appends.
    const job = await buildJob(clip, false)
    await repoRPC(settings, 'CompleteCapture', [cardID, job.clip, job.media])
    void refreshPendingBadge()
    return { ok: true }
  } catch (err) {
    console.error('complete pending clip failed:', err)
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  } finally {
    // The tab is a tool, not a destination — always close it.
    if (tabId != null) await chrome.tabs.remove(tabId).catch(() => undefined)
  }
}
