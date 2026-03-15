// Lightweight i18n system with reactive locale
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
    localStorage.setItem('bruv-locale', locale)
  }
}

export function loadLocale() {
  const saved = localStorage.getItem('bruv-locale')
  if (saved && locales[saved]) {
    i18n.locale = saved
  }
}

export function registerLocale(code: string, translations: Translations) {
  locales[code] = translations
}

export function availableLocales(): string[] {
  return Object.keys(locales)
}
