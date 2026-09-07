import { Compartment, EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap, drawSelection, dropCursor, highlightActiveLine, rectangularSelection, crosshairCursor } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { bracketMatching, indentOnInput } from '@codemirror/language'
import { search, searchKeymap, highlightSelectionMatches } from '@codemirror/search'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import type { ActionReturn } from 'svelte/action'
import { bruvEditorTheme } from './theme'

export interface CursorPosition {
  /** 1-based. */
  line: number
  /** 1-based. */
  col: number
}

/** What the host gets back once the view exists — navigation without owning the view. */
export interface CodeMirrorApi {
  goToLine(line: number): void
  focus(): void
}

export interface CodeMirrorOptions {
  /** The document. Changing it to something the editor did not emit replaces the content (a reload). */
  doc: string
  /** Language + per-format behaviour (see languages.ts). */
  language: Extension
  /** CodeMirror UI strings (search panel…) — localized by the caller. */
  phrases: Record<string, string>
  onChange: (doc: string) => void
  onCursor?: (pos: CursorPosition) => void
  /** Ctrl/Cmd+S. */
  onSave?: () => void
  /**
   * Ctrl/Cmd+Enter — the keyboard contract's commit-and-close chord. Bound
   * here because CodeMirror's default keymap would otherwise take it
   * (insertBlankLine) and the host would never see it.
   */
  onCommit?: () => void
  onReady?: (api: CodeMirrorApi) => void
}

/**
 * Svelte action hosting a CodeMirror 6 view in `node`.
 *
 *   <div use:codemirror={{ doc, language, phrases, onChange }}></div>
 *
 * The host owns the text: every edit is reported through `onChange`, and a
 * `doc` that differs from the last emitted value (a reload from disk) is
 * dispatched into the view, cursor kept where possible. Comparing against
 * the last emitted string, not the view's, keeps the per-keystroke update
 * O(1) for the common case (same string instance).
 */
export function codemirror(node: HTMLElement, options: CodeMirrorOptions): ActionReturn<CodeMirrorOptions> {
  let opts = options
  let lastEmitted = opts.doc
  const language = new Compartment()

  const view = new EditorView({
    parent: node,
    state: EditorState.create({
      doc: opts.doc,
      extensions: [
        EditorState.phrases.of(opts.phrases),
        history(),
        drawSelection(),
        dropCursor(),
        rectangularSelection(),
        crosshairCursor(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        bracketMatching(),
        closeBrackets(),
        indentOnInput(),
        search({ top: true }),
        EditorView.lineWrapping,
        bruvEditorTheme,
        keymap.of([
          { key: 'Mod-s', run: () => { opts.onSave?.(); return true } },
          { key: 'Mod-Enter', run: () => { opts.onCommit?.(); return true } },
          ...closeBracketsKeymap,
          ...searchKeymap,
          ...historyKeymap,
          ...defaultKeymap,
          indentWithTab,
        ]),
        language.of(opts.language),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            lastEmitted = update.state.doc.toString()
            opts.onChange(lastEmitted)
          }
          if (update.selectionSet || update.docChanged) {
            const head = update.state.selection.main.head
            const line = update.state.doc.lineAt(head)
            opts.onCursor?.({ line: line.number, col: head - line.from + 1 })
          }
        }),
      ],
    }),
  })

  const api: CodeMirrorApi = {
    goToLine(line) {
      const n = Math.max(1, Math.min(line, view.state.doc.lines))
      const pos = view.state.doc.line(n).from
      view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: 'start', yMargin: 24 }) })
      view.focus()
    },
    focus() { view.focus() },
  }
  opts.onReady?.(api)
  view.focus()

  return {
    update(next) {
      const prevLanguage = opts.language
      opts = next
      if (next.language !== prevLanguage) {
        view.dispatch({ effects: language.reconfigure(next.language) })
      }
      if (next.doc !== lastEmitted) {
        lastEmitted = next.doc
        const head = Math.min(view.state.selection.main.head, next.doc.length)
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: next.doc },
          selection: { anchor: head },
        })
      }
    },
    destroy() {
      view.destroy()
    },
  }
}
