import { describe, it, expect } from 'vitest'
import { isoToDateInput, isoToLocalInput, localInputToIso } from '@shared/dateTimeInput'

// Regression: the agent config posted the raw datetime-local value
// ("2026-09-27T14:30") as start_date, which Go's time.Time rejects, so
// Save failed whenever a time was picked. Assertions are written to hold
// in any host timezone.

describe('localInputToIso', () => {
  it('emits RFC 3339 with a zone for a datetime-local value', () => {
    const iso = localInputToIso('2026-09-27T14:30')
    expect(iso).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/)
    expect(new Date(iso!).getTime()).toBe(new Date(2026, 8, 27, 14, 30).getTime())
  })

  it('returns null for empty or unparseable input', () => {
    expect(localInputToIso('')).toBeNull()
    expect(localInputToIso(null)).toBeNull()
    expect(localInputToIso(undefined)).toBeNull()
    expect(localInputToIso('not a date')).toBeNull()
  })
})

describe('isoToLocalInput', () => {
  it('renders local wall-clock time without seconds or zone', () => {
    const iso = new Date(2026, 0, 5, 9, 7, 42).toISOString()
    expect(isoToLocalInput(iso)).toBe('2026-01-05T09:07')
  })

  it('returns an empty string for empty or unparseable input', () => {
    expect(isoToLocalInput('')).toBe('')
    expect(isoToLocalInput(null)).toBe('')
    expect(isoToLocalInput(undefined)).toBe('')
    expect(isoToLocalInput('garbage')).toBe('')
  })

  it('round-trips through localInputToIso', () => {
    const local = '2026-12-31T23:59'
    expect(isoToLocalInput(localInputToIso(local))).toBe(local)
  })
})

describe('isoToDateInput', () => {
  it('keeps the written calendar date, even for offset timestamps', () => {
    expect(isoToDateInput('2026-09-27')).toBe('2026-09-27')
    expect(isoToDateInput('2026-09-27T00:30:00+10:00')).toBe('2026-09-27')
  })

  it('returns an empty string for empty or unparseable input', () => {
    expect(isoToDateInput('')).toBe('')
    expect(isoToDateInput(null)).toBe('')
    expect(isoToDateInput('garbage')).toBe('')
  })
})
