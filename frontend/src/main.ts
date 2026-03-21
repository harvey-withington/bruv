import './style.css'
import { mount } from 'svelte'
import { initBackend } from './lib/adapters'
import { GetCardProjectContext } from './lib/api'
import App from './App.svelte'

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

  // External links (http/https) — in Wails mode, ALWAYS open in system
  // browser to prevent the WebView from navigating away from the app.
  const href = anchor.getAttribute('href')
  if (!href) return
  if (href.startsWith('http://') || href.startsWith('https://')) {
    e.preventDefault()
    e.stopPropagation()
    try {
      const { BrowserOpenURL } = (window as any).runtime || {}
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

initBackend().then(() => {
  const app = mount(App, {
    target: document.getElementById('app')!
  })
})
