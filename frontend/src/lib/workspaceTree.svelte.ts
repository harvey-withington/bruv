import type { WorkspaceEntry } from '@shared/types'

/**
 * Lazy, per-directory cache behind the Workspace file tree.
 *
 * WHY LAZY: the tree used to render from one whole-workspace index, so the
 * cost of opening the panel was proportional to everything on disk. A single
 * `node_modules`, a nested `.git`, or a folder of 80k photos anywhere under
 * the root choked it. The first fix attempt was a server-side blocklist of
 * known-heavy directory names; Harvey rejected it (2026-08-02) as "a band-aid,
 * not a solution — this issue would apply to a .git folder or many others
 * neither you nor I can predict. The tree should load collapsed and load
 * incrementally."
 *
 * So: nothing is fetched until a folder is actually opened, and each fetch
 * costs exactly ONE directory (`ListWorkspaceDir`). A directory nobody opens
 * is never read, no matter what is inside it.
 *
 * The cache is keyed by directory path ('' = workspace root) and holds both
 * the children and the load state, because "empty", "still loading" and
 * "failed" must be three visibly different things in the tree — an
 * error rendered as an empty folder is exactly the silent-fallback bug class
 * this project keeps hunting.
 */

/** One row of the tree, precomputed for the renderer. */
export interface WorkspaceTreeNode {
  /** Slash-relative path from the workspace root — the stable render key. */
  path: string
  /** Last path segment (the display label). */
  name: string
  isDir: boolean
  symlink: boolean
}

/**
 * State of one directory in the cache. A directory is absent from the cache
 * until something asks for it; the union then covers the three outcomes the
 * tree must render distinctly.
 */
export type WorkspaceDirState =
  | { status: 'loading' }
  | {
      status: 'ready'
      /** Empty array = a genuinely empty directory. */
      children: readonly WorkspaceTreeNode[]
      /** This directory's own children include a `.ft` folder. */
      isTemplateRoot: boolean
    }
  | { status: 'error'; error: string }

/** Fetches the immediate children of one directory ('' = workspace root). */
export type WorkspaceDirLoader = (dir: string) => Promise<readonly WorkspaceEntry[]>

export interface WorkspaceDirCache {
  /** Reactive read: `undefined` until the directory has been asked for. */
  get(dir: string): WorkspaceDirState | undefined
  /** Load a directory once. A cached result — including a cached error — never refetches. */
  ensure(dir: string): Promise<void>
  /** Force a fetch regardless of cache state (retry after an error). */
  reload(dir: string): Promise<void>
  /** Drop everything (Refresh / project switch). In-flight loads are discarded. */
  clear(): void
  /** Directories whose children are loaded — the bound on Expand All. */
  loadedDirs(): string[]
  /** Known-template-root test; false while the directory is unloaded. */
  isTemplateRoot(dir: string): boolean
}

/**
 * Folder-Template marker. Eagerly, template roots were found by scanning the
 * whole flat index for `<dir>/.ft/template.json`. Lazily, a directory can only
 * be classified once its OWN children are loaded — so the rule is "a loaded
 * directory whose children include a `.ft` folder". The icon therefore appears
 * when the folder is opened; nothing waits on it.
 */
const TEMPLATE_MARKER_DIR = '.ft'

/** Last path segment. Root-level paths have no slash and are their own name. */
export function baseName(path: string): string {
  const cut = path.lastIndexOf('/')
  return cut === -1 ? path : path.slice(cut + 1)
}

/** Parent directory of a path; `''` for root-level entries (the root's key). */
export function parentPath(path: string): string {
  const cut = path.lastIndexOf('/')
  return cut === -1 ? '' : path.slice(0, cut)
}

/**
 * Paths the tree can place. The server emits clean slash-relative paths; a
 * degenerate one (empty, leading/trailing/double slash) has no well-defined
 * parent and would render as a nameless row, so it is skipped — same as the
 * original prefix-filter renderer did.
 */
export function isRenderablePath(p: string): boolean {
  return p.length > 0 && !p.startsWith('/') && !p.endsWith('/') && !p.includes('//')
}

export function createWorkspaceDirCache(load: WorkspaceDirLoader): WorkspaceDirCache {
  let dirs = $state<Record<string, WorkspaceDirState>>({})
  // Bumped by clear(): a response from before the clear must not stamp stale
  // children back over a freshly emptied cache (Refresh mid-flight).
  let generation = 0

  async function fetchDir(dir: string): Promise<void> {
    const gen = generation
    dirs[dir] = { status: 'loading' }
    try {
      const entries = await load(dir)
      if (gen !== generation) return
      const children: WorkspaceTreeNode[] = []
      let isTemplateRoot = false
      for (const e of entries) {
        if (!isRenderablePath(e.path)) continue
        const node: WorkspaceTreeNode = {
          path: e.path,
          name: baseName(e.path),
          isDir: e.is_dir === true,
          symlink: e.symlink === true,
        }
        if (node.isDir && node.name === TEMPLATE_MARKER_DIR) isTemplateRoot = true
        children.push(node)
      }
      // Server order kept as-is (full path ascending: files and folders
      // interleaved by name, not folders-first) so a lazily loaded level
      // reads identically to the eager tree it replaced.
      dirs[dir] = { status: 'ready', children, isTemplateRoot }
    } catch (err) {
      if (gen !== generation) return
      dirs[dir] = { status: 'error', error: err instanceof Error ? err.message : String(err) }
    }
  }

  return {
    get: (dir) => dirs[dir],
    ensure(dir) {
      // Cache hit — ready, loading OR error — never refetches: re-collapsing
      // and re-expanding a folder must cost nothing, and a failed directory
      // must keep showing its error until the user retries.
      if (dirs[dir]) return Promise.resolve()
      return fetchDir(dir)
    },
    reload: (dir) => fetchDir(dir),
    clear() {
      generation++
      dirs = {}
    },
    loadedDirs: () => Object.keys(dirs).filter((d) => dirs[d].status === 'ready'),
    isTemplateRoot(dir) {
      const s = dirs[dir]
      return s?.status === 'ready' && s.isTemplateRoot
    },
  }
}
