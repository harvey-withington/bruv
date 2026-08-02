import type { WorkspaceEntry } from '@shared/types'

/**
 * A workspace index entry as it arrives on the wire — including
 * `not_indexed`, set on directories the server records but deliberately
 * never walks (dependency/build caches: `node_modules`, `.venv`, `.next`…).
 * Named separately from the wire type only to keep call sites readable.
 */
export type WorkspaceIndexEntry = WorkspaceEntry

/** One row of the tree, with everything the renderer needs precomputed. */
export interface WorkspaceTreeNode {
  /** Slash-relative path from the workspace root — the stable render key. */
  path: string
  /** Last path segment (the display label). */
  name: string
  isDir: boolean
  /** Directory that exists but was never walked: no expander, no children. */
  notIndexed: boolean
  symlink: boolean
  /** Directory holding `.ft/template.json` — rendered as a generator. */
  isTemplateRoot: boolean
}

/** O(1) child lookup over a flat workspace index. */
export interface WorkspaceTree {
  /** Direct children of a directory path (`''` = workspace root). */
  childrenOf(dir: string): readonly WorkspaceTreeNode[]
  /** Nodes actually indexed (malformed paths are skipped). */
  readonly nodeCount: number
  readonly templateRoots: ReadonlySet<string>
}

const TEMPLATE_MARKER = '/.ft/template.json'
const NO_CHILDREN: readonly WorkspaceTreeNode[] = Object.freeze([])

/**
 * Paths the tree can place. The server emits clean slash-relative paths; a
 * degenerate one (empty, leading/trailing/double slash) has no well-defined
 * parent, and the old prefix-filter renderer silently dropped it too — so
 * skipping keeps behaviour identical instead of inventing a root-level row.
 */
function isRenderablePath(p: string): boolean {
  return p.length > 0 && !p.startsWith('/') && !p.endsWith('/') && !p.includes('//')
}

/**
 * Index a flat workspace index into parent → children buckets, ONCE.
 *
 * WHY: the tree renderer is recursive, and it used to hand the full flat
 * entries array to every nesting level, each of which filtered all n entries
 * to find its own children — O(n × mounted levels) of string work, redone on
 * every expand/collapse because `collapsed` is one shared reactive record. At
 * the 20k-entry server cap that is hundreds of thousands of `startsWith` +
 * `slice` + `includes` calls per click (Harvey, 2026-08-02: "any folder that
 * contains a node_modules folder is going to choke it").
 *
 * Building the map is a single O(n) pass; each level then does one Map lookup
 * and gets a stable array reference back, so re-renders don't re-derive.
 *
 * Sibling order is the server's order, preserved exactly — `core/workspace`
 * sorts entries by full path ascending, and the old filter was
 * order-preserving, so this keeps the tree looking identical (files and
 * folders interleaved by name, not folders-first).
 */
export function buildWorkspaceTree(entries: readonly WorkspaceIndexEntry[]): WorkspaceTree {
  const children = new Map<string, WorkspaceTreeNode[]>()
  const templateRoots = new Set<string>()
  const dirNodes: WorkspaceTreeNode[] = []
  let nodeCount = 0

  for (const e of entries) {
    const path = e.path
    if (!isRenderablePath(path)) continue

    if (e.is_dir !== true && path.endsWith(TEMPLATE_MARKER)) {
      templateRoots.add(path.slice(0, -TEMPLATE_MARKER.length))
    }

    const cut = path.lastIndexOf('/')
    const node: WorkspaceTreeNode = {
      path,
      name: cut === -1 ? path : path.slice(cut + 1),
      isDir: e.is_dir === true,
      notIndexed: e.not_indexed === true,
      symlink: e.symlink === true,
      isTemplateRoot: false,
    }

    const parent = cut === -1 ? '' : path.slice(0, cut)
    const bucket = children.get(parent)
    if (bucket) bucket.push(node)
    else children.set(parent, [node])

    if (node.isDir) dirNodes.push(node)
    nodeCount++
  }

  // Stamped after the walk, not during: a directory entry is seen before the
  // `.ft/template.json` inside it, so the set isn't complete until the end.
  for (const d of dirNodes) {
    if (templateRoots.has(d.path)) d.isTemplateRoot = true
  }

  return {
    childrenOf: (dir: string) => children.get(dir) ?? NO_CHILDREN,
    nodeCount,
    templateRoots,
  }
}
