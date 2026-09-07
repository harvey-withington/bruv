import type { DocumentFormat, Entity, OutlineNode } from '../types'
import { countWords } from '../text'
import { classifyFountain, characterName } from './classify'
import { renderFountain } from './render'
import { fountainStyles } from './styles'

export { classifyFountain, characterName, type FountainDocument, type FountainLine, type FountainLineKind } from './classify'
export { renderFountain, renderFountainInline } from './render'

/** Sections (by depth) and scene headings, with the synopsis that follows each as its description. */
export function fountainOutline(text: string): OutlineNode[] {
  const out: OutlineNode[] = []
  let sectionLevel = 0
  for (const l of classifyFountain(text).lines) {
    if (l.kind === 'section') {
      sectionLevel = l.level ?? 1
      out.push({ id: String(l.line), level: sectionLevel, title: l.text, line: l.line })
    } else if (l.kind === 'scene_heading') {
      const title = l.sceneNumber ? `${l.sceneNumber}. ${l.text}` : l.text
      out.push({ id: String(l.line), level: sectionLevel + 1, title, line: l.line })
    } else if (l.kind === 'synopsis' && out.length > 0 && out[out.length - 1].description === undefined) {
      out[out.length - 1].description = l.text
    }
  }
  return out
}

/** Every character cue, deduplicated by bare name, counted by dialogue blocks. */
export function fountainEntities(text: string): Entity[] {
  const counts = new Map<string, number>()
  for (const l of classifyFountain(text).lines) {
    if (l.kind !== 'character') continue
    const name = characterName(l.text)
    if (name) counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([name, count]) => ({ kind: 'character', name, count }))
}

const COUNTED = new Set(['scene_heading', 'action', 'character', 'parenthetical', 'dialogue', 'lyric', 'transition', 'centered'])

/** Words the reader sees: no title page, sections, synopses, notes or boneyard. */
export function fountainWordCount(text: string): number {
  let n = 0
  for (const l of classifyFountain(text).lines) {
    if (COUNTED.has(l.kind)) n += countWords(l.text)
  }
  return n
}

export const fountainFormat: DocumentFormat = {
  id: 'fountain',
  extensions: ['.fountain', '.spmd'],
  render: renderFountain,
  styles: fountainStyles,
  outline: fountainOutline,
  entities: fountainEntities,
  wordCount: fountainWordCount,
  printable: true,
}
