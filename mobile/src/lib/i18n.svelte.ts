// Mobile i18n — same shape as the desktop frontend's i18n module.
//
// Mobile keeps its own locale file (mobile-specific strings only)
// rather than sharing with the desktop bundle, since the surfaces
// are different and shipping all of desktop's strings to a phone
// would bloat the bundle for no benefit. Shared types and API live
// in /shared; user-facing copy stays per-app.
import en from './locales/en.json'

type Translations = Record<string, string>

const locales: Record<string, Translations> = { en }

export const i18n = $state({
  locale: 'en',
})

export function t(key: string, params?: Record<string, string | number>): string {
  const dict = locales[i18n.locale] || locales.en
  let str = dict[key] ?? key
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      str = str.replace(`{${k}}`, String(v))
    }
  }
  return str
}

export function setLocale(locale: string) {
  if (locales[locale]) {
    i18n.locale = locale
    localStorage.setItem('bruv:locale', locale)
  }
}

export function loadLocale() {
  const saved = localStorage.getItem('bruv:locale')
  if (saved && locales[saved]) {
    i18n.locale = saved
  }
}
