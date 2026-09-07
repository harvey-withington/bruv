<script lang="ts">
  import type { DocumentFormatId } from '@shared/documentFormats'
  import { codemirror, type CodeMirrorApi, type CursorPosition } from '../../../lib/editor/codemirror'
  import { languageExtensions } from '../../../lib/editor/languages'
  import { editorPhrases } from '../../../lib/editor/phrases'

  // The CodeMirror host. Owns no document state: text flows in as a prop
  // and out through onChange (see the codemirror action for the reload
  // semantics); a format change reconfigures the language in place.
  let { text, format, onChange, onCursor, onSave, onCommit, onReady }: {
    text: string
    format: DocumentFormatId
    onChange: (text: string) => void
    onCursor?: (pos: CursorPosition) => void
    /** Ctrl/Cmd+S. */
    onSave?: () => void
    /** Ctrl/Cmd+Enter — commit and close, per the keyboard contract. */
    onCommit?: () => void
    onReady?: (api: CodeMirrorApi) => void
  } = $props()

  const language = $derived(languageExtensions(format))
  const phrases = editorPhrases()
</script>

<div class="editor-pane" use:codemirror={{ doc: text, language, phrases, onChange, onCursor, onSave, onCommit, onReady }}></div>

<style>
  .editor-pane {
    height: 100%;
    min-height: 0;
    min-width: 0;
  }
  .editor-pane :global(.cm-editor) {
    height: 100%;
  }
</style>
