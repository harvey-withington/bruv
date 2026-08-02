import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import type { WorkspaceEntry } from '@shared/types'
import WorkspaceFileTree from './WorkspaceFileTree.svelte'
import { createWorkspaceDirCache, type WorkspaceDirLoader } from '../../lib/workspaceTree.svelte'

// The headline behaviour: the tree opens COLLAPSED and each folder costs one
// ListWorkspaceDir call the first time it is opened — never the whole
// workspace (Harvey's ruling, 2026-08-02). These tests hold that line at the
// component level, where the `collapsed` polarity (absent = collapsed, only
// `=== false` expands) actually bites.
//
// Expansion is driven through `rerender({ collapsed })` rather than clicks:
// the record is a plain object here, so only the owning component's $state
// makes clicks reactive. What matters — a level mounts, fetches once, and a
// re-mount is served from cache — is exercised either way.

const TREE: Record<string, WorkspaceEntry[]> = {
  '': [{ path: 'README.md' }, { path: 'node_modules', is_dir: true }, { path: 'src', is_dir: true }],
  src: [{ path: 'src/main.ts' }],
  node_modules: [{ path: 'node_modules/left-pad', is_dir: true }],
}

function setup(custom?: WorkspaceDirLoader) {
  const calls: string[] = []
  const loader: WorkspaceDirLoader = (d) => {
    calls.push(d)
    return custom ? custom(d) : Promise.resolve(TREE[d] ?? [])
  }
  const cache = createWorkspaceDirCache(loader)
  const view = render(WorkspaceFileTree, { props: { cache, collapsed: {} } })
  const expand = (...dirs: string[]) => {
    const collapsed: Record<string, boolean> = {}
    for (const d of dirs) collapsed[d] = false
    return view.rerender({ collapsed })
  }
  return { ...view, cache, calls, expand }
}

/** Lets the mount effect fire, the loader promise settle, and the DOM update. */
async function settle() {
  await tick()
  await Promise.resolve()
  await tick()
}

describe('WorkspaceFileTree — lazy loading', () => {
  it('loads only the root and shows every folder collapsed', async () => {
    const { calls } = setup()
    await settle()
    expect(calls).toEqual([''])
    expect(screen.getByText('README.md')).toBeTruthy()
    expect(screen.getByText('src')).toBeTruthy()
    // Nothing below the root was fetched or rendered.
    expect(screen.queryByText('main.ts')).toBeNull()
  })

  it('fetches a folder on first expand and never again on re-expand', async () => {
    const { calls, expand } = setup()
    await settle()
    await expand('src')
    await settle()
    expect(screen.getByText('main.ts')).toBeTruthy()
    expect(calls).toEqual(['', 'src'])

    // Collapse (the level unmounts), then expand again: served from cache.
    await expand()
    await settle()
    expect(screen.queryByText('main.ts')).toBeNull()
    await expand('src')
    await settle()
    expect(screen.getByText('main.ts')).toBeTruthy()
    expect(calls).toEqual(['', 'src'])
  })

  it('never reads a folder the user did not open, however heavy', async () => {
    const { calls, expand } = setup()
    await settle()
    await expand('src')
    await settle()
    expect(calls).not.toContain('node_modules')
    expect(screen.queryByText('left-pad')).toBeNull()
  })

  it('shows a retry affordance when a folder fails, not an empty folder', async () => {
    const custom = vi
      .fn<WorkspaceDirLoader>()
      .mockResolvedValueOnce(TREE[''])
      .mockRejectedValueOnce(new Error('EACCES'))
      .mockResolvedValueOnce(TREE.src)
    const { getByText, queryByText, expand } = setup(custom)
    await settle()
    await expand('src')
    await settle()
    expect(getByText("Couldn't read this folder")).toBeTruthy()
    expect(queryByText('main.ts')).toBeNull()

    await fireEvent.click(getByText('Try again'))
    await settle()
    expect(getByText('main.ts')).toBeTruthy()
    expect(queryByText("Couldn't read this folder")).toBeNull()
  })

  it('reloads a visible level after the cache is cleared (Refresh)', async () => {
    const { cache, calls } = setup()
    await settle()
    cache.clear()
    await settle()
    expect(calls).toEqual(['', ''])
    expect(screen.getByText('README.md')).toBeTruthy()
  })
})
