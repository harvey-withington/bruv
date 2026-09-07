import { EditorState, RangeSetBuilder, type Extension } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin, type DecorationSet, type ViewUpdate } from '@codemirror/view'
import { classifyFountain, type FountainLineKind } from '@shared/documentFormats/fountain'

// Fountain highlighting for CodeMirror: the shared classifier decides what
// every line is, and this plugin only dresses the lines up. One grammar,
// two consumers (this and the renderer) — the plan's rule for the format.
// Classification is a linear pass, cheap enough to redo on every change
// even for a feature-length script.

const lineDecorations: Record<FountainLineKind, Decoration | null> = {
  title_page: Decoration.line({ class: 'cm-fn-title-page' }),
  scene_heading: Decoration.line({ class: 'cm-fn-scene-heading' }),
  action: null,
  character: Decoration.line({ class: 'cm-fn-character' }),
  parenthetical: Decoration.line({ class: 'cm-fn-parenthetical' }),
  dialogue: Decoration.line({ class: 'cm-fn-dialogue' }),
  lyric: Decoration.line({ class: 'cm-fn-lyric' }),
  transition: Decoration.line({ class: 'cm-fn-transition' }),
  centered: Decoration.line({ class: 'cm-fn-centered' }),
  page_break: Decoration.line({ class: 'cm-fn-page-break' }),
  section: Decoration.line({ class: 'cm-fn-section' }),
  synopsis: Decoration.line({ class: 'cm-fn-synopsis' }),
  note: Decoration.line({ class: 'cm-fn-note' }),
  boneyard: Decoration.line({ class: 'cm-fn-boneyard' }),
  blank: null,
}

const inlineNote = Decoration.mark({ class: 'cm-fn-note-inline' })
const INLINE_NOTE = /\[\[[\s\S]*?\]\]/g

function decorate(state: EditorState): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  const doc = classifyFountain(state.doc.toString())
  for (const l of doc.lines) {
    const deco = lineDecorations[l.kind]
    if (deco) builder.add(l.from, l.from, deco)
    if (l.kind !== 'note' && l.kind !== 'boneyard') {
      for (const m of l.raw.matchAll(INLINE_NOTE)) {
        builder.add(l.from + m.index, l.from + m.index + m[0].length, inlineNote)
      }
    }
  }
  return builder.finish()
}

const highlighter = ViewPlugin.fromClass(class {
  decorations: DecorationSet
  constructor(view: EditorView) { this.decorations = decorate(view.state) }
  update(u: ViewUpdate) { if (u.docChanged) this.decorations = decorate(u.state) }
}, { decorations: v => v.decorations })

// Screenplay geometry in the source: Courier, cues/dialogue indented the
// way the page will read, so the writer sees the shape while typing.
const theme = EditorView.theme({
  '.cm-content': { fontFamily: '"Courier Prime", "Courier New", Courier, monospace', fontSize: '0.95rem' },
  '.cm-fn-title-page': { color: 'var(--text-secondary)' },
  '.cm-fn-scene-heading': { fontWeight: '700', textTransform: 'uppercase', paddingTop: '0.8em' },
  '.cm-fn-character': { paddingLeft: '13em', textTransform: 'uppercase' },
  '.cm-fn-parenthetical': { paddingLeft: '9.5em', color: 'var(--text-secondary)' },
  '.cm-fn-dialogue': { paddingLeft: '6em', paddingRight: '6em' },
  '.cm-fn-lyric': { paddingLeft: '6em', fontStyle: 'italic' },
  '.cm-fn-transition': { textAlign: 'right', textTransform: 'uppercase' },
  '.cm-fn-centered': { textAlign: 'center' },
  '.cm-fn-page-break': { color: 'var(--text-faint)', letterSpacing: '0.2em' },
  '.cm-fn-section': { color: 'var(--accent-light)', fontWeight: '700' },
  '.cm-fn-synopsis': { color: 'var(--text-muted)', fontStyle: 'italic' },
  '.cm-fn-note, .cm-fn-note-inline': { color: 'var(--text-muted)', fontStyle: 'italic' },
  '.cm-fn-boneyard': { color: 'var(--text-faint)' },
})

export const fountainExtensions: Extension = [
  highlighter,
  theme,
  EditorState.languageData.of(() => [{ closeBrackets: { brackets: ['(', '['] } }]),
]
