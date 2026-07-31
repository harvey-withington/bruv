// Popup "Pending clips" section — the completion flow's front door.
//
// Pending clips are cards the SERVER created from a shared URL but couldn't
// read (bot-walled platform): url-only, tagged 'clip-pending', slide already
// appended. Completing one hands the job to the background worker, which
// opens the URL in this (logged-in) browser and posts the capture back.
//
// The section is self-contained: it owns its DOM subtree, its per-row state
// (keyed by card ID — rows are re-rendered, indices aren't stable) and its
// own error reporting through the popup's status line.

import type { ClipperSettings, CompleteRequestMessage, CompleteResponse } from '../lib/types'
import { loadPendingCards, refreshPendingBadge, type PendingCard } from '../lib/pending'

const PENDING_LIMIT = 10

type StatusFn = (text: string, ok: boolean) => void

const $ = <T extends HTMLElement>(id: string): T => document.getElementById(id) as T
const msg = (key: string): string => chrome.i18n.getMessage(key)

type Row = { card: PendingCard; button: HTMLButtonElement; state: HTMLSpanElement }

const rows = new Map<string, Row>()

function setState(id: string, text: string, tone: '' | 'ok' | 'err'): void {
  const row = rows.get(id)
  if (!row) return
  row.state.textContent = text
  row.state.className = `state${tone ? ` ${tone}` : ''}`
}

// Opening the source tab steals focus, which closes this popup — so a
// completion's real progress lives on the toolbar badge. These inline
// states are for the runs the popup does survive to see.
async function complete(card: PendingCard, showStatus: StatusFn): Promise<void> {
  const row = rows.get(card.id)
  if (!row || !card.url) return
  row.button.disabled = true
  setState(card.id, msg('popup_pending_running'), '')

  const request: CompleteRequestMessage = { type: 'BRUV_COMPLETE', cardID: card.id, url: card.url }
  let res: CompleteResponse | undefined
  try {
    res = await chrome.runtime.sendMessage<CompleteRequestMessage, CompleteResponse>(request)
  } catch (err) {
    res = { ok: false, error: err instanceof Error ? err.message : String(err) }
  }

  if (res?.ok) {
    setState(card.id, msg('popup_pending_done'), 'ok')
    row.button.hidden = true
  } else {
    setState(card.id, msg('popup_pending_failed'), 'err')
    row.button.disabled = false
    if (res?.error) showStatus(res.error, false)
  }
  void refreshPendingBadge()
}

function renderRow(card: PendingCard, showStatus: StatusFn): HTMLLIElement {
  const li = document.createElement('li')
  li.className = 'pending-row'

  const title = document.createElement('span')
  title.className = 'title'
  title.textContent = card.title || msg('popup_untitled')
  title.title = card.url || card.title
  li.appendChild(title)

  const state = document.createElement('span')
  state.className = 'state'
  li.appendChild(state)

  const button = document.createElement('button')
  button.type = 'button'
  button.textContent = msg('popup_pending_complete')
  li.appendChild(button)

  rows.set(card.id, { card, button, state })

  // No source URL = nothing to open. Shown (disabled) rather than hidden:
  // a stuck card the user can see is one they can go fix.
  if (!card.url) {
    button.disabled = true
    setState(card.id, msg('popup_pending_no_url'), 'err')
  } else {
    button.addEventListener('click', () => void complete(card, showStatus))
  }

  return li
}

// Every completion is dispatched at once; the background worker runs them
// one at a time, so they finish even after this popup is gone.
function completeAll(showStatus: StatusFn): void {
  for (const row of rows.values()) {
    if (row.card.url && !row.button.disabled) void complete(row.card, showStatus)
  }
}

export async function renderPendingSection(settings: ClipperSettings, showStatus: StatusFn): Promise<void> {
  const section = $<HTMLDivElement>('pending')
  const list = $<HTMLUListElement>('pending-list')
  const actions = $<HTMLDivElement>('pending-actions')

  let cards: PendingCard[] = []
  try {
    cards = await loadPendingCards(settings, PENDING_LIMIT)
  } catch (err) {
    // Unlike the badge (background, silent), this ran because the user
    // opened the popup — tell them why the list is missing.
    showStatus(err instanceof Error ? err.message : String(err), false)
    section.hidden = true
    return
  }

  rows.clear()
  list.innerHTML = ''
  section.hidden = cards.length === 0
  actions.hidden = cards.filter((c) => c.url).length < 2
  for (const card of cards) list.appendChild(renderRow(card, showStatus))

  $('complete-all-btn').onclick = () => completeAll(showStatus)
}
