// Document format modules — the compile-time registry behind the workspace
// document editor (plan: "2026-09-06 document formats"). A module teaches
// the editor one plain-text format that renders into something else:
// Markdown, Fountain screenplays, the Manuscript profile.
//
// Everything here is transport- and DOM-agnostic on purpose: modules are
// pure functions over the document text so they run in Vitest, on the
// mobile surface (read-only preview later), and inside the desktop editor
// alike. The one desktop-only piece — the CodeMirror language extension —
// lives in frontend/src/lib/editor/languages.ts keyed by format id, so this
// package never pulls CodeMirror into shared/ or mobile/.

export type DocumentFormatId = 'markdown' | 'fountain' | 'plaintext'

/** One entry of the structure pane: a heading, scene, chapter… */
export interface OutlineNode {
  /** Stable within one outline pass — the line number is unique per node. */
  id: string
  /** Nesting depth, 1 = top level. */
  level: number
  title: string
  /** 1-based line the node starts on — what the editor scrolls to. */
  line: number
  /** Optional secondary text (a Fountain synopsis, a chapter word count). */
  description?: string
}

export type DiagnosticSeverity = 'error' | 'warning' | 'info'

/** A format-validation finding, in document character offsets. */
export interface Diagnostic {
  from: number
  to: number
  severity: DiagnosticSeverity
  message: string
  /** Rule id, for "why is this flagged" and for per-rule tests. */
  rule: string
}

/** Something the board could hold a card for: a character, a location… */
export interface Entity {
  kind: 'character' | 'location'
  name: string
  /** Occurrences in the text (Fountain: dialogue blocks for a character). */
  count: number
}

/** Per-render inputs a module may need beyond the text. */
export interface RenderContext {
  /** File name of the document being rendered (title fallback for Fountain). */
  name: string
}

export interface DocumentFormat {
  id: DocumentFormatId
  /** Lower-case extensions including the dot: ['.md', '.markdown']. */
  extensions: string[]
  /**
   * HTML for the preview pane. Must be safe to inject: escape any text
   * that did not go through a Markdown engine.
   */
  render: (text: string, ctx: RenderContext) => string
  /** CSS applied around the preview (screen + print). Scope selectors under `.doc-preview`. */
  styles?: string
  /** Headings / scenes / chapters for the outline pane. */
  outline?: (text: string) => OutlineNode[]
  /** Format validation; the editor shows these in the lint gutter + problems list. */
  lint?: (text: string) => Diagnostic[]
  /** Characters / locations the text mentions — the seed for "as cards". */
  entities?: (text: string) => Entity[]
  /** Word count for the status bar. Defaults to counting every word in the text. */
  wordCount?: (text: string) => number
  /** Whether the preview is worth printing as-is (Fountain: yes; plain text: no). */
  printable?: boolean
}
