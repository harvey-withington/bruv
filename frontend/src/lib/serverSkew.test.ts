import { describe, it, expect, vi, afterEach } from 'vitest'
import { serverIsOlder, fetchServerBuild } from '@shared/serverSkew'

describe('serverIsOlder', () => {
  it('flags a server built from an older commit', () => {
    expect(serverIsOlder('2026-10-16T05:00:00+07:00', '2026-09-27T01:13:17+07:00')).toBe(true)
  })

  it('compares instants, not strings, across time zones', () => {
    // Same instant written in two zones — not older.
    expect(serverIsOlder('2026-10-16T05:00:00+07:00', '2026-10-15T22:00:00Z')).toBe(false)
    // One second earlier in UTC — older.
    expect(serverIsOlder('2026-10-16T05:00:00+07:00', '2026-10-15T21:59:59Z')).toBe(true)
  })

  it('stays quiet for a same-commit or newer server', () => {
    expect(serverIsOlder('2026-10-16T05:00:00Z', '2026-10-16T05:00:00Z')).toBe(false)
    expect(serverIsOlder('2026-10-01T00:00:00Z', '2026-10-16T05:00:00Z')).toBe(false)
  })

  it('never warns when either side is unstamped or unparseable', () => {
    expect(serverIsOlder('', '2026-09-27T01:13:17Z')).toBe(false)
    expect(serverIsOlder('2026-10-16T05:00:00Z', undefined)).toBe(false)
    expect(serverIsOlder('2026-10-16T05:00:00Z', 'not a date')).toBe(false)
  })
})

describe('fetchServerBuild', () => {
  afterEach(() => vi.unstubAllGlobals())

  function stubFetch(impl: () => Promise<Response>) {
    const fn = vi.fn(impl)
    vi.stubGlobal('fetch', fn)
    return fn
  }

  it('reads version and commit_date from /version', async () => {
    const fn = stubFetch(async () => new Response(JSON.stringify({ version: 'dev-abc', commit_date: '2026-10-16T05:00:00Z', os: 'windows' })))
    expect(await fetchServerBuild('http://host:9870/')).toEqual({ version: 'dev-abc', commit_date: '2026-10-16T05:00:00Z' })
    expect(fn).toHaveBeenCalledWith('http://host:9870/version', expect.anything())
  })

  it('treats an older server without commit_date as unstamped', async () => {
    stubFetch(async () => new Response(JSON.stringify({ version: 'dev-f413920', build_date: '2026-10-10T17:20:42Z' })))
    expect(await fetchServerBuild('http://host:9870')).toEqual({ version: 'dev-f413920', commit_date: undefined })
  })

  it('returns null when unreachable, non-OK or not a BRUV /version', async () => {
    stubFetch(async () => { throw new TypeError('network') })
    expect(await fetchServerBuild('http://host')).toBeNull()
    stubFetch(async () => new Response('nope', { status: 404 }))
    expect(await fetchServerBuild('http://host')).toBeNull()
    stubFetch(async () => new Response(JSON.stringify({ hello: 'world' })))
    expect(await fetchServerBuild('http://host')).toBeNull()
  })
})
