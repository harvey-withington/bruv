// Mobile-side SSE client. Subscribes to /repos/<id>/events on the
// enrolled server and dispatches named events to listeners. Mirrors
// the desktop's EventStream behaviour but stays mobile-only and
// lighter — no buffering, no resume cursor, no heartbeat watchdog
// beyond what EventSource already gives us.
//
// EventSource handles reconnection automatically with exponential
// backoff baked into the browser. The server emits a `: heartbeat`
// comment every 15s so transport failures surface as `error` events
// rather than silent stalls.
//
// Token-on-querystring is mandatory because EventSource can't set
// custom headers. Same shape the cloud adapter uses on desktop.

import { readEnrolment, readActiveRepoID } from './auth'
import { KNOWN_TOPICS, type KnownTopic } from '@shared/adapters/topics'

// Topics come from the single shared list — a hand-maintained mobile
// copy silently drifted (the workspace:* topics were missing, so those
// events were published but never delivered here).
export type BackendEventTopic = KnownTopic

export type BackendEvent = {
  topic: BackendEventTopic
  payload: Record<string, unknown>
}

export type EventListener = (event: BackendEvent) => void

let source: EventSource | null = null
let activeRepoID: string | null = null
const listeners = new Set<EventListener>()

function dispatch(topic: BackendEventTopic, raw: string): void {
  let payload: Record<string, unknown> = {}
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') payload = parsed as Record<string, unknown>
  } catch {
    // Some events have non-JSON payloads (or empty); pass through as
    // an empty payload rather than dropping the topic.
    payload = {}
  }
  const ev: BackendEvent = { topic, payload }
  for (const cb of listeners) {
    try {
      cb(ev)
    } catch (err) {
      console.error(`[bruv events] listener threw for ${topic}:`, err)
    }
  }
}

function attach(repoID: string): void {
  const enrol = readEnrolment()
  if (!enrol) return
  const url = `${enrol.serverURL}/repos/${encodeURIComponent(repoID)}/events?token=${encodeURIComponent(enrol.deviceToken)}`
  const src = new EventSource(url)
  source = src
  activeRepoID = repoID

  for (const topic of KNOWN_TOPICS) {
    src.addEventListener(topic, (ev) => dispatch(topic, (ev as MessageEvent).data))
  }

  src.onerror = () => {
    // EventSource reconnects on its own; surface for diagnostics.
    console.warn('[bruv events] SSE connection error — EventSource will retry')
  }
}

function detach(): void {
  source?.close()
  source = null
  activeRepoID = null
}

/**
 * Start the SSE connection against the currently-active repo. Called
 * from App.svelte once enrolment + repo selection are present. Idempotent
 * — calling start() twice with the same repo is a no-op; calling with a
 * different repo restarts the stream against the new one.
 */
export function startEvents(): void {
  const repoID = readActiveRepoID()
  if (!repoID) return
  if (source && activeRepoID === repoID) return
  detach()
  attach(repoID)
}

/**
 * Tear down the SSE connection. Used when the user un-pairs / clears
 * enrolment, or when switching repos before re-attaching.
 */
export function stopEvents(): void {
  detach()
}

/**
 * Subscribe to all topics. Returns an unsubscribe function. Listeners
 * filter by topic themselves — keeps the API simple, avoids per-topic
 * subscription bookkeeping for the handful of topics each page cares
 * about.
 */
export function onEvent(listener: EventListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
