import { describe, it, expect, vi } from 'vitest'
import type { WorkspaceEntry } from '@shared/types'
import {
  createWorkspaceDirCache,
  baseName,
  parentPath,
  isRenderablePath,
  type WorkspaceDirLoader,
} from './workspaceTree.svelte'

// The Workspace file tree used to render from one whole-workspace index, so
// opening the panel cost whatever was on disk (node_modules, .git, a photo
// dump). It now browses ONE directory at a time via ListWorkspaceDir. These
// tests pin the cache that makes that safe: fetch once, never refetch on
// re-expand, never touch a directory nobody opened, and never let a failure
// masquerade as an empty folder.

function dir(path: string): WorkspaceEntry {
  return { path, is_dir: true }
}
function file(path: string, extra: Partial<WorkspaceEntry> = {}): WorkspaceEntry {
  return { path, ...extra }
}

/** A fake ListWorkspaceDir over a canned {dir → children} map. No network. */
function fakeLoader(byDir: Record<string, WorkspaceEntry[]>) {
  const calls: string[] = []
  const loader: WorkspaceDirLoader = async (d) => {
    calls.push(d)
    return byDir[d] ?? []
  }
  return { loader, calls }
}

/** A loader whose responses are resolved by hand, so ordering is testable. */
function deferredLoader() {
  const calls: string[] = []
  const pending: { resolve: (e: WorkspaceEntry[]) => void; reject: (e: Error) => void }[] = []
  const loader: WorkspaceDirLoader = (d) => {
    calls.push(d)
    return new Promise<WorkspaceEntry[]>((resolve, reject) => pending.push({ resolve, reject }))
  }
  return { loader, calls, pending }
}

const SAMPLE: Record<string, WorkspaceEntry[]> = {
  '': [file('README.md'), dir('src'), dir('node_modules')],
  src: [dir('src/lib'), file('src/main.ts')],
  'src/lib': [file('src/lib/a.ts')],
  node_modules: [dir('node_modules/left-pad')],
}

describe('path helpers', () => {
  it('names root-level paths as themselves and nested paths by last segment', () => {
    expect(baseName('README.md')).toBe('README.md')
    expect(baseName('src/lib/a.ts')).toBe('a.ts')
    expect(baseName('.github')).toBe('.github')
  })

  it('maps a path to its parent, with the root key for top-level entries', () => {
    expect(parentPath('README.md')).toBe('')
    expect(parentPath('src/main.ts')).toBe('src')
    expect(parentPath('a/b/c/d.txt')).toBe('a/b/c')
  })

  it('splits on the last slash only, so slash-adjacent names stay siblings', () => {
    // "foo-bar" is a sibling of "foo", never a child of it.
    expect(parentPath('foo-bar/baz')).toBe('foo-bar')
    expect(parentPath('foo/bar')).toBe('foo')
  })

  it('rejects degenerate paths that have no well-defined parent', () => {
    expect(isRenderablePath('ok.txt')).toBe(true)
    expect(isRenderablePath('a/b.txt')).toBe(true)
    expect(isRenderablePath('')).toBe(false)
    expect(isRenderablePath('/absolute.txt')).toBe(false)
    expect(isRenderablePath('trailing/')).toBe(false)
    expect(isRenderablePath('double//slash.txt')).toBe(false)
  })
})

describe('workspace dir cache — loading', () => {
  it('reports nothing for a directory that has never been asked for', () => {
    const { loader, calls } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    expect(cache.get('')).toBeUndefined()
    expect(calls).toEqual([])
  })

  it('exposes a loading state synchronously, then the children', async () => {
    const { loader, pending } = deferredLoader()
    const cache = createWorkspaceDirCache(loader)
    const p = cache.ensure('src')
    expect(cache.get('src')).toEqual({ status: 'loading' })
    pending[0].resolve([file('src/main.ts')])
    await p
    expect(cache.get('src')?.status).toBe('ready')
  })

  it('maps entries to nodes with name, isDir and symlink', async () => {
    const { loader } = fakeLoader({
      '': [dir('src'), file('link', { symlink: true }), file('plain.txt', { size: 12 })],
    })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    const state = cache.get('')
    if (state?.status !== 'ready') throw new Error('expected ready')
    expect(state.children).toEqual([
      { path: 'src', name: 'src', isDir: true, symlink: false },
      { path: 'link', name: 'link', isDir: false, symlink: true },
      { path: 'plain.txt', name: 'plain.txt', isDir: false, symlink: false },
    ])
  })

  it('keeps the server order within a level (path-ascending, not folders-first)', async () => {
    const { loader } = fakeLoader({
      '': [file('README.md'), dir('assets'), file('index.html'), dir('src')],
    })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    const state = cache.get('')
    if (state?.status !== 'ready') throw new Error('expected ready')
    expect(state.children.map((n) => n.name)).toEqual(['README.md', 'assets', 'index.html', 'src'])
  })

  it('skips degenerate paths instead of rendering nameless rows', async () => {
    const { loader } = fakeLoader({
      '': [file(''), file('/absolute.txt'), dir('trailing/'), file('double//slash.txt'), file('ok.txt')],
    })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    const state = cache.get('')
    if (state?.status !== 'ready') throw new Error('expected ready')
    expect(state.children.map((n) => n.name)).toEqual(['ok.txt'])
  })

  it('renders an empty directory as genuinely empty, distinct from loading and error', async () => {
    const { loader } = fakeLoader({ empty: [] })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('empty')
    expect(cache.get('empty')).toEqual({ status: 'ready', children: [], isTemplateRoot: false })
  })
})

describe('workspace dir cache — cache hits and misses', () => {
  it('fetches on a miss and serves later reads from cache (no refetch on re-expand)', async () => {
    const { loader, calls } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('src')
    // Collapse + re-expand + re-expand again: still one fetch.
    await cache.ensure('src')
    await cache.ensure('src')
    expect(calls).toEqual(['src'])
  })

  it('collapses concurrent ensures of the same directory into one fetch', async () => {
    const { loader, calls, pending } = deferredLoader()
    const cache = createWorkspaceDirCache(loader)
    const a = cache.ensure('src')
    const b = cache.ensure('src')
    pending[0].resolve([file('src/main.ts')])
    await Promise.all([a, b])
    expect(calls).toEqual(['src'])
  })

  it('loading one directory never triggers loads of any other', async () => {
    const { loader, calls } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    // The root lists node_modules and src — neither is read until opened.
    expect(calls).toEqual([''])
    expect(cache.get('src')).toBeUndefined()
    expect(cache.get('node_modules')).toBeUndefined()

    await cache.ensure('src')
    expect(calls).toEqual(['', 'src'])
    expect(cache.get('src/lib')).toBeUndefined()
    expect(cache.get('node_modules')).toBeUndefined()
  })

  it('treats the workspace root and nested paths as ordinary, independent keys', async () => {
    const { loader, calls } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    await cache.ensure('src/lib')
    expect(calls).toEqual(['', 'src/lib'])
    const root = cache.get('')
    const lib = cache.get('src/lib')
    if (root?.status !== 'ready' || lib?.status !== 'ready') throw new Error('expected ready')
    expect(root.children.map((n) => n.path)).toEqual(['README.md', 'src', 'node_modules'])
    expect(lib.children.map((n) => n.path)).toEqual(['src/lib/a.ts'])
  })

  it('lists only ready directories as loaded — the bound on Expand All', async () => {
    const { loader, pending } = deferredLoader()
    const cache = createWorkspaceDirCache(loader)
    await (async () => {
      const p = cache.ensure('')
      pending[0].resolve([dir('src')])
      await p
    })()
    const slow = cache.ensure('src')
    const failing = cache.ensure('broken')
    pending[2].reject(new Error('nope'))
    await failing
    // 'src' is still in flight, 'broken' errored: neither counts as loaded.
    expect(cache.loadedDirs()).toEqual([''])
    pending[1].resolve([])
    await slow
    expect(cache.loadedDirs().sort()).toEqual(['', 'src'])
  })
})

describe('workspace dir cache — errors', () => {
  it('captures the failure per directory instead of showing an empty folder', async () => {
    const loader = vi.fn<WorkspaceDirLoader>().mockRejectedValue(new Error('permission denied'))
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('secret')
    expect(cache.get('secret')).toEqual({ status: 'error', error: 'permission denied' })
  })

  it('retains the error across re-expands — ensure() never silently retries', async () => {
    const loader = vi.fn<WorkspaceDirLoader>().mockRejectedValue(new Error('EIO'))
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('bad')
    await cache.ensure('bad')
    await cache.ensure('bad')
    expect(loader).toHaveBeenCalledTimes(1)
    expect(cache.get('bad')?.status).toBe('error')
  })

  it('reload() is the retry: it refetches a failed directory and can recover', async () => {
    const loader = vi
      .fn<WorkspaceDirLoader>()
      .mockRejectedValueOnce(new Error('EBUSY'))
      .mockResolvedValueOnce([file('bad/ok.txt')])
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('bad')
    expect(cache.get('bad')?.status).toBe('error')
    await cache.reload('bad')
    expect(loader).toHaveBeenCalledTimes(2)
    const state = cache.get('bad')
    if (state?.status !== 'ready') throw new Error('expected ready')
    expect(state.children.map((n) => n.name)).toEqual(['ok.txt'])
  })

  it('stringifies a non-Error rejection rather than losing the reason', async () => {
    const loader: WorkspaceDirLoader = () => Promise.reject('rpc down')
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('x')
    expect(cache.get('x')).toEqual({ status: 'error', error: 'rpc down' })
  })

  it('errors in one directory leave its siblings untouched', async () => {
    const loader: WorkspaceDirLoader = async (d) => {
      if (d === 'bad') throw new Error('EACCES')
      return [file(`${d}/ok.txt`)]
    }
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('bad')
    await cache.ensure('good')
    expect(cache.get('bad')?.status).toBe('error')
    expect(cache.get('good')?.status).toBe('ready')
  })
})

describe('workspace dir cache — invalidation', () => {
  it('clear() drops every directory so the next expand refetches', async () => {
    const { loader, calls } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('')
    await cache.ensure('src')
    cache.clear()
    expect(cache.get('')).toBeUndefined()
    expect(cache.get('src')).toBeUndefined()
    expect(cache.loadedDirs()).toEqual([])
    await cache.ensure('')
    expect(calls).toEqual(['', 'src', ''])
  })

  it('discards an in-flight response that lands after a clear', async () => {
    const { loader, pending } = deferredLoader()
    const cache = createWorkspaceDirCache(loader)
    const p = cache.ensure('src')
    cache.clear()
    pending[0].resolve([file('src/stale.ts')])
    await p
    // The stale listing must not resurrect a directory the user just refreshed.
    expect(cache.get('src')).toBeUndefined()
  })

  it('discards an in-flight failure that lands after a clear', async () => {
    const { loader, pending } = deferredLoader()
    const cache = createWorkspaceDirCache(loader)
    const p = cache.ensure('src')
    cache.clear()
    pending[0].reject(new Error('too late'))
    await p
    expect(cache.get('src')).toBeUndefined()
  })
})

describe('workspace dir cache — template roots', () => {
  it('flags a directory whose own children include a .ft folder', async () => {
    const { loader } = fakeLoader({
      Templates: [dir('Templates/Episode')],
      'Templates/Episode': [dir('Templates/Episode/.ft'), file('Templates/Episode/notes.md')],
    })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('Templates/Episode')
    expect(cache.isTemplateRoot('Templates/Episode')).toBe(true)
  })

  it('reports false for a directory that is loaded but has no marker', async () => {
    const { loader } = fakeLoader({ Plain: [file('Plain/a.md')] })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('Plain')
    expect(cache.isTemplateRoot('Plain')).toBe(false)
  })

  it('reports false for an unloaded directory instead of guessing', async () => {
    // Lazily, template-root-ness is only knowable once the folder is opened —
    // the icon appears then, and nothing waits on it.
    const { loader } = fakeLoader(SAMPLE)
    const cache = createWorkspaceDirCache(loader)
    expect(cache.isTemplateRoot('Templates/Episode')).toBe(false)
    await cache.ensure('')
    expect(cache.isTemplateRoot('src')).toBe(false)
  })

  it('does not accept a FILE named .ft as the marker', async () => {
    const { loader } = fakeLoader({ x: [file('x/.ft')] })
    const cache = createWorkspaceDirCache(loader)
    await cache.ensure('x')
    expect(cache.isTemplateRoot('x')).toBe(false)
  })
})
