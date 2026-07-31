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

function renderDeckTarget(): void {
  const el = $<HTMLDivElement>('deck-target')
  if (settings?.deckTarget) {
    el.textContent = settings.deckTarget.name
    el.classList.remove('muted')
  } else {
    el.textContent = msg('popup_no_deck')
    el.classList.add('muted')
  }
}

async function setDeckTarget(target: DeckTarget | null): Promise<void> {
  if (!settings) return
  settings = { ...settings, deckTarget: target }
  await saveSettings(settings)
  renderDeckTarget()
  $<HTMLUListElement>('deck-results').innerHTML = ''
  $<HTMLInputElement>('deck-search').value = ''
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

function renderResults(results: SearchResult[]): void {
  const list = $<HTMLUListElement>('deck-results')
  list.innerHTML = ''
  for (const r of results) {
    const li = document.createElement('li')
    const btn = document.createElement('button')
    btn.type = 'button'
    const title = document.createElement('span')
    title.textContent = r.Title || msg('popup_untitled')
    btn.appendChild(title)
    if (r.ProjectContext) {
      const path = document.createElement('span')
      path.className = 'path'
      path.textContent = r.ProjectContext
      btn.appendChild(path)
    }
    btn.addEventListener('click', () => void pickCardAsDeck(r.CardID, r.Title || msg('popup_untitled')))
    li.appendChild(btn)
    list.appendChild(li)
  }
}

let searchSeq = 0
async function runSearch(query: string): Promise<void> {
  if (!settings) return
  const seq = ++searchSeq
  try {
    const results = query.trim()
      ? await repoRPC<SearchResult[]>(settings, 'SearchCards', [query.trim(), 8])
      : await repoRPC<SearchResult[]>(settings, 'RecentCards', [8])
    if (seq !== searchSeq) return
    renderResults(results ?? [])
  } catch {
    if (seq === searchSeq) renderResults([])
  }
}

let debounce: number | undefined
$('deck-search').addEventListener('input', (e) => {
  const q = (e.target as HTMLInputElement).value
  clearTimeout(debounce)
  debounce = setTimeout(() => void runSearch(q), 250) as unknown as number
})
$('deck-search').addEventListener('focus', () => void runSearch($<HTMLInputElement>('deck-search').value))

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

$('clear-deck-btn').addEventListener('click', () => void setDeckTarget(null))

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

void (async () => {
  settings = await loadSettings()
  const paired = settings !== null && settings.repoID !== ''
  $('paired').hidden = !paired
  $('unpaired').hidden = paired
  if (!paired) {
    $('unpaired').textContent = msg('popup_not_paired')
    const link = document.createElement('a')
    link.href = '#'
    link.textContent = ` ${msg('popup_open_options')}`
    link.style.color = '#b7a8f0'
    link.addEventListener('click', (e) => {
      e.preventDefault()
      void chrome.runtime.openOptionsPage()
    })
    $('unpaired').appendChild(link)
    return
  }
  $<HTMLDivElement>('pair-line').textContent = `${settings!.repoName || settings!.repoID} @ ${settings!.serverURL}`
  $<HTMLInputElement>('deck-search').placeholder = msg('popup_search_placeholder')
  renderDeckTarget()
  await refreshQueue()
  await renderPendingSection(settings!, showStatus)
  void refreshPendingBadge()
})()
