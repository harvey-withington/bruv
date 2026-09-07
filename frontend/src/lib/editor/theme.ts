import { EditorView } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

// The editor's look, built entirely from BRUV's design tokens (style.css)
// so dark/light follow the app with no JS theme switching. Prose-first:
// the app font at a reading size, monospace only for code.

const MONO = '"Fira Code", "Consolas", monospace'

const chrome = EditorView.theme({
  '&': {
    color: 'var(--text-primary)',
    backgroundColor: 'var(--bg-base)',
    height: '100%',
    fontSize: '0.95rem',
  },
  '.cm-scroller': {
    fontFamily: 'inherit',
    lineHeight: '1.65',
    overflow: 'auto',
  },
  '.cm-content': {
    padding: '1rem 0',
    caretColor: 'var(--text-primary)',
  },
  '.cm-line': { padding: '0 1.25rem' },
  '&.cm-focused': { outline: 'none' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text-primary)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'var(--accent-glow-2)',
  },
  '.cm-activeLine': { backgroundColor: 'var(--bg-subtle)' },
  '.cm-selectionMatch': { backgroundColor: 'var(--accent-glow-3)' },
  '.cm-searchMatch': {
    backgroundColor: 'var(--warning-bg)',
    outline: '1px solid var(--warning-border)',
  },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--warning-border)' },
  '.cm-matchingBracket, .cm-nonmatchingBracket': { backgroundColor: 'var(--bg-subtle-hover)' },
  '.cm-panels': {
    backgroundColor: 'var(--bg-elevated)',
    color: 'var(--text-primary)',
    fontSize: '0.8rem',
  },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--border-muted)' },
  '.cm-panel.cm-search': { padding: '0.4rem 0.75rem' },
  '.cm-panel.cm-search label': { color: 'var(--text-secondary)' },
  '.cm-textfield': {
    backgroundColor: 'var(--bg-base)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border)',
    borderRadius: '4px',
    fontFamily: 'inherit',
  },
  '.cm-button': {
    backgroundImage: 'none',
    backgroundColor: 'var(--bg-base)',
    color: 'var(--text-secondary)',
    border: '1px solid var(--border)',
    borderRadius: '4px',
    fontFamily: 'inherit',
    cursor: 'pointer',
  },
  '.cm-button:active': { backgroundImage: 'none', backgroundColor: 'var(--bg-subtle-hover)' },
  '.cm-panel button[name="close"]': { color: 'var(--text-muted)', cursor: 'pointer' },
  '.cm-tooltip': {
    backgroundColor: 'var(--bg-elevated)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border)',
    borderRadius: '6px',
  },
  '.cm-lintRange-error': { backgroundImage: 'none', textDecoration: 'underline wavy var(--danger)' },
  '.cm-lintRange-warning': { backgroundImage: 'none', textDecoration: 'underline wavy var(--warning)' },
  '.cm-lintRange-info': { backgroundImage: 'none', textDecoration: 'underline dotted var(--info)' },
})

// Markdown source styling: headings read as headings, markup characters
// fade, code goes monospace. Fountain adds its own decorations on top.
const highlight = HighlightStyle.define([
  { tag: tags.heading1, fontSize: '1.45em', fontWeight: '700', color: 'var(--text-primary)' },
  { tag: tags.heading2, fontSize: '1.25em', fontWeight: '700', color: 'var(--text-primary)' },
  { tag: tags.heading3, fontSize: '1.1em', fontWeight: '600', color: 'var(--text-primary)' },
  { tag: [tags.heading4, tags.heading5, tags.heading6], fontWeight: '600', color: 'var(--text-primary)' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strong, fontWeight: '700' },
  { tag: tags.strikethrough, textDecoration: 'line-through', color: 'var(--text-muted)' },
  { tag: tags.link, color: 'var(--link)', textDecoration: 'underline' },
  { tag: tags.url, color: 'var(--link)' },
  { tag: tags.monospace, fontFamily: MONO, fontSize: '0.9em', color: 'var(--accent-light)' },
  { tag: tags.quote, color: 'var(--text-secondary)', fontStyle: 'italic' },
  { tag: tags.list, color: 'var(--accent-light)' },
  { tag: tags.contentSeparator, color: 'var(--text-faint)' },
  { tag: tags.processingInstruction, color: 'var(--text-faint)' },
  { tag: tags.meta, color: 'var(--text-muted)' },
  { tag: tags.comment, color: 'var(--text-faint)', fontStyle: 'italic' },
  { tag: tags.keyword, color: 'var(--accent-light)' },
  { tag: tags.string, color: 'var(--success)' },
  { tag: tags.escape, color: 'var(--text-muted)' },
  { tag: tags.labelName, color: 'var(--info)' },
])

export const bruvEditorTheme: Extension = [chrome, syntaxHighlighting(highlight)]
