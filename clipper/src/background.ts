// Background service worker — the platform-blind conductor. Owns context
// menus, receives extracted clips from the content script, runs plugin
// enrichment + media download, executes the pipeline (queueing on network
// failure), and reports the outcome back to the page as a toast.

import type { ClipExtractedMessage, ClipRequestMessage, ToastMessage } from './lib/types'
import { loadSettings, isNetworkError } from './lib/api'
import { pluginById } from './lib/plugins/registry'
import { buildJob, executeJob } from './lib/clip'
import { enqueue, drainQueue, listQueue } from './lib/queue'

const MENU_CLIP = 'bruv-clip'
const MENU_CLIP_DECK = 'bruv-clip-deck'

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    const contexts: chrome.contextMenus.ContextType[] = ['page', 'selection', 'image', 'video', 'link']
    chrome.contextMenus.create({ id: MENU_CLIP, title: chrome.i18n.getMessage('menu_clip'), contexts })
    chrome.contextMenus.create({ id: MENU_CLIP_DECK, title: chrome.i18n.getMessage('menu_clip_deck'), contexts })
  })
  chrome.alarms.create('bruv-queue', { periodInMinutes: 1 })
})

chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (!tab?.id) return
  if (info.menuItemId !== MENU_CLIP && info.menuItemId !== MENU_CLIP_DECK) return
  const msg: ClipRequestMessage = { type: 'BRUV_CLIP', includeInDeck: info.menuItemId === MENU_CLIP_DECK }
  void chrome.tabs.sendMessage(tab.id, msg)
})

function toast(tabId: number | undefined, text: string, ok: boolean): void {
  if (tabId == null) return
  const msg: ToastMessage = { type: 'BRUV_TOAST', text, ok }
  void chrome.tabs.sendMessage(tabId, msg)
}

chrome.runtime.onMessage.addListener((message: ClipExtractedMessage, sender) => {
  if (message?.type !== 'BRUV_EXTRACTED') return
  void handleExtracted(message, sender.tab?.id)
})

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
    clip = await plugin.enrich(clip)
  }

  const job = await buildJob(clip, message.includeInDeck)
  try {
    const outcome = await executeJob(settings, job)
    toast(
      tabId,
      chrome.i18n.getMessage(outcome.slideAppended ? 'toast_clipped_deck' : 'toast_clipped'),
      true,
    )
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
  if (alarm.name !== 'bruv-queue') return
  void (async () => {
    const settings = await loadSettings()
    if (!settings) return
    if ((await listQueue()).length === 0) return
    await drainQueue(settings)
  })()
})
