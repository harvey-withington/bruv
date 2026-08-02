// Small display formatters shared across components.

/**
 * formatBytes renders a byte count the way a person reads it: whole units
 * for big numbers, one decimal where the extra digit carries meaning.
 *
 * Used wherever BRUV asks the user to agree to a size — a template import,
 * the initial commit of a published workspace — so the number they weigh
 * the decision against is written the same way every time.
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  // Bytes and kilobytes are never interesting to a decimal place; from
  // megabytes up, "1.4 GB" says something "1 GB" doesn't.
  const decimals = unit >= 2 && value < 100 ? 1 : 0
  return `${value.toFixed(decimals)} ${units[unit]}`
}
