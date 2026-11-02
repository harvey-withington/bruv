// Theme management with persistence. The user picks System (follow the
// OS, live), Light or Dark; `mode` is what's actually showing, and is what
// data-theme and the CSS see. New installs follow the system.

export type ThemeChoice = 'system' | 'light' | 'dark'
type Mode = 'light' | 'dark'

const STORAGE_KEY = 'bruv-theme'

const darkQuery = typeof window !== 'undefined' ? window.matchMedia?.('(prefers-color-scheme: dark)') : undefined

export const theme = $state({
  choice: 'system' as ThemeChoice,
  mode: 'dark' as Mode,
})

function resolve(choice: ThemeChoice): Mode {
  if (choice !== 'system') return choice
  return darkQuery?.matches === false ? 'light' : 'dark'
}

function isChoice(v: string | null): v is ThemeChoice {
  return v === 'system' || v === 'light' || v === 'dark'
}

/** The sidebar button: pins the opposite of what's showing. */
export function toggleTheme() {
  setTheme(theme.mode === 'dark' ? 'light' : 'dark')
}

export function setTheme(choice: ThemeChoice) {
  theme.choice = choice
  applyTheme()
  localStorage.setItem(STORAGE_KEY, choice)
}

export function loadTheme() {
  const saved = localStorage.getItem(STORAGE_KEY)
  theme.choice = isChoice(saved) ? saved : 'system'
  applyTheme()
}

function applyTheme() {
  theme.mode = resolve(theme.choice)
  document.documentElement.setAttribute('data-theme', theme.mode)
}

darkQuery?.addEventListener?.('change', () => {
  if (theme.choice === 'system') applyTheme()
})
