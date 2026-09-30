// A slide media field's literal value is a LIST: image fields hold every
// gallery URL / attachment ref joined with '\n' (shared/slideBindings.ts
// multi=true; renderers show a carousel), video fields hold one. Editing
// that as a single-line string merged the URLs (inputs strip line breaks)
// and the attachment chip's ✕ wiped every image — so the editor works on
// the split list and re-joins on every change.

/** Split a stored media value into its items (blank lines dropped). */
export function splitMediaValue(value: string): string[] {
  return value.split('\n').map((s) => s.trim()).filter(Boolean)
}

/** Join edited items back into the stored value (blank items dropped). */
export function joinMediaValue(items: readonly string[]): string {
  return splitMediaValue(items.join('\n')).join('\n')
}

/** Pasted text → the items it names (one per non-blank line). */
export function pastedMediaItems(text: string): string[] {
  return splitMediaValue(text.replace(/\r\n?/g, '\n'))
}
