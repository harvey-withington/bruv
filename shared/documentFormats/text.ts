// Small text utilities every format module needs. Pure, no DOM.

const WORD = /[\p{L}\p{N}]/u

/**
 * Counts words the way a writer counts them: whitespace-separated tokens
 * that contain at least one letter or digit, so "—", "***" and lone
 * punctuation don't inflate a manuscript's count.
 */
export function countWords(text: string): number {
  if (!text) return 0
  let n = 0
  for (const token of text.split(/\s+/)) {
    if (WORD.test(token)) n++
  }
  return n
}

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * Reduces inline Markdown to its visible text for outline titles:
 * "**Bold** [link](url) `code`" → "Bold link code".
 */
export function stripInlineMarkdown(text: string): string {
  return text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/(\*\*|__)(.*?)\1/g, '$2')
    .replace(/(\*|_)(.*?)\1/g, '$2')
    .replace(/~~(.*?)~~/g, '$1')
    .trim()
}

/** Splits on any line ending; index + 1 is the 1-based line number. */
export function splitLines(text: string): string[] {
  return text.split(/\r\n|\r|\n/)
}
