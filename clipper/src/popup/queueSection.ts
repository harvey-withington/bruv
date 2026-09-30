// Popup "queued clips" section — the offline queue's face.
//
// Lists EVERY queued clip with its state and last error (nothing in the
// queue is ever dropped silently, so this list is where a stuck clip is
// seen and dealt with). The popup never touches queue storage itself: Retry
// and Discard are requests to the background worker, the queue's only
// writer and drainer. The list re-renders from storage changes, so it
// follows a drain the worker runs while the popup is open.

import type { QueueDiscardMessage, QueueDrainMessage, QueueEntry, QueueResponse } from '../lib/types'
import { isQueueStorageKey, listQueue } from '../lib/queue'
import { refreshPendingBadge } from '../lib/pending'
import { armButton } from './armButton'

type StatusFn = (text: string, ok: boolean) => void

const $ = <T extends HTMLElement>(id: string): T => document.getElementById(id) as T
const msg = (key: string): string => chrome.i18n.getMessage(key)

async function send(message: QueueDrainMessage | QueueDiscardMessage): Promise<QueueResponse> {
  try {
    return await chrome.runtime.sendMessage<QueueDrainMessage | QueueDiscardMessage, QueueResponse>(message)
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}

function stateText(entry: QueueEntry, now: number): { text: string; tone: '' | 'err' } {
  if ((entry.leaseUntil ?? 0) > now) return { text: msg('popup_queue_state_sending'), tone: '' }
  // The card already exists; the retry finishes it rather than making another.
  if (entry.progress.cardID) return { text: msg('popup_queue_state_partial'), tone: entry.lastError ? 'err' : '' }
  if (entry.lastError) return { text: msg('popup_queue_state_failed'), tone: 'err' }
  return { text: msg('popup_queue_state_waiting'), tone: '' }
}

function detailText(entry: QueueEntry, now: number): string {
  const parts: string[] = []
  if (entry.lastError) parts.push(entry.lastError)
  if ((entry.leaseUntil ?? 0) <= now && entry.nextAttemptAt > now) {
    const minutes = Math.max(1, Math.ceil((entry.nextAttemptAt - now) / 60_000))
    parts.push(msg('popup_queue_next_try').replace('{n}', String(minutes)))
  }
  return parts.join(' · ')
}

export class QueueSection {
  private retryAvailable = true

  constructor(private readonly showStatus: StatusFn) {
    $('retry-btn').addEventListener('click', () => void this.retry())
    armButton($<HTMLButtonElement>('discard-btn'), {
      idleText: msg('popup_discard'),
      armedText: msg('popup_discard_confirm'),
      onConfirm: () => void this.discard(undefined),
    })
    chrome.storage.onChanged.addListener((changes, area) => {
      if (area === 'local' && Object.keys(changes).some(isQueueStorageKey)) void this.refresh()
    })
  }

  // The queue is local storage — always readable. Only Retry needs the
  // server; Discard keeps working offline.
  setServerAvailable(available: boolean): void {
    this.retryAvailable = available
    $<HTMLButtonElement>('retry-btn').disabled = !available
  }

  async refresh(): Promise<void> {
    const entries = await listQueue()
    const line = $<HTMLSpanElement>('queue-line')
    const list = $<HTMLUListElement>('queue-list')
    $<HTMLDivElement>('queue-actions').hidden = entries.length === 0
    list.replaceChildren()
    if (entries.length === 0) {
      line.textContent = msg('popup_queue_empty')
      return
    }
    const failed = entries.filter((e) => e.lastError).length
    line.textContent =
      msg('popup_queue_count').replace('{n}', String(entries.length)) +
      (failed > 0 ? ` — ${msg('popup_queue_failed').replace('{n}', String(failed))}` : '')
    const now = Date.now()
    for (const entry of entries) list.appendChild(this.renderRow(entry, now))
  }

  private renderRow(entry: QueueEntry, now: number): HTMLLIElement {
    const li = document.createElement('li')
    li.className = 'pending-row queue-row'

    const title = document.createElement('span')
    title.className = 'title'
    title.textContent = entry.label || msg('popup_untitled')
    title.title = entry.label
    li.appendChild(title)

    const { text, tone } = stateText(entry, now)
    const state = document.createElement('span')
    state.className = `state${tone ? ` ${tone}` : ''}`
    state.textContent = text
    li.appendChild(state)

    const del = document.createElement('button')
    del.type = 'button'
    del.className = 'danger'
    li.appendChild(del)
    armButton(del, {
      idleText: '×',
      idleTitle: msg('popup_queue_discard'),
      armedText: msg('popup_queue_discard_confirm'),
      onConfirm: () => void this.discard([entry.id]),
    })

    const detail = detailText(entry, now)
    if (detail) {
      const d = document.createElement('div')
      d.className = `detail${entry.lastError ? ' err' : ''}`
      d.textContent = detail
      li.appendChild(d)
    }
    return li
  }

  private async retry(): Promise<void> {
    const btn = $<HTMLButtonElement>('retry-btn')
    btn.disabled = true
    const res = await send({ type: 'BRUV_QUEUE_DRAIN' })
    if (res.ok && res.result) {
      this.showStatus(msg('popup_retry_done').replace('{n}', String(res.result.done)), true)
      void refreshPendingBadge()
    } else {
      this.showStatus(res.error ?? msg('popup_queue_retry_failed'), false)
    }
    btn.disabled = !this.retryAvailable
    await this.refresh()
  }

  private async discard(jobIDs: string[] | undefined): Promise<void> {
    const res = await send({ type: 'BRUV_QUEUE_DISCARD', jobIDs })
    if (!res.ok) this.showStatus(res.error ?? msg('popup_queue_discard_failed'), false)
    await this.refresh()
  }
}
