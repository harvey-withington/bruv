import { describe, it, expect } from 'vitest'
import { buildWorkspaceTree, type WorkspaceIndexEntry, type WorkspaceTreeNode } from './workspaceTree'

// The Workspace file tree used to hand the full flat entry list to every
// recursive level, each re-filtering all n entries for its own children —
// O(n × levels) per render, redone on every expand/collapse. These tests pin
// the indexed replacement: one O(n) pass, O(1) lookups, server order kept.

function dir(path: string, extra: Partial<WorkspaceIndexEntry> = {}): WorkspaceIndexEntry {
  return { path, is_dir: true, ...extra }
}
function file(path: string, extra: Partial<WorkspaceIndexEntry> = {}): WorkspaceIndexEntry {
  return { path, ...extra }
}
function names(nodes: readonly WorkspaceTreeNode[]): string[] {
  return nodes.map(n => n.name)
}

describe('buildWorkspaceTree', () => {
  it('returns root-level children for the empty parent key', () => {
    const tree = buildWorkspaceTree([dir('src'), file('README.md'), file('src/main.ts')])
    expect(names(tree.childrenOf(''))).toEqual(['src', 'README.md'])
  })

  it('looks up nested children by their parent directory path', () => {
    const tree = buildWorkspaceTree([
      dir('src'),
      dir('src/lib'),
      file('src/lib/a.ts'),
      file('src/main.ts'),
    ])
    expect(names(tree.childrenOf('src'))).toEqual(['lib', 'main.ts'])
    expect(names(tree.childrenOf('src/lib'))).toEqual(['a.ts'])
  })

  it('resolves deep paths without re-walking ancestors', () => {
    const deep = 'a/b/c/d/e/f/g/h'
    const entries = deep
      .split('/')
      .map((_, i, parts) => dir(parts.slice(0, i + 1).join('/')))
    entries.push(file(`${deep}/leaf.txt`))
    const tree = buildWorkspaceTree(entries)
    expect(names(tree.childrenOf('a/b/c/d/e/f/g'))).toEqual(['h'])
    expect(names(tree.childrenOf(deep))).toEqual(['leaf.txt'])
  })

  it('handles empty input', () => {
    const tree = buildWorkspaceTree([])
    expect(tree.childrenOf('')).toEqual([])
    expect(tree.nodeCount).toBe(0)
  })

  it('returns an empty list for unknown or leaf directories', () => {
    const tree = buildWorkspaceTree([dir('src'), file('src/main.ts')])
    expect(tree.childrenOf('nope')).toEqual([])
    expect(tree.childrenOf('src/main.ts')).toEqual([])
  })

  it('preserves the server order within a level (path-ascending, not folders-first)', () => {
    // core/workspace sorts by full path, so uppercase sorts before lowercase
    // and files interleave with folders. The old prefix filter was
    // order-preserving; the index must look identical on screen.
    const tree = buildWorkspaceTree([
      file('README.md'),
      dir('assets'),
      file('index.html'),
      dir('src'),
      file('src/z.ts'),
    ])
    expect(names(tree.childrenOf(''))).toEqual(['README.md', 'assets', 'index.html', 'src'])
  })

  it('splits on the last slash only, so slash-adjacent names stay put', () => {
    // "foo.txt" sorts BETWEEN "foo" and "foo/bar" ('.' < '/'), and
    // "foo-bar" is a sibling of "foo", never a child of it.
    const tree = buildWorkspaceTree([
      dir('foo'),
      file('foo.txt'),
      file('foo/bar'),
      dir('foo-bar'),
      file('foo-bar/baz'),
    ])
    expect(names(tree.childrenOf(''))).toEqual(['foo', 'foo.txt', 'foo-bar'])
    expect(names(tree.childrenOf('foo'))).toEqual(['bar'])
    expect(names(tree.childrenOf('foo-bar'))).toEqual(['baz'])
  })

  it('keeps dotted segments addressable as ordinary directories', () => {
    const tree = buildWorkspaceTree([dir('.github'), dir('.github/workflows'), file('.github/workflows/ci.yml')])
    expect(names(tree.childrenOf('.github/workflows'))).toEqual(['ci.yml'])
    expect(tree.childrenOf('')[0]?.name).toBe('.github')
  })

  it('skips degenerate paths instead of inventing root-level rows', () => {
    // The prefix-filter renderer dropped these silently; so does the index.
    const tree = buildWorkspaceTree([
      file(''),
      file('/absolute.txt'),
      dir('trailing/'),
      file('double//slash.txt'),
      file('ok.txt'),
    ])
    expect(names(tree.childrenOf(''))).toEqual(['ok.txt'])
    expect(tree.nodeCount).toBe(1)
  })

  it('carries is_dir, symlink and not_indexed onto the node', () => {
    const tree = buildWorkspaceTree([
      dir('node_modules', { not_indexed: true }),
      file('link', { symlink: true }),
      file('plain.txt', { size: 12 }),
    ])
    const [mods, link, plain] = tree.childrenOf('')
    expect(mods).toMatchObject({ isDir: true, notIndexed: true })
    expect(link).toMatchObject({ isDir: false, symlink: true, notIndexed: false })
    expect(plain).toMatchObject({ isDir: false, symlink: false, notIndexed: false })
  })

  it('never lists children under a not-indexed directory', () => {
    // The server stops walking at node_modules, so there is nothing to show —
    // the UI hides the expander rather than rendering an empty folder.
    const tree = buildWorkspaceTree([dir('node_modules', { not_indexed: true }), file('package.json')])
    expect(tree.childrenOf('node_modules')).toEqual([])
  })

  it('flags folder-template roots, including when the marker follows the dir entry', () => {
    const tree = buildWorkspaceTree([
      dir('Templates'),
      dir('Templates/Episode'),
      dir('Templates/Episode/.ft'),
      file('Templates/Episode/.ft/template.json'),
      dir('Plain'),
    ])
    expect(tree.templateRoots.has('Templates/Episode')).toBe(true)
    const [episode] = tree.childrenOf('Templates')
    expect(episode.isTemplateRoot).toBe(true)
    expect(tree.childrenOf('')[1].isTemplateRoot).toBe(false)
  })

  it('does not treat a directory named like the marker as a template root', () => {
    const tree = buildWorkspaceTree([dir('x'), dir('x/.ft'), dir('x/.ft/template.json')])
    expect(tree.templateRoots.size).toBe(0)
  })
})

describe('buildWorkspaceTree at index scale', () => {
  // 20k is the server's MaxIndexEntries cap — the size that used to choke the
  // renderer.
  function syntheticIndex(total: number): WorkspaceIndexEntry[] {
    const entries: WorkspaceIndexEntry[] = []
    let n = 0
    for (let a = 0; n < total; a++) {
      entries.push(dir(`d${a}`))
      n++
      for (let b = 0; b < 20 && n < total; b++) {
        entries.push(dir(`d${a}/s${b}`))
        n++
        for (let c = 0; c < 20 && n < total; c++) {
          entries.push(file(`d${a}/s${b}/f${c}.ts`))
          n++
        }
      }
    }
    return entries
  }

  it('reads each entry once at build time and never again on lookup', () => {
    const entries = syntheticIndex(20000)
    let reads = 0
    // Counts indexed reads of the source array: proves lookups are Map hits,
    // not a re-scan of the entries (the old per-level filter).
    const counted = new Proxy(entries, {
      get(target, prop, receiver) {
        if (typeof prop === 'string' && /^\d+$/.test(prop)) reads++
        return Reflect.get(target, prop, receiver)
      },
    })

    const tree = buildWorkspaceTree(counted)
    const buildReads = reads
    expect(buildReads).toBe(entries.length)
    expect(tree.nodeCount).toBe(entries.length)

    for (let i = 0; i < 1000; i++) {
      const a = i % 40 // only fully-populated top dirs, so lookups are non-empty
      expect(tree.childrenOf(`d${a}`).length).toBeGreaterThan(0)
      expect(tree.childrenOf(`d${a}/s${i % 20}`).length).toBeGreaterThan(0)
    }
    expect(reads).toBe(buildReads)
  })

  it('hands back the same array instance for repeat lookups, so re-renders re-derive nothing', () => {
    const tree = buildWorkspaceTree(syntheticIndex(20000))
    expect(tree.childrenOf('d3')).toBe(tree.childrenOf('d3'))
    expect(tree.childrenOf('missing')).toBe(tree.childrenOf('also-missing'))
  })

  it('places every entry under exactly one parent', () => {
    const entries = syntheticIndex(20000)
    const tree = buildWorkspaceTree(entries)
    // Every node reachable by walking parents == every entry indexed: no
    // duplicates, no orphans.
    let seen = 0
    const walk = (path: string) => {
      for (const n of tree.childrenOf(path)) {
        seen++
        if (n.isDir) walk(n.path)
      }
    }
    walk('')
    expect(seen).toBe(entries.length)
  })
})
