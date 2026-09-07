import type { DocumentFormat } from './types'
import { escapeHtml } from './text'

// The fallback for any text file no module claims: the editor still edits
// it (that is what the workspace viewer always did), and the preview is
// the text as-is.
export const plaintextFormat: DocumentFormat = {
  id: 'plaintext',
  extensions: [],
  render: (text) => `<pre class="doc-plain">${escapeHtml(text)}</pre>`,
}
