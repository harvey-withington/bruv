import type { Attachment } from './types'
import { documentFormatForPath } from './documentFormats'

// Which attachments open in the document editor rather than a preview or
// a download. The format registry decides for the extensions it knows;
// a generic text mime is accepted too, so a `.log` or `.csv` still opens
// as plain text. Binary is refused server-side regardless (the open RPC
// checks UTF-8), so this only has to be a good first guess.
const TEXT_EXTENSIONS = ['.md', '.markdown', '.txt', '.fountain', '.json', '.yaml', '.yml', '.csv', '.xml', '.log']

export function isEditableTextAttachment(att: Pick<Attachment, 'name' | 'mime'>): boolean {
  const name = att.name.toLowerCase()
  const mime = (att.mime ?? '').toLowerCase()
  if (mime === 'text/html' || name.endsWith('.html') || name.endsWith('.htm')) return false
  if (documentFormatForPath(name).id !== 'plaintext') return true
  if (TEXT_EXTENSIONS.some(ext => name.endsWith(ext))) return true
  return mime.startsWith('text/') || mime === 'application/json' || mime === 'application/xml' || mime === 'application/yaml'
}
