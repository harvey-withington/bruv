import { describe, it, expect, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import { setBackend } from '@shared/adapters'
import type { WorkspaceEntry, WorkspaceFileEntry } from '@shared/types'
import { buildWorkspaceFilesTree } from '@shared/workspaceFiles'
import { createMockAdapter } from '../../lib/adapters/mock'
import { refreshWorkspaceDirs } from '../../lib/workspaceLocations.svelte'
import WorkspaceFilesTreeView from './WorkspaceFilesTreeView.svelte'

// Field report 2026-09-17: clicking a folder's CHEVRON in the Workspace
// Files block sometimes didn't expand it while clicking the label always
// did. Both sit inside one button, so the test clicks the chevron's SVG
// specifically and asserts one toggle per click.

const LISTING: Record<string, WorkspaceEntry[]> = {
  '': [{ path: 'Chapters', is_dir: true }, { path: 'notes.md' }],
  Chapters: [{ path: 'Chapters/01.md' }, { path: 'Chapters/02.md' }],
}

const folder: WorkspaceFileEntry = { id: 'wsf-1', workspace_id: 'ws-1', path: 'Chapters', is_dir: true }

async function settle() {
  await tick()
  await Promise.resolve()
  await Promise.resolve()
  await tick()
}

beforeEach(() => {
  setBackend({
    ...createMockAdapter(),
    ResolveWorkspace: async () => ({ brand_slug: 'b', stream_slug: 's', project_slug: 'p', workspace: { id: 'ws-1', project_id: 'p', origin: { kind: 'local', url: 'C:/x' }, adapter: 'plain-folder', created_at: '', updated_at: '' } }),
    ListWorkspaceDir: async (_b: string, _s: string, _p: string, rel: string) => LISTING[rel] ?? [],
  })
  refreshWorkspaceDirs('ws-1')
})

describe('WorkspaceFilesTreeView folder entry', () => {
  it('toggles once per click on the chevron, and once per click on the label', async () => {
    const expanded: Record<string, boolean> = {}
    const { container, rerender } = render(WorkspaceFilesTreeView, {
      props: { nodes: buildWorkspaceFilesTree([folder]), expanded, onOpen: () => {}, onOpenPath: () => {}, onRemove: () => {}, onRelink: () => {} },
    })
    await settle()
    const button = container.querySelector('.entry .main') as HTMLButtonElement
    expect(button).not.toBeNull()
    expect(button.disabled).toBe(false) // parent listing found it — not "missing"
    expect(container.querySelector('.subtree')).toBeNull()

    // Chevron: the first SVG inside the button.
    await fireEvent.click(button.querySelector('svg') as SVGElement)
    await rerender({ nodes: buildWorkspaceFilesTree([folder]), expanded, onOpen: () => {}, onOpenPath: () => {}, onRemove: () => {}, onRelink: () => {} })
    await settle()
    expect(expanded['ws-1:Chapters']).toBe(true)
    expect(container.querySelector('.subtree')).not.toBeNull()

    // Chevron again (now the "down" icon): collapses, never a no-op.
    await fireEvent.click(button.querySelector('svg') as SVGElement)
    await rerender({ nodes: buildWorkspaceFilesTree([folder]), expanded, onOpen: () => {}, onOpenPath: () => {}, onRemove: () => {}, onRelink: () => {} })
    await settle()
    expect(expanded['ws-1:Chapters']).toBe(false)
    expect(container.querySelector('.subtree')).toBeNull()

    // Label
    await fireEvent.click(button.querySelector('.name') as HTMLElement)
    await rerender({ nodes: buildWorkspaceFilesTree([folder]), expanded, onOpen: () => {}, onOpenPath: () => {}, onRemove: () => {}, onRelink: () => {} })
    await settle()
    expect(expanded['ws-1:Chapters']).toBe(true)
  })
})
