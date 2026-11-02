// A fetch that gives up after `ms`. Reachability probes need one: a phone
// whose tunnel is up but whose server is gone otherwise waits on the
// browser's own socket timeout, which is minutes, and no "can't reach the
// server" state ever shows. Uses an AbortController rather than
// AbortSignal.timeout so older iOS Safari (before 16) works too.

/** Fetch `url`, aborting after `ms`. Rejects on timeout like any network failure. */
export async function fetchWithin(url: string, ms: number, init: RequestInit = {}): Promise<Response> {
  const ctl = new AbortController()
  const timer = setTimeout(() => ctl.abort(), ms)
  try {
    return await fetch(url, { ...init, signal: ctl.signal })
  } finally {
    clearTimeout(timer)
  }
}

/** How long a /version probe may take before the server counts as unreachable. */
export const PROBE_TIMEOUT_MS = 3000
