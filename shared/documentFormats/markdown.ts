import { renderMarkdown } from '../markdown'
import type { DocumentFormat, OutlineNode } from './types'
import { splitLines, stripInlineMarkdown } from './text'

// Markdown — the baseline module. Rendering is the app's existing engine
// (shared/markdown.ts), so a workspace .md previews exactly like a card's
// text block, bruv: and workspace:// links included. The outline is a
// line scanner rather than a parse tree so it stays shared-safe and
// cheap; it understands the two things that matter: fenced code (a "#"
// inside a fence is not a heading) and both heading syntaxes.

const ATX = /^ {0,3}(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/
const ATX_EMPTY = /^ {0,3}#{1,6}[ \t]*$/
const FENCE = /^ {0,3}(`{3,}|~{3,})/
const SETEXT_H1 = /^ {0,3}=+[ \t]*$/
const SETEXT_H2 = /^ {0,3}-+[ \t]*$/
const BLANK = /^[ \t]*$/
const LIST_OR_QUOTE = /^ {0,3}(?:[-*+]|\d+[.)])[ \t]|^ {0,3}>/

export function markdownOutline(text: string): OutlineNode[] {
  const out: OutlineNode[] = []
  const lines = splitLines(text)
  let fence: string | null = null
  // The previous line when it could be a setext heading's text.
  let paragraphLine: string | null = null
  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i]
    const fenceMatch = FENCE.exec(raw)
    if (fence !== null) {
      if (fenceMatch && fenceMatch[1][0] === fence[0] && fenceMatch[1].length >= fence.length) fence = null
      paragraphLine = null
      continue
    }
    if (fenceMatch) {
      fence = fenceMatch[1]
      paragraphLine = null
      continue
    }
    const atx = ATX.exec(raw)
    if (atx || ATX_EMPTY.test(raw)) {
      const title = atx ? stripInlineMarkdown(atx[2]) : ''
      if (title) out.push({ id: String(i + 1), level: atx![1].length, title, line: i + 1 })
      paragraphLine = null
      continue
    }
    // Setext: a paragraph line followed by === (h1) or --- (h2). Only the
    // single preceding line is considered — enough for headings, and it
    // keeps a "---" rule under a list item from becoming a heading.
    if (paragraphLine !== null && (SETEXT_H1.test(raw) || SETEXT_H2.test(raw))) {
      out.push({ id: String(i), level: SETEXT_H1.test(raw) ? 1 : 2, title: stripInlineMarkdown(paragraphLine), line: i })
      paragraphLine = null
      continue
    }
    paragraphLine = BLANK.test(raw) || LIST_OR_QUOTE.test(raw) ? null : raw
  }
  return out
}

export const markdownFormat: DocumentFormat = {
  id: 'markdown',
  extensions: ['.md', '.markdown', '.mdown', '.mkd'],
  render: (text) => renderMarkdown(text),
  outline: markdownOutline,
}
