// Why the server's UNCLI chat engine can't be used, as a locale key both
// surfaces define (uncli.*) with its params, or null when it's ready.
// The settings show it beside the chat engine choice; a card opted into
// UNCLI shows it while it falls back to the built-in chat.

import type { UncliStatus } from './types'

export type UncliUnavailable = { key: string; params?: Record<string, string> }

/** status null + no error: still asking. */
export function uncliUnavailable(status: UncliStatus | null, error: string): UncliUnavailable | null {
  if (status?.ready) return null
  if (!status) return error ? { key: 'uncli.unavailable_unknown', params: { detail: error } } : { key: 'uncli.checking' }
  switch (status.reason) {
    case 'service_account':
      return { key: 'uncli.unavailable_service_account', params: { account: status.account ?? '' } }
    case 'not_hosted':
      return { key: 'uncli.unavailable_not_hosted' }
    default:
      return { key: 'uncli.unavailable_failed', params: { detail: status.detail ?? '' } }
  }
}
