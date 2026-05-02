import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { registerServiceWorker } from './lib/serviceWorker'
import { loadLocale } from './lib/i18n.svelte'
import { hasActiveRepo, isEnrolled } from './lib/auth'
import { loadRepoMeta } from './lib/repoMeta.svelte'

// Restore the user's previously-chosen locale before any component
// renders. Synchronous; no network — safe to do before mount.
loadLocale()

// If the user is already enrolled with an active repo (returning
// visit), pre-warm the per-repo metadata (card types + global tag
// colour map) so badges + chips render in colour from the first
// paint. Fire-and-forget; fallback styling is fine if the request
// lags. Fresh enrolments hit this via RepoPickerPage.select().
if (isEnrolled() && hasActiveRepo()) {
  loadRepoMeta()
}

const target = document.getElementById('app')
if (!target) throw new Error('mobile: #app root element missing from index.html')

const app = mount(App, { target })

// Fire-and-forget — registration failure shouldn't block the app from
// rendering. Errors are surfaced inside the helper.
registerServiceWorker()

export default app
