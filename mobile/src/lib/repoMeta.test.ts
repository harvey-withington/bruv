import { describe, it, expect, vi, beforeEach } from 'vitest'

// REGRESSION (2026-08-10): the boot-time card-type/tag-colour load was
// fail-open AND latched `loaded = true` even when both RPCs failed — one
// bad moment at app start (offline/flaky link) cached an EMPTY type
// registry for the whole session: grey type badges, an empty type
// picker, and no retry path. The fix: failures are quiet but never
// cached — `loaded` latches only on full success, partial results render
// immediately, and ensureRepoMeta / the reconnect hook retry later.

const repoRPC = vi.fn()

vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>()
  return { ...actual, repoRPC: (...args: unknown[]) => repoRPC(...args) }
})

const { repoMeta, loadRepoMeta, ensureRepoMeta, loadProjectTags, resetRepoMeta } = await import('./repoMeta.svelte')

const TYPES = [{ id: 'home-todo', label: 'Home-Todo', color: '#f59e0b' }]
const COLORS = { urgent: '#ef4444' }

function respondWith(types: unknown, colors: unknown) {
  repoRPC.mockImplementation(async (method: string) => {
    const result = method === 'ListCardTypes' ? types : colors
    if (result instanceof Error) throw result
    return result
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  resetRepoMeta()
})

describe('loadRepoMeta failure semantics', () => {
  it('a failed load is NOT cached — loaded stays false and a retry succeeds', async () => {
    respondWith(new Error('offline'), new Error('offline'))
    await loadRepoMeta()
    expect(repoMeta.loaded).toBe(false)
    expect(repoMeta.cardTypes).toEqual([])

    respondWith(TYPES, COLORS)
    await loadRepoMeta()
    expect(repoMeta.loaded).toBe(true)
    expect(repoMeta.cardTypes).toEqual(TYPES)
    expect(repoMeta.tagColor('urgent')).toBe('#ef4444')
  })

  it('partial success stores what arrived but keeps retrying', async () => {
    respondWith(TYPES, new Error('offline'))
    await loadRepoMeta()
    // Types render immediately…
    expect(repoMeta.cardTypes).toEqual(TYPES)
    // …but the load isn't latched, so the missing half can still heal.
    expect(repoMeta.loaded).toBe(false)
  })

  it('a genuinely empty registry from a successful RPC IS a completed load', async () => {
    respondWith([], {})
    await loadRepoMeta()
    expect(repoMeta.loaded).toBe(true)
    expect(repoMeta.cardTypes).toEqual([])
  })
})

describe('ensureRepoMeta', () => {
  it('no-ops once loaded', async () => {
    respondWith(TYPES, COLORS)
    await loadRepoMeta()
    repoRPC.mockClear()
    await ensureRepoMeta()
    expect(repoRPC).not.toHaveBeenCalled()
  })

  it('retries while not loaded', async () => {
    respondWith(new Error('offline'), new Error('offline'))
    await loadRepoMeta()
    respondWith(TYPES, COLORS)
    await ensureRepoMeta()
    expect(repoMeta.loaded).toBe(true)
  })
})

describe('loadProjectTags failure semantics (same class as the registry bug)', () => {
  it('a failed load is NOT cached — the next call retries and succeeds', async () => {
    repoRPC.mockRejectedValueOnce(new Error('offline'))
    await loadProjectTags('b', 's', 'p')
    expect(repoMeta.tagColor('urgent', 'b/s/p')).toBe('var(--border)')

    repoRPC.mockResolvedValueOnce([{ name: 'urgent', color: '#ef4444' }])
    await loadProjectTags('b', 's', 'p')
    expect(repoMeta.tagColor('urgent', 'b/s/p')).toBe('#ef4444')
  })

  it('a successful load latches — no refetch on the next call', async () => {
    repoRPC.mockResolvedValueOnce([{ name: 'urgent', color: '#ef4444' }])
    await loadProjectTags('b', 's', 'p')
    repoRPC.mockClear()
    await loadProjectTags('b', 's', 'p')
    expect(repoRPC).not.toHaveBeenCalled()
  })

  it('concurrent calls for one project share the fetch', async () => {
    repoRPC.mockResolvedValue([])
    await Promise.all([loadProjectTags('b', 's', 'p'), loadProjectTags('b', 's', 'p')])
    expect(repoRPC).toHaveBeenCalledTimes(1)
  })
})

describe('concurrent loads', () => {
  it('share a single in-flight request', async () => {
    respondWith(TYPES, COLORS)
    await Promise.all([loadRepoMeta(), loadRepoMeta(), ensureRepoMeta()])
    // One ListCardTypes + one GetTagColors — not three of each.
    expect(repoRPC).toHaveBeenCalledTimes(2)
  })
})
