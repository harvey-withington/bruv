// Mobile theme switcher. Three states: 'light' / 'dark' / 'auto'.
// Default 'auto' so a fresh install follows the phone's system theme.
//
// Implementation: a class on <html> ('.theme-light' | '.theme-dark' |
// '.theme-auto') drives CSS overrides in app.css. The .theme-auto
// class flips to light only inside a (prefers-color-scheme: light)
// media query — so it tracks the system setting. The other two are
// unconditional.

const STORAGE_KEY = 'bruv:theme'
export type Theme = 'light' | 'dark' | 'auto'

function read(): Theme {
  if (typeof localStorage === 'undefined') return 'auto'
  const v = localStorage.getItem(STORAGE_KEY)
  return v === 'light' || v === 'dark' ? v : 'auto'
}

function apply(t: Theme): void {
  if (typeof document === 'undefined') return
  const html = document.documentElement
  html.classList.remove('theme-light', 'theme-dark', 'theme-auto')
  html.classList.add(`theme-${t}`)
}

let _theme = $state<Theme>(read())
apply(_theme)

export const theme = {
  get current(): Theme {
    return _theme
  },
}

/** Set the current theme and persist. Call from a UI toggle. */
export function setTheme(t: Theme): void {
  _theme = t
  try {
    localStorage.setItem(STORAGE_KEY, t)
  } catch {
    /* private mode / storage full — non-fatal */
  }
  apply(t)
}
