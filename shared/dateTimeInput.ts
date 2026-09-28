// Conversions between stored timestamps and <input type="datetime-local">.
//
// The input speaks "YYYY-MM-DDTHH:MM" in LOCAL wall-clock time with no
// seconds and no zone. Storage (and Go's time.Time JSON decoding) wants
// RFC 3339 / ISO 8601 with a zone. Sending the raw input value to the
// backend fails to unmarshal, and rendering an ISO string via
// toISOString().slice(0, 16) shows UTC instead of local — so every
// datetime-local binding goes through these two helpers.

const pad = (n: number, width = 2) => String(n).padStart(width, '0')

/** ISO / RFC 3339 timestamp → datetime-local value (local time). '' when empty or unparseable. */
export function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${pad(d.getFullYear(), 4)}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** datetime-local value (local time) → ISO timestamp in UTC. null when empty or unparseable. */
export function localInputToIso(local: string | null | undefined): string | null {
  if (!local) return null
  const d = new Date(local)
  return Number.isNaN(d.getTime()) ? null : d.toISOString()
}

/**
 * Stored date or timestamp → <input type="date"> value ("YYYY-MM-DD").
 * A value that already starts with a calendar date keeps that date as
 * written — reparsing would shift it across midnight for offset
 * timestamps. '' when empty or unparseable.
 */
export function isoToDateInput(iso: string | null | undefined): string {
  if (!iso) return ''
  if (/^\d{4}-\d{2}-\d{2}/.test(iso)) return iso.slice(0, 10)
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${pad(d.getFullYear(), 4)}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
