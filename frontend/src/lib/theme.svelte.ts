// Theme management with persistence

export const theme = $state({
  mode: 'dark' as 'light' | 'dark',
})

export function toggleTheme() {
  theme.mode = theme.mode === 'dark' ? 'light' : 'dark'
  applyTheme()
  localStorage.setItem('bruv-theme', theme.mode)
}

export function setTheme(mode: 'light' | 'dark') {
  theme.mode = mode
  applyTheme()
  localStorage.setItem('bruv-theme', mode)
}

export function loadTheme() {
  const saved = localStorage.getItem('bruv-theme') as 'light' | 'dark' | null
  if (saved === 'light' || saved === 'dark') {
    theme.mode = saved
  }
  applyTheme()
}

function applyTheme() {
  document.documentElement.setAttribute('data-theme', theme.mode)
}
