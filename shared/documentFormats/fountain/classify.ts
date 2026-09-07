import { splitLines } from '../text'

// Fountain (fountain.io) line classifier — the single source of the
// grammar for both the editor's highlighting and the screenplay renderer.
// Fountain is context-sensitive across lines (a character cue is an
// uppercase line that is preceded by a blank line AND followed by text),
// so classification is a pass over the whole document with a little
// state, not a per-line regex. Pure function; tested against the spec's
// own examples in frontend/src/lib/fountain.test.ts.

export type FountainLineKind =
  | 'title_page'
  | 'scene_heading'
  | 'action'
  | 'character'
  | 'parenthetical'
  | 'dialogue'
  | 'lyric'
  | 'transition'
  | 'centered'
  | 'page_break'
  | 'section'
  | 'synopsis'
  | 'note'
  | 'boneyard'
  | 'blank'

export interface FountainLine {
  /** 1-based. */
  line: number
  kind: FountainLineKind
  /** The line as written, leading whitespace included. */
  raw: string
  /** Element text with the forcing character / markers removed and trimmed. */
  text: string
  /** Document offset of the line start / end (end excludes the newline). */
  from: number
  to: number
  /** Character cue that shares the page with the previous one (`^`). */
  dual?: boolean
  /** Section depth (number of leading `#`). */
  level?: number
  /** Scene number from a trailing `#12A#`. */
  sceneNumber?: string
  /** Element was forced with a leading `.`, `!`, `@` or `>`. */
  forced?: boolean
}

export interface TitlePageEntry {
  key: string
  /** Multi-line values keep their line breaks. */
  value: string
}

export interface FountainDocument {
  titlePage: TitlePageEntry[]
  lines: FountainLine[]
}

const SCENE_HEADING = /^(?:INT|EXT|EST|INT\.?\/EXT|I\/E)[. ]/i
const SCENE_NUMBER = /\s*#([A-Za-z0-9.\-]+)#\s*$/
const TITLE_KEY = /^([A-Za-z][A-Za-z ]*?):[ \t]*(.*)$/
// A document is only a title page when it OPENS with one of the keys the
// spec names — "Note: he enters" at the top of a script is action.
const TITLE_START = /^(?:title|credit|authors?|source|notes?|draft date|date|contact|copyright|revision)\s*:/i
const CHARACTER_EXTENSION = /\([^)]*\)/g
const HAS_LETTER = /\p{L}/u

function isBlank(s: string): boolean {
  return s.trim() === ''
}

/** Uppercase with at least one letter, ignoring parenthesised extensions like (V.O.). */
function isCue(s: string): boolean {
  const bare = s.replace(CHARACTER_EXTENSION, '').replace(/\^\s*$/, '').trim()
  return bare !== '' && HAS_LETTER.test(bare) && bare === bare.toUpperCase()
}

/**
 * Marks every line that lies entirely inside a boneyard (/* … *\/) or a
 * note ([[ … ]]). Inline notes that share a line with other text leave the
 * line's kind alone; the renderer strips them.
 */
function commentLines(lines: string[]): Map<number, 'note' | 'boneyard'> {
  const out = new Map<number, 'note' | 'boneyard'>()
  let open: { kind: 'note' | 'boneyard'; close: string; startLine: number; whole: boolean } | null = null
  for (let i = 0; i < lines.length; i++) {
    const s = lines[i]
    let pos = 0
    while (pos <= s.length) {
      if (open) {
        const end = s.indexOf(open.close, pos)
        if (end === -1) {
          if (open.whole || i > open.startLine) out.set(i, open.kind)
          break
        }
        const rest = s.slice(end + open.close.length)
        if (i > open.startLine && isBlank(rest)) out.set(i, open.kind)
        else if (i === open.startLine && open.whole && isBlank(rest)) out.set(i, open.kind)
        pos = end + open.close.length
        open = null
        continue
      }
      const b = s.indexOf('/*', pos)
      const n = s.indexOf('[[', pos)
      const next = b === -1 ? n : n === -1 ? b : Math.min(b, n)
      if (next === -1) break
      const kind = next === b ? 'boneyard' : 'note'
      open = { kind, close: kind === 'boneyard' ? '*/' : ']]', startLine: i, whole: isBlank(s.slice(0, next)) }
      pos = next + 2
    }
  }
  return out
}

export function classifyFountain(text: string): FountainDocument {
  const rawLines = splitLines(text)
  const comments = commentLines(rawLines)
  const offsets: number[] = []
  let off = 0
  for (const l of rawLines) {
    offsets.push(off)
    off += l.length + 1
  }
  const result: FountainLine[] = []
  const titlePage: TitlePageEntry[] = []

  const make = (i: number, kind: FountainLineKind, textValue: string, extra: Partial<FountainLine> = {}): FountainLine => ({
    line: i + 1,
    kind,
    raw: rawLines[i],
    text: textValue,
    from: offsets[i],
    to: offsets[i] + rawLines[i].length,
    ...extra,
  })

  // Comment-only lines count as blank for adjacency: a note between two
  // elements must not glue them into one dialogue block.
  const effectivelyBlank = (i: number): boolean => i < 0 || i >= rawLines.length || comments.has(i) || isBlank(rawLines[i])

  let i = 0
  // Title page: key/value pairs from the top until the first blank line.
  if (rawLines.length > 0 && TITLE_START.test(rawLines[0])) {
    let current: TitlePageEntry | null = null
    for (; i < rawLines.length && !isBlank(rawLines[i]); i++) {
      const m = TITLE_KEY.exec(rawLines[i])
      if (m && !/^\s/.test(rawLines[i])) {
        current = { key: m[1].trim(), value: m[2].trim() }
        titlePage.push(current)
      } else if (current) {
        current.value = current.value ? `${current.value}\n${rawLines[i].trim()}` : rawLines[i].trim()
      }
      result.push(make(i, 'title_page', rawLines[i].trim()))
    }
  }

  let inDialogue = false
  for (; i < rawLines.length; i++) {
    const raw = rawLines[i]
    const comment = comments.get(i)
    if (comment) {
      result.push(make(i, comment, raw.trim()))
      inDialogue = false
      continue
    }
    if (isBlank(raw)) {
      // Two spaces on an otherwise empty line keep a dialogue block open.
      if (inDialogue && raw.length >= 2) {
        result.push(make(i, 'dialogue', ''))
      } else {
        result.push(make(i, 'blank', ''))
        inDialogue = false
      }
      continue
    }
    const s = raw.trim()
    const blankBefore = effectivelyBlank(i - 1)
    const blankAfter = effectivelyBlank(i + 1)

    if (/^={3,}\s*$/.test(s)) {
      result.push(make(i, 'page_break', ''))
      inDialogue = false
      continue
    }
    if (s.startsWith('#')) {
      const level = /^#+/.exec(s)![0].length
      result.push(make(i, 'section', s.slice(level).trim(), { level }))
      inDialogue = false
      continue
    }
    if (s.startsWith('=')) {
      result.push(make(i, 'synopsis', s.slice(1).trim()))
      inDialogue = false
      continue
    }
    if (s.startsWith('!')) {
      result.push(make(i, 'action', s.slice(1), { forced: true }))
      inDialogue = false
      continue
    }
    if (s.startsWith('@')) {
      result.push(make(i, 'character', s.slice(1).replace(/\^\s*$/, '').trim(), { forced: true, ...(/\^\s*$/.test(s) ? { dual: true } : {}) }))
      inDialogue = true
      continue
    }
    if (s.startsWith('~')) {
      result.push(make(i, 'lyric', s.slice(1).trim()))
      continue
    }
    if (s.startsWith('.') && !s.startsWith('..')) {
      const num = SCENE_NUMBER.exec(s)
      result.push(make(i, 'scene_heading', s.slice(1).replace(SCENE_NUMBER, '').trim(), { forced: true, ...(num ? { sceneNumber: num[1] } : {}) }))
      inDialogue = false
      continue
    }
    if (s.startsWith('>') && s.endsWith('<')) {
      result.push(make(i, 'centered', s.slice(1, -1).trim()))
      inDialogue = false
      continue
    }
    if (s.startsWith('>')) {
      result.push(make(i, 'transition', s.slice(1).trim(), { forced: true }))
      inDialogue = false
      continue
    }
    if (inDialogue) {
      if (s.startsWith('(') && s.endsWith(')')) result.push(make(i, 'parenthetical', s))
      else result.push(make(i, 'dialogue', s))
      continue
    }
    // The spec asks for a blank line after a scene heading too, but an
    // INT./EXT. line is a heading far more often than it is a character
    // named "INT. HOUSE" — and requiring the blank makes the line flicker
    // between cue and heading while it is being typed, or misread a
    // heading with a synopsis right under it.
    if (blankBefore && SCENE_HEADING.test(s)) {
      const num = SCENE_NUMBER.exec(s)
      result.push(make(i, 'scene_heading', s.replace(SCENE_NUMBER, '').trim(), num ? { sceneNumber: num[1] } : {}))
      continue
    }
    if (blankBefore && blankAfter && isCue(s) && /TO:$/.test(s)) {
      result.push(make(i, 'transition', s))
      continue
    }
    if (blankBefore && !blankAfter && isCue(s)) {
      result.push(make(i, 'character', s.replace(/\^\s*$/, '').trim(), /\^\s*$/.test(s) ? { dual: true } : {}))
      inDialogue = true
      continue
    }
    result.push(make(i, 'action', s))
  }

  return { titlePage, lines: result }
}

/** The bare character name: extension and dual marker removed, upper-cased. */
export function characterName(cue: string): string {
  return cue.replace(CHARACTER_EXTENSION, '').replace(/\^\s*$/, '').trim().toUpperCase()
}
