import { describe, expect, it } from 'vitest'
import { buildWorkspaceFilesTree, mergeWorkspaceFiles, newWorkspaceFileEntry, sortedWorkspaceFiles } from '@shared/workspaceFiles'
import { asWorkspaceFiles, workspaceParentPath, workspacePathName } from '@shared/blockValues'
import type { WorkspaceFileEntry } from '@shared/types'

const e = (path: string, isDir = false, ws = 'ws-1'): WorkspaceFileEntry =>
  isDir ? { id: `id-${path}`, workspace_id: ws, path, is_dir: true } : { id: `id-${path}`, workspace_id: ws, path }

describe('buildWorkspaceFilesTree', () => {
  it('groups files under implied folders and sorts folders first', () => {
    const tree = buildWorkspaceFilesTree([e('zeta.md'), e('Chapters/02.md'), e('Chapters/01.md'), e('alpha.md')])
    expect(tree.map(n => n.name)).toEqual(['Chapters', 'alpha.md', 'zeta.md'])
    const chapters = tree[0]
    expect(chapters.entry).toBeUndefined()
    expect(chapters.children.map(n => n.name)).toEqual(['01.md', '02.md'])
    expect(chapters.children[0].entry?.path).toBe('Chapters/01.md')
  })

  it('keeps a folder entry as an entry node', () => {
    const tree = buildWorkspaceFilesTree([e('Episodes/EP002', true), e('Episodes/EP002/script.fountain')])
    expect(tree[0].name).toBe('Episodes')
    const ep = tree[0].children[0]
    expect(ep.entry?.is_dir).toBe(true)
    expect(ep.children[0].entry?.path).toBe('Episodes/EP002/script.fountain')
  })

  it('adds a root per workspace only when entries span workspaces', () => {
    const one = buildWorkspaceFilesTree([e('a.md')], () => 'Show')
    expect(one[0].name).toBe('a.md')
    const two = buildWorkspaceFilesTree([e('a.md', false, 'ws-1'), e('b.md', false, 'ws-2')], id => `Project ${id}`)
    expect(two.map(n => n.name)).toEqual(['Project ws-1', 'Project ws-2'])
    expect(two[0].children[0].name).toBe('a.md')
  })
})

describe('mergeWorkspaceFiles', () => {
  it('skips entries already present, ignoring a trailing slash', () => {
    const merged = mergeWorkspaceFiles([e('notes.md'), e('Chapters', true)], [e('notes.md'), e('Chapters/', true), e('new.md')])
    expect(merged.map(x => x.path)).toEqual(['notes.md', 'Chapters', 'new.md'])
  })
})

describe('entry helpers', () => {
  it('mints ids and normalises folder paths', () => {
    const d = newWorkspaceFileEntry('ws-1', 'Chapters/', true)
    expect(d.id).toMatch(/^wsf-/)
    expect(d.path).toBe('Chapters')
    expect(d.is_dir).toBe(true)
    expect(newWorkspaceFileEntry('ws-1', 'a.md', false).is_dir).toBeUndefined()
  })

  it('sorts flat display by path', () => {
    expect(sortedWorkspaceFiles([e('b.md'), e('A.md')]).map(x => x.path)).toEqual(['A.md', 'b.md'])
  })

  it('narrows stored values tolerantly', () => {
    expect(asWorkspaceFiles(null)).toEqual([])
    expect(asWorkspaceFiles([{ workspace_id: 'w', path: 'a.md' }, { path: 'nope' }, 'junk'])).toEqual([
      { id: 'wsf-w-a.md', workspace_id: 'w', path: 'a.md', is_dir: undefined },
    ])
    expect(workspacePathName('Chapters/01.md')).toBe('01.md')
    expect(workspacePathName('Chapters/')).toBe('Chapters')
    expect(workspaceParentPath('Chapters/01.md')).toBe('Chapters')
    expect(workspaceParentPath('01.md')).toBe('')
  })
})
