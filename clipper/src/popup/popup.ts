// Popup: shows pairing status, manages the STICKY DECK TARGET (the thing
// that makes clip #2 onward one click), surfaces the offline queue, and
// lists PENDING CLIPS waiting for this browser to complete them (see
// popup/pendingSection.ts).
// Deck picking: search cards, pick one, first slide_deck block wins (multi-
// deck cards are rare; the options page story can grow later). "New deck"
// creates a card with an empty deck block in one step.

import { loadSettings, repoRPC, saveSettings } from '../lib/api'
import { drainQueue, clearQueue, listQueue } from '../lib/queue'
import { refreshPendingBadge } from '../lib/pending'
import { typeahead } from '../lib/typeahead'
import { renderPendingSection } from './pendingSection'
import type { ClipperSettings, DeckTarget } from '../lib/types'

const $ = <T extends HTMLElement>(id: string): T => document.getElementById(id) as T
const msg = (key: string): string => chrome.i18n.getMessage(key)

for (const el of Array.from(document.querySelectorAll<HTMLElement>('[data-i18n]'))) {
  el.textContent = msg(el.dataset.i18n as string)
}

const statusEl = $<HTMLDivElement>('status')
function showStatus(text: string, ok: boolean): void {
  statusEl.textContent = text
  statusEl.className = ok ? 'ok' : 'err'
}

type SearchResult = { CardID: string; Title: string; ProjectContext?: string }
type CardBlocks = { id: string; blocks?: Array<{ id: string; type: string; label: string }> }

let settings: ClipperSettings | null = null

async function setDeckTarget(target: DeckTarget | null): Promise<void> {
  if (!settings) return
  settings = { ...settings, deckTarget: target }
  await saveSettings(settings)
  deckPicker.setValue(target?.name ?? '')
}

async function pickCardAsDeck(cardID: string, title: string): Promise<void> {
  if (!settings) return
  try {
    const card = await repoRPC<CardBlocks>(settings, 'GetCard', [cardID])
    const deck = (card.blocks ?? []).find((b) => b.type === 'slide_deck')
    if (!deck) {
      showStatus(msg('popup_card_has_no_deck'), false)
      return
    }
    await setDeckTarget({ cardID, blockID: deck.id, name: `${title}${deck.label ? ` › ${deck.label}` : ''}` })
    showStatus(msg('popup_deck_set'), true)
  } catch (err) {
    showStatus(err instanceof Error ? err.message : String(err), false)
  }
}

// One combobox IS the deck target: its value at rest is the current
// selection, typing searches, the inline × clears it. Recents on focus
// (empty query) per the picker rules.
const deckPicker = typeahead({
  input: $<HTMLInputElement>('deck-search'),
  list: $<HTMLUListElement>('deck-results'),
  clearBtn: $<HTMLButtonElement>('deck-clear'),
  emptyLabel: msg('popup_no_results'),
  entries: async (query) => {
    if (!settings) return []
    try {
      const q = query.trim()
      const results =
        (q
          ? await repoRPC<SearchResult[]>(settings, 'SearchCards', [q, 8])
          : await repoRPC<SearchResult[]>(settings, 'RecentCards', [8])) ?? []
      return results.map((r) => ({
        id: r.CardID,
        label: r.Title || msg('popup_untitled'),
        sub: r.ProjectContext,
      }))
    } catch (err) {
      showStatus(err instanceof Error ? err.message : String(err), false)
      return []
    }
  },
  onSelect: (entry) => pickCardAsDeck(entry.id, entry.label),
  onClear: () => setDeckTarget(null),
})

// New-deck flow: inline name row, no native prompt (project convention —
// native dialogs are banned everywhere users see BRUV).
$('new-deck-btn').addEventListener('click', () => {
  const row = $<HTMLDivElement>('new-deck-row')
  row.hidden = !row.hidden
  if (!row.hidden) $<HTMLInputElement>('new-deck-name').focus()
})
$('new-deck-cancel').addEventListener('click', () => {
  $<HTMLDivElement>('new-deck-row').hidden = true
  $<HTMLInputElement>('new-deck-name').value = ''
})
$('new-deck-create').addEventListener('click', () => {
  void (async () => {
    if (!settings) return
    const name = $<HTMLInputElement>('new-deck-name').value.trim()
    if (!name) return
    try {
      const card = await repoRPC<{ id: string }>(settings, 'CreateCard', ['', name])
      const blockID = `blk-${crypto.randomUUID().slice(0, 8)}`
      await repoRPC(settings, 'UpdateCardBlocks', [
        card.id,
        [{ id: blockID, type: 'slide_deck', label: 'Slides', key: '', value: { slides: [] } }],
      ])
      await setDeckTarget({ cardID: card.id, blockID, name })
      $<HTMLDivElement>('new-deck-row').hidden = true
      $<HTMLInputElement>('new-deck-name').value = ''
      showStatus(msg('popup_deck_created'), true)
    } catch (err) {
      showStatus(err instanceof Error ? err.message : String(err), false)
    }
  })()
})

async function refreshQueue(): Promise<void> {
  const jobs = await listQueue()
  const line = $<HTMLSpanElement>('queue-line')
  const actions = $<HTMLDivElement>('queue-actions')
  if (jobs.length === 0) {
    line.textContent = msg('popup_queue_empty')
    actions.hidden = true
    return
  }
  const failed = jobs.filter((j) => j.lastError).length
  line.textContent =
    msg('popup_queue_count').replace('{n}', String(jobs.length)) +
    (failed > 0 ? ` — ${msg('popup_queue_failed').replace('{n}', String(failed))}` : '')
  actions.hidden = false
}

$('retry-btn').addEventListener('click', () => {
  void (async () => {
    if (!settings) return
    const btn = $<HTMLButtonElement>('retry-btn')
    btn.disabled = true
    try {
      const res = await drainQueue(settings)
      showStatus(msg('popup_retry_done').replace('{n}', String(res.done)), true)
      void refreshPendingBadge()
    } finally {
      btn.disabled = false
      await refreshQueue()
    }
  })()
})

// Two-step destructive confirm (no native confirm() — project convention):
// first click arms the button for 3 seconds, second click discards.
let discardArmed = false
let discardTimer: number | undefined
$('discard-btn').addEventListener('click', () => {
  const btn = $<HTMLButtonElement>('discard-btn')
  if (!discardArmed) {
    discardArmed = true
    btn.textContent = msg('popup_discard_confirm')
    clearTimeout(discardTimer)
    discardTimer = setTimeout(() => {
      discardArmed = false
      btn.textContent = msg('popup_discard')
    }, 3000) as unknown as number
    return
  }
  void (async () => {
    clearTimeout(discardTimer)
    discardArmed = false
    btn.textContent = msg('popup_discard')
    await clearQueue()
    await refreshQueue()
  })()
})

// Options is reachable from the header at all times (Chrome's own route
// to it is buried); the unpaired state just says what's wrong.
$('options-btn').addEventListener('click', () => {
  void chrome.runtime.openOptionsPage()
})

// --- server availability ----------------------------------------------
//
// Everything below the pair line needs the server: picking a deck target
// searches cards, the pending list fetches them. With the server down
// those controls used to sit there looking usable and fail one by one on
// click. Probe first, and when it's unreachable make the dead parts LOOK
// dead — greyed, click-blocked, with one banner saying why.

const REACH_TIMEOUT_MS = 2500

async function serverReachable(s: ClipperSettings): Promise<boolean> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), REACH_TIMEOUT_MS)
  try {
    // /version is unauthenticated — this asks "is a server there?", not
    // "is my token good", so an expired pairing still reports online and
    // fails with a real auth error where the user can act on it.
    const res = await fetch(`${s.serverURL}/version`, { signal: ctrl.signal })
    return res.ok
  } catch {
    return false
  } finally {
    clearTimeout(timer)
  }
}

function setServerAvailable(available: boolean, serverURL: string): void {
  $('offline').hidden = available
  const deck = $('deck-section')
  deck.classList.toggle('unavailable', !available)
  // `inert` as well as the CSS: pointer-events alone still lets Tab
  // reach the search box, so the section would be keyboard-usable while
  // looking dead.
  deck.inert = !available
  // The queue COUNT stays readable offline (knowing clips are waiting is
  // exactly what you want to see) — only the server-dependent Retry is
  // disabled. Discard is local storage, so it keeps working.
  $<HTMLButtonElement>('retry-btn').disabled = !available
  if (!available) {
    $<HTMLDivElement>('offline-text').textContent = msg('popup_offline').replace('{url}', serverURL)
  }
}

async function init(): Promise<void> {
  settings = await loadSettings()
  const paired = settings !== null && settings.repoID !== ''
  $('paired').hidden = !paired
  $('unpaired').hidden = paired
  if (!paired) {
    $('unpaired').textContent = msg('popup_not_paired')
    return
  }
  $<HTMLDivElement>('pair-line').textContent = `${settings!.repoName || settings!.repoID} @ ${settings!.serverURL}`
  $<HTMLInputElement>('deck-search').placeholder = msg('popup_no_deck')
  deckPicker.setValue(settings!.deckTarget?.name ?? '')
  // Queue is local storage — always meaningful, online or not.
  await refreshQueue()

  const online = await serverReachable(settings!)
  setServerAvailable(online, settings!.serverURL)
  if (!online) return

  await renderPendingSection(settings!, showStatus)
  void refreshPendingBadge()
}

$('offline-retry').addEventListener('click', () => {
  void (async () => {
    const btn = $<HTMLButtonElement>('offline-retry')
    btn.disabled = true
    showStatus('', true)
    try {
      await init()
    } finally {
      btn.disabled = false
    }
  })()
})

void init()
