import './style.css'
import { mount } from 'svelte'
import { initBackend } from '@shared/adapters'
import { NeedsEnrolmentError } from '@shared/adapters/cloud'
import { GetCardProjectContext } from '@shared/api'
import type { WailsWindow } from '@shared/types'
import App from './App.svelte'
import EnrolmentScreen from './components/EnrolmentScreen.svelte'

// Global link click interceptor
document.addEventListener('click', (e) => {
  const anchor = (e.target as HTMLElement).closest('a')
  if (!anchor) return

  // Internal BRUV links — dispatch navigation event
  const bruv = anchor.getAttribute('data-bruv')
  if (bruv) {
    e.preventDefault()
    e.stopPropagation()
    if (bruv.startsWith('card:')) {
      const id = bruv.slice(5)
      document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'card', id } }))
    } else if (bruv.startsWith('project:')) {
      const id = bruv.slice(8)
      document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'project', id } }))
    }
    return
  }

  // Workspace file links (workspace://<ws-id>/<path>) — open via the panel.
  const wsLink = anchor.getAttribute('data-workspace')
  if (wsLink) {
    e.preventDefault()
    e.stopPropagation()
    document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'workspace-file', id: wsLink } }))
    return
  }

  // External links (http/https) — in Wails mode, ALWAYS open in system
  // browser to prevent the WebView from navigating away from the app.
  const href = anchor.getAttribute('href')
  if (!href) return
  if (href.startsWith('http://') || href.startsWith('https://')) {
    e.preventDefault()
    e.stopPropagation()
    try {
      const { BrowserOpenURL } = (window as WailsWindow).runtime ?? {}
      if (BrowserOpenURL) {
        BrowserOpenURL(href)
      } else if (anchor.target === '_blank') {
        window.open(href, '_blank')
      } else {
        window.location.href = href
      }
    } catch {
      if (anchor.target === '_blank') {
        window.open(href, '_blank')
      } else {
        window.location.href = href
      }
    }
  }
})

// Resolve bruv:card link titles from the index at render time.
// Links with an explicit title attribute (user override) are left untouched.
const CARD_LINK_SEL = '.bruv-link[data-bruv^="card:"]:not([title])'

function resolveBruvTitles(root: Element | Document) {
  const links = root.querySelectorAll<HTMLAnchorElement>(CARD_LINK_SEL)
  for (const link of links) {
    const cardId = link.getAttribute('data-bruv')!.slice(5)
    GetCardProjectContext(cardId).then(ctx => { if (ctx) link.title = ctx })
  }
}

const observer = new MutationObserver((mutations) => {
  for (const m of mutations) {
    for (const node of m.addedNodes) {
      if (!(node instanceof HTMLElement)) continue
      if (node.matches?.(CARD_LINK_SEL)) {
        const cardId = node.getAttribute('data-bruv')!.slice(5)
        GetCardProjectContext(cardId).then(ctx => { if (ctx) node.title = ctx })
      }
      resolveBruvTitles(node)
    }
  }
})
observer.observe(document.body, { childList: true, subtree: true })

// Boot the backend adapter. If the cloud adapter reports that the
// user hasn't enrolled yet (browser mode, no Wails shell, no saved
// credentials), render the enrolment wizard instead of the main app
// shell. On successful enrolment the wizard reloads the page and
// this path resolves through to App normally.
initBackend()
  .then(() => {
    mount(App, { target: document.getElementById('app')! })
  })
  .catch((err) => {
    if (err instanceof NeedsEnrolmentError) {
      mount(EnrolmentScreen, { target: document.getElementById('app')! })
      return
    }
    // Any other error is a real startup failure — surface it.
    console.error('[bruv] backend init failed:', err)
    const target = document.getElementById('app')
    if (target) {
      target.innerHTML = `<pre style="padding:24px;color:#f87171">BRUV failed to start: ${String(err?.message ?? err)}</pre>`
    }
  })
