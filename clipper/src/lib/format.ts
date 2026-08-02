// Presentation helpers shared by the background (note text) and the in-page
// capture dialog (option labels), so a size reads the same wherever it
// appears.

const MB = 1024 * 1024
const GB = 1024 * MB

// formatBytes renders a size the way the capture dialog states it: whole
// MB up to 1024 MB, then GB with one decimal. Sizes here are estimates
// (bitrate × duration), so more precision would be a lie.
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return ''
  if (bytes < GB) return `${Math.max(1, Math.round(bytes / MB))} MB`
  return `${(bytes / GB).toFixed(1)} GB`
}
