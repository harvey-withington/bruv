// Shared relative-time formatter ("just now", "5m ago", "in 2h", ...).
//
// This lives in shared/ so both the desktop frontend and the mobile PWA can
// use one implementation instead of hand-rolling the same diff/threshold
// logic. shared/ modules can't import either surface's i18n module directly
// (that would create a dependency the wrong way round), so the translate
// function is passed in by the caller instead.
export type TranslateFn = (key: string, params?: Record<string, string | number>) => string

/**
 * Formats an ISO timestamp relative to now, e.g. "just now", "5m ago",
 * "in 2h". Handles both past and future timestamps.
 *
 * Expects these keys to exist in the caller's locale dictionary:
 *   time.just_now, time.soon,
 *   time.minutes_ago, time.hours_ago, time.days_ago,
 *   time.in_minutes, time.in_hours, time.in_days
 */
export function formatRelativeTime(iso: string, t: TranslateFn): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''

  const diff = Date.now() - d.getTime()
  const mins = Math.floor(Math.abs(diff) / 60000)
  const future = diff < 0

  if (mins < 1) return future ? t('time.soon') : t('time.just_now')
  if (mins < 60) return future ? t('time.in_minutes', { n: mins }) : t('time.minutes_ago', { n: mins })

  const hours = Math.floor(mins / 60)
  if (hours < 24) return future ? t('time.in_hours', { n: hours }) : t('time.hours_ago', { n: hours })

  const days = Math.floor(hours / 24)
  return future ? t('time.in_days', { n: days }) : t('time.days_ago', { n: days })
}
