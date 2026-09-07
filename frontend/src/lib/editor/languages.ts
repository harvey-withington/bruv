import { EditorState, type Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import type { DocumentFormatId } from '@shared/documentFormats'
import { fountainExtensions } from './fountain'

// The desktop half of the format registry: one CodeMirror language /
// behaviour bundle per format id. Kept out of shared/ so the pure modules
// there never depend on CodeMirror (mobile only ever needs render +
// outline).

const prose: Extension = [
  // Writers want the browser's spellcheck on prose; code editors don't.
  EditorView.contentAttributes.of({ spellcheck: 'true', autocorrect: 'off', autocapitalize: 'off' }),
  // Quotes are left out of auto-pairing everywhere: an apostrophe
  // mid-sentence must never spawn a closing quote.
  EditorState.languageData.of(() => [{ closeBrackets: { brackets: ['(', '[', '{'] } }]),
]

// markdownLanguage is GFM (tables, task lists, strikethrough) — the same
// dialect shared/markdown.ts renders.
const markdownExtensions: Extension = [
  markdown({ base: markdownLanguage, addKeymap: true }),
  markdownLanguage.data.of({ closeBrackets: { brackets: ['(', '[', '{', '`'] } }),
  prose,
]

const fountainBundle: Extension = [fountainExtensions, prose]

// Returns the same reference per format: the codemirror action
// reconfigures the language only when this identity changes.
export function languageExtensions(format: DocumentFormatId): Extension {
  switch (format) {
    case 'markdown': return markdownExtensions
    case 'fountain': return fountainBundle
    default: return prose
  }
}
