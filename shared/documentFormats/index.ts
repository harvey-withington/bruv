import type { DocumentFormat, DocumentFormatId } from './types'
import { markdownFormat } from './markdown'
import { fountainFormat } from './fountain'
import { plaintextFormat } from './plaintext'

export type * from './types'
export { countWords } from './text'

// Compile-time registry: no dynamic loading, no marketplace. Adding a
// format = adding a module here (plus a language extension in
// frontend/src/lib/editor/languages.ts when it highlights).
const FORMATS: readonly DocumentFormat[] = [markdownFormat, fountainFormat, plaintextFormat]

const byExtension = new Map<string, DocumentFormat>()
for (const f of FORMATS) {
  for (const ext of f.extensions) byExtension.set(ext, f)
}

export function documentFormats(): readonly DocumentFormat[] {
  return FORMATS
}

export function documentFormatById(id: DocumentFormatId): DocumentFormat {
  return FORMATS.find(f => f.id === id) ?? plaintextFormat
}

/** The module for a file path, by extension; plain text when none claims it. */
export function documentFormatForPath(path: string): DocumentFormat {
  const name = path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1)
  const dot = name.lastIndexOf('.')
  if (dot <= 0) return plaintextFormat
  return byExtension.get(name.slice(dot).toLowerCase()) ?? plaintextFormat
}

/** File name without directories — the editor's title and render context. */
export function documentName(path: string): string {
  return path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1)
}
