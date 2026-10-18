// DOM helpers for client-side file downloads. Shared by desktop and
// mobile; no backend coupling.

export function downloadBlob(content: string, filename: string, mime: string): void {
  // Blob + anchor download — works in both browser and Wails shell
  // without needing a backend file-write RPC. The native save dialog
  // (where supported) is driven by the `download` attribute.
  const url = URL.createObjectURL(new Blob([content], { type: mime }))
  saveURL(url, filename)
  // Revoke after a tick so the browser has time to start the download.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Save what `url` points at under `filename` (anchor + download attribute). */
export function saveURL(url: string, filename: string): void {
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
}

/**
 * Open an attachment URL for the user. A signed server URL opens in a new
 * tab / the system viewer; a device copy (a blob: URL, offline) can't be
 * opened in another window reliably — it's saved instead, which every
 * platform hands to its viewer (iOS Quick Look, Android's downloads).
 */
export function openAttachmentURL(url: string, filename: string): void {
  if (url.startsWith('blob:')) saveURL(url, filename)
  else window.open(url, '_blank', 'noopener,noreferrer')
}

/** A signed attachment path is server-relative; offline it's a blob: URL. */
export function absoluteAttachmentURL(serverBase: string, path: string): string {
  return path.startsWith('/') ? `${serverBase}${path}` : path
}

export function sanitizeFilenameStem(title: string | undefined | null): string {
  const base = (title?.trim() || 'card')
    .replace(/[\\/:*?"<>|]/g, '-')   // strip filesystem-unsafe chars
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
  return base || 'card'
}
