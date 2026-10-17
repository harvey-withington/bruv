// Line-based three-way text merge for offline sync.
//
// mine and theirs both started from base. Regions only one side changed
// take that side; regions both changed identically take either; regions
// both changed differently are a conflict and the merge reports failure
// so the caller can ask the user. No conflict markers are ever written
// into the text.

export type Merge3Result = { clean: true; text: string } | { clean: false }

export function merge3(base: string, mine: string, theirs: string): Merge3Result {
  if (mine === theirs || theirs === base) return { clean: true, text: mine }
  if (mine === base) return { clean: true, text: theirs }

  const b = base.split('\n')
  const m = mine.split('\n')
  const t = theirs.split('\n')
  const toMine = matchMap(b, m)
  const toTheirs = matchMap(b, t)

  const out: string[] = []
  let i = 0, j = 0, k = 0
  while (i < b.length || j < m.length || k < t.length) {
    // A stable line: base line i is matched to the current line on both sides.
    if (i < b.length && toMine[i] === j && toTheirs[i] === k) {
      out.push(b[i])
      i++; j++; k++
      continue
    }
    // Otherwise the chunk runs to the next base line matched on both sides.
    let ni = i
    while (ni < b.length && !(toMine[ni] >= j && toTheirs[ni] >= k)) ni++
    const nj = ni < b.length ? toMine[ni] : m.length
    const nk = ni < b.length ? toTheirs[ni] : t.length
    const bc = b.slice(i, ni), mc = m.slice(j, nj), tc = t.slice(k, nk)
    if (sameLines(mc, bc)) out.push(...tc)
    else if (sameLines(tc, bc) || sameLines(mc, tc)) out.push(...mc)
    else return { clean: false }
    i = ni; j = nj; k = nk
  }
  return { clean: true, text: out.join('\n') }
}

function sameLines(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((line, i) => line === b[i])
}

/** For each line of a, the index of its matched line in b (longest common
 *  subsequence), or -1. Matches are strictly increasing. */
export function matchMap(a: string[], b: string[]): number[] {
  const map = new Array<number>(a.length).fill(-1)
  // Trim the common prefix and suffix — most edits touch a small region,
  // and it keeps the Myers search short.
  let start = 0
  while (start < a.length && start < b.length && a[start] === b[start]) {
    map[start] = start
    start++
  }
  let endA = a.length, endB = b.length
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--; endB--
    map[endA] = endB
  }
  for (const [x, y] of myers(a.slice(start, endA), b.slice(start, endB))) map[start + x] = start + y
  return map
}

/** Matched index pairs of a shortest edit script (Myers, O((N+M)·D)). */
function myers(a: string[], b: string[]): Array<[number, number]> {
  const n = a.length, m = b.length
  if (n === 0 || m === 0) return []
  const max = n + m
  const offset = max
  const v = new Array<number>(2 * max + 2).fill(0)
  const trace: number[][] = []
  outer: for (let d = 0; d <= max; d++) {
    trace.push(v.slice())
    for (let kk = -d; kk <= d; kk += 2) {
      let x = kk === -d || (kk !== d && v[offset + kk - 1] < v[offset + kk + 1])
        ? v[offset + kk + 1]
        : v[offset + kk - 1] + 1
      let y = x - kk
      while (x < n && y < m && a[x] === b[y]) { x++; y++ }
      v[offset + kk] = x
      if (x >= n && y >= m) break outer
    }
  }
  // Walk the trace back from (n, m), collecting the diagonal (matched) moves.
  const pairs: Array<[number, number]> = []
  let x = n, y = m
  for (let d = trace.length - 1; d >= 0 && (x > 0 || y > 0); d--) {
    const vd = trace[d]
    const kk = x - y
    const prevK = kk === -d || (kk !== d && vd[offset + kk - 1] < vd[offset + kk + 1]) ? kk + 1 : kk - 1
    const prevX = d === 0 ? 0 : vd[offset + prevK]
    const prevY = prevX - prevK
    while (x > prevX && y > prevY) {
      x--; y--
      pairs.push([x, y])
    }
    if (d > 0) { x = prevX; y = prevY }
  }
  return pairs.reverse()
}
