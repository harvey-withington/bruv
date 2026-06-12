// DOM helpers for client-side file downloads. Shared by desktop and
// mobile; no backend coupling.

export function downloadBlob(content: string, filename: string, mime: string): void {
  // Blob + anchor download — works in both browser and Wails shell
  // without needing a backend file-write RPC. The native save dialog
  // (where supported) is driven by the `download` attribute.
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Revoke after a tick so the browser has time to start the download.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function sanitizeFilenameStem(title: string | undefined | null): string {
  const base = (title?.trim() || 'card')
    .replace(/[\\/:*?"<>|]/g, '-')   // strip filesystem-unsafe chars
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
  return base || 'card'
}
