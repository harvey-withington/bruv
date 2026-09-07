import { escapeHtml } from '../text'
import { classifyFountain, type FountainDocument, type FountainLine } from './classify'

// Screenplay HTML from the classifier's line stream. Layout (Courier,
// indents, dual-dialogue columns, page breaks) is all CSS — see styles.ts —
// so the same markup prints through the webview's paged media.

/** Inline emphasis: ***bold italic***, **bold**, *italic*, _underline_; notes muted; boneyard gone. */
export function renderFountainInline(text: string): string {
  let s = escapeHtml(text)
  // Boneyard fragments within a line disappear; notes stay visible on
  // screen (muted) and are hidden in print by the stylesheet.
  s = s.replace(/\/\*[\s\S]*?\*\//g, '')
  s = s.replace(/\[\[([\s\S]*?)\]\]/g, '<span class="fn-note">$1</span>')
  // Escaped markers first so they never pair.
  s = s.replace(/\\\*/g, '&#42;').replace(/\\_/g, '&#95;')
  s = s.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
  s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  s = s.replace(/\*(.+?)\*/g, '<em>$1</em>')
  s = s.replace(/_(.+?)_/g, '<u>$1</u>')
  return s
}

const TITLE_CENTER = new Set(['title', 'credit', 'author', 'authors', 'source'])

function renderTitlePage(doc: FountainDocument): string {
  if (doc.titlePage.length === 0) return ''
  const centered: string[] = []
  const corner: string[] = []
  for (const e of doc.titlePage) {
    const key = e.key.toLowerCase()
    const value = e.value.split('\n').map(renderFountainInline).join('<br>')
    const cls = `fn-tp-${key.replace(/[^a-z]+/g, '-')}`
    if (TITLE_CENTER.has(key)) centered.push(`<p class="fn-tp ${cls}">${value}</p>`)
    else corner.push(`<p class="fn-tp ${cls}">${value}</p>`)
  }
  return `<section class="fn-title-page"><div class="fn-tp-center">${centered.join('')}</div><div class="fn-tp-corner">${corner.join('')}</div></section>`
}

interface DialogueBlock {
  kind: 'dialogue'
  character: string
  dual: boolean
  parts: { kind: 'parenthetical' | 'dialogue' | 'lyric'; text: string }[]
}
interface SimpleBlock {
  kind: 'action' | 'scene_heading' | 'transition' | 'centered' | 'page_break' | 'section' | 'synopsis' | 'note' | 'lyric'
  lines: FountainLine[]
}
type Block = DialogueBlock | SimpleBlock

/** Groups lines into renderable blocks: consecutive action lines, one dialogue exchange, … */
function blocks(doc: FountainDocument): Block[] {
  const out: Block[] = []
  let current: Block | null = null
  for (const l of doc.lines) {
    switch (l.kind) {
      case 'title_page':
      case 'boneyard':
        break
      case 'blank':
        current = null
        break
      case 'character':
        current = { kind: 'dialogue', character: l.text, dual: l.dual === true, parts: [] }
        out.push(current)
        break
      case 'parenthetical':
      case 'dialogue':
        if (current?.kind === 'dialogue') current.parts.push({ kind: l.kind, text: l.text })
        else out.push(current = { kind: 'action', lines: [l] })
        break
      case 'lyric':
        if (current?.kind === 'dialogue') current.parts.push({ kind: 'lyric', text: l.text })
        else out.push(current = { kind: 'lyric', lines: [l] })
        break
      case 'action':
        if (current?.kind === 'action') current.lines.push(l)
        else out.push(current = { kind: 'action', lines: [l] })
        break
      case 'note':
        if (current?.kind === 'note') current.lines.push(l)
        else out.push(current = { kind: 'note', lines: [l] })
        break
      default:
        out.push(current = { kind: l.kind, lines: [l] })
        current = null
    }
  }
  return out
}

function renderDialogue(b: DialogueBlock): string {
  const parts = b.parts.map(p => {
    if (p.kind === 'parenthetical') return `<p class="fn-parenthetical">${renderFountainInline(p.text)}</p>`
    if (p.kind === 'lyric') return `<p class="fn-dialogue fn-lyric">${renderFountainInline(p.text)}</p>`
    return `<p class="fn-dialogue">${p.text === '' ? '&nbsp;' : renderFountainInline(p.text)}</p>`
  }).join('')
  return `<div class="fn-dialogue-block"><p class="fn-character">${renderFountainInline(b.character)}</p>${parts}</div>`
}

function renderBlock(b: Block): string {
  switch (b.kind) {
    case 'dialogue':
      return renderDialogue(b)
    case 'action':
      return `<p class="fn-action">${b.lines.map(l => renderFountainInline(l.forced ? l.raw.replace(/^\s*!/, '') : l.raw.trimEnd())).join('<br>')}</p>`
    case 'scene_heading': {
      const l = b.lines[0]
      const num = l.sceneNumber ? `<span class="fn-scene-number">${escapeHtml(l.sceneNumber)}</span>` : ''
      return `<p class="fn-scene-heading">${renderFountainInline(l.text)}${num}</p>`
    }
    case 'transition':
      return `<p class="fn-transition">${renderFountainInline(b.lines[0].text)}</p>`
    case 'centered':
      return `<p class="fn-centered">${renderFountainInline(b.lines[0].text)}</p>`
    case 'lyric':
      return `<p class="fn-lyric">${b.lines.map(l => renderFountainInline(l.text)).join('<br>')}</p>`
    case 'page_break':
      return '<hr class="fn-page-break">'
    case 'section':
      return `<p class="fn-section fn-section-${Math.min(b.lines[0].level ?? 1, 6)}">${renderFountainInline(b.lines[0].text)}</p>`
    case 'synopsis':
      return `<p class="fn-synopsis">${renderFountainInline(b.lines[0].text)}</p>`
    case 'note':
      return `<p class="fn-note-block">${b.lines.map(l => renderFountainInline(l.text.replace(/^\[\[|\]\]$/g, ''))).join('<br>')}</p>`
  }
}

export function renderFountain(text: string): string {
  const doc = classifyFountain(text)
  const body: string[] = []
  const list = blocks(doc)
  for (let i = 0; i < list.length; i++) {
    const b = list[i]
    // Dual dialogue: the cue marked ^ shares the page with the exchange
    // right before it.
    const next = list[i + 1]
    if (b.kind === 'dialogue' && next?.kind === 'dialogue' && next.dual) {
      body.push(`<div class="fn-dual">${renderDialogue(b)}${renderDialogue(next)}</div>`)
      i++
      continue
    }
    body.push(renderBlock(b))
  }
  return `${renderTitlePage(doc)}<section class="fn-script">${body.join('')}</section>`
}
