// Client/server version skew. A client talking to a server built from
// older code calls RPCs the server doesn't have yet, and every such
// feature fails in its own way ("Couldn't load the agent configuration").
// Builds stamp their commit's time (commit_date: /version for the
// server, GetBuildInfo for the desktop binary); comparing the two catches
// a stale deploy up front. build_date can't: an old checkout built today
// gets today's build_date.

/** What the unauthenticated GET /version reports. */
export interface ServerBuild {
  version: string
  commit_date?: string
}

/**
 * True only when both builds carry a commit date and the server's is
 * strictly older. Unstamped (dev) builds and unparseable dates never
 * warn — a false alarm would teach users to ignore the real one.
 */
export function serverIsOlder(appCommitDate: string | undefined, serverCommitDate: string | undefined): boolean {
  if (!appCommitDate || !serverCommitDate) return false
  const app = Date.parse(appCommitDate)
  const server = Date.parse(serverCommitDate)
  if (Number.isNaN(app) || Number.isNaN(server)) return false
  return server < app
}

/** Fetch a server's /version. Null when unreachable or not a BRUV server. */
export async function fetchServerBuild(baseURL: string): Promise<ServerBuild | null> {
  try {
    const res = await fetch(`${baseURL.replace(/\/+$/, '')}/version`, { cache: 'no-store' })
    if (!res.ok) return null
    const body: unknown = await res.json()
    if (!body || typeof body !== 'object') return null
    const { version, commit_date } = body as Record<string, unknown>
    if (typeof version !== 'string') return null
    return { version, commit_date: typeof commit_date === 'string' ? commit_date : undefined }
  } catch {
    return null
  }
}
