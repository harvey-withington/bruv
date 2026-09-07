import { t } from '../i18n.svelte'

// CodeMirror's own UI strings (search/replace panel, go-to-line) keyed by
// their English source text, as EditorState.phrases expects. "$" is
// CodeMirror's number placeholder — keep it in the translations.
export function editorPhrases(): Record<string, string> {
  return {
    'Find': t('document.cm.find'),
    'Replace': t('document.cm.replace'),
    'next': t('document.cm.next'),
    'previous': t('document.cm.previous'),
    'all': t('document.cm.all'),
    'match case': t('document.cm.match_case'),
    'by word': t('document.cm.by_word'),
    'regexp': t('document.cm.regexp'),
    'replace': t('document.cm.replace_one'),
    'replace all': t('document.cm.replace_all'),
    'close': t('document.cm.close'),
    'current match': t('document.cm.current_match'),
    'replaced $ matches': t('document.cm.replaced_matches'),
    'replaced match on line $': t('document.cm.replaced_on_line'),
    'on line': t('document.cm.on_line'),
    'Go to line': t('document.cm.go_to_line'),
    'go': t('document.cm.go'),
    'Control character': t('document.cm.control_character'),
  }
}
