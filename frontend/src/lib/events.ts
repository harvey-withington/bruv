// Event subscription facade. Components use `onEvent(topic, cb)` to
// receive domain events. The facade routes through whichever adapter
// is active — Wails IPC in desktop, SSE in cloud mode, trivially
// stubbed in the mock. Components don't know or care which.
//
// Return value is an unsubscribe function (same shape as Wails'
// EventsOn) so the migration from `EventsOn(topic, cb)` to
// `onEvent(topic, cb)` is a rename at every call site.

import { getBackend } from '@shared/adapters'
import type { BackendEvent, EventCallback } from '@shared/types'

/**
 * Subscribe to a single event topic. Returns an unsubscribe function.
 *
 * Matches the ergonomics of wailsjs's EventsOn so existing call sites
 * port with a straight rename. Under the hood it attaches a callback
 * to the adapter's event stream and filters by topic before
 * delegating to the user's handler.
 */
export function onEvent<T = unknown>(topic: string, handler: (data: T) => void): () => void {
  const adapter = getBackend()
  const filter: EventCallback = (ev: BackendEvent) => {
    // BackendEvent carries the topic in `type`; the topic-specific
    // payload fields live alongside it. Callers narrow to T.
    if (ev.type === topic) {
      handler(ev as unknown as T)
    }
  }
  adapter.subscribe(filter)
  return () => adapter.unsubscribe(filter)
}
