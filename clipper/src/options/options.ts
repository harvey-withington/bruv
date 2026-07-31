// Options page: pair with a BRUV server (bootstrap token → device token,
// same flow as the mobile PWA), then pick the target repo and where clipped
// cards get pinned. The pin target is a mention-style searchable picker
// (BRUV's picker rules: pre-populated on focus, type to filter, arrow keys
// + Enter to select) rather than a bare <select>.

import { clearSettings, enrol, enrolLocal, listRepos, loadSettings, repoRPC, saveSettings } from '../lib/api'
import { typeahead } from '../lib/typeahead'
import { PIN_WITH_DECK, type ClipperSettings } from '../lib/types'

const $ = <T extends HTMLElement>(id: string): T => document.getElementById(id) as T

const msg = (key: string): string => chrome.i18n.getMessage(key)

for (const el of Array.from(document.querySelectorAll<HTMLElement>('[data-i18n]'))) {
  el.textContent = msg(el.dataset.i18n as string)
}

const statusEl = $<HTMLDivElement>('status')
function showStatus(text: string, ok: boolean): void {
  statusEl.textContent = text
  statusEl.className = `status ${ok ? 'ok' : 'err'}`
}

// fetch() rejects with TypeError when the server was never reached —
// "Failed to fetch" tells the user nothing, so name the likely cause
// (same treatment as the repo-list load).
function showError(err: unknown, serverURL: string): void {
  if (err instanceof TypeError) {
    showStatus(msg('options_err_unreachable').replace('{url}', serverURL), false)
    return
  }
  showStatus(err instanceof Error ? err.message : String(err), false)
}

// Field names mirror card.CategoryPath's JSON tags (camelCase — unlike
// index.SearchResult, which has no tags and stays PascalCase).
type CategoryPath = { categoryId: string; breadcrumb: string }

type PinEntry = { id: string; name: string; special: boolean }

let current: ClipperSettings | null = null
let allCategories: CategoryPath[] = []
let selectedPin: { id: string; name: string } = { id: '', name: '' }

function specialEntries(): PinEntry[] {
  return [
    { id: '', name: msg('options_category_inbox'), special: true },
    { id: PIN_WITH_DECK, name: msg('options_category_deck'), special: true },
  ]
}

function pinEntries(query: string): PinEntry[] {
  const q = query.trim().toLowerCase()
  const all = [
    ...specialEntries(),
    ...allCategories.map((c): PinEntry => ({ id: c.categoryId, name: c.breadcrumb, special: false })),
  ]
  if (!q) return all
  return all.filter((e) => e.name.toLowerCase().includes(q))
}

// Same combobox behaviour as the popup's deck picker (lib/typeahead.ts).
// Entries here are local — specials plus the loaded category list — so
// the lookup is synchronous.
const pinPicker = typeahead({
  input: $<HTMLInputElement>('category-search'),
  list: $<HTMLUListElement>('category-results'),
  entries: (query) =>
    pinEntries(query).map((e) => ({ id: e.id, label: e.name, special: e.special })),
  onSelect: (entry) => {
    selectedPin = { id: entry.id, name: entry.label }
  },
})

async function refresh(): Promise<void> {
  current = await loadSettings()
  const paired = current !== null
  $('pair-form').hidden = paired
  $('paired-info').hidden = !paired
  $('unpair-btn').hidden = !paired
  $('target-section').hidden = !paired
  if (!current) return

  $('paired-info').textContent = `${msg('options_paired_with')} ${current.serverURL}`

  const repoSelect = $<HTMLSelectElement>('repo-select')
  repoSelect.innerHTML = ''
  try {
    const repos = await listRepos(current)
    for (const r of repos) {
      const opt = document.createElement('option')
      opt.value = r.id
      opt.textContent = r.name
      opt.selected = r.id === current.repoID
      repoSelect.appendChild(opt)
    }
    if (!current.repoID && repos.length > 0) repoSelect.value = repos[0].id
    await loadCategories()
  } catch (err) {
    // "Failed to fetch" is a TypeError with zero diagnostic value —
    // translate it: the server at the paired URL isn't answering, and
    // for a local pairing that almost always means the desktop app
    // isn't running (or rebound to a different port).
    if (err instanceof TypeError) {
      showStatus(msg('options_err_unreachable').replace('{url}', current.serverURL), false)
    } else {
      showStatus(err instanceof Error ? err.message : String(err), false)
    }
  }

  // Seed the picker from the stored selection.
  if (current.categoryID === PIN_WITH_DECK) {
    selectedPin = { id: PIN_WITH_DECK, name: msg('options_category_deck') }
  } else if (current.categoryID) {
    selectedPin = { id: current.categoryID, name: current.categoryName || current.categoryID }
    // A stored category that isn't in the selected repo (pairing switched
    // servers, category deleted) would silently send every clip to the
    // Inbox — flag it while the stale name still reads legitimate.
    if (allCategories.length > 0 && !allCategories.some((c) => c.categoryId === current?.categoryID)) {
      showStatus(msg('options_category_stale'), false)
    }
  } else {
    selectedPin = { id: '', name: msg('options_category_inbox') }
  }
  pinPicker.setValue(selectedPin.name)
}

async function loadCategories(): Promise<void> {
  if (!current) return
  const repoSelect = $<HTMLSelectElement>('repo-select')
  allCategories = []
  try {
    const probe: ClipperSettings = { ...current, repoID: repoSelect.value }
    allCategories = (await repoRPC<CategoryPath[]>(probe, 'ListAllCategories', [])) ?? []
  } catch {
    // Category list is a nicety; the special entries always work.
  }
}

$('pair-btn').addEventListener('click', () => {
  void (async () => {
    const btn = $<HTMLButtonElement>('pair-btn')
    btn.disabled = true
    try {
      const result = await enrol($<HTMLInputElement>('server-url').value, $<HTMLInputElement>('bootstrap-token').value)
      const settings: ClipperSettings = {
        serverURL: result.serverURL,
        deviceToken: result.deviceToken,
        deviceID: result.deviceID,
        repoID: '',
        repoName: '',
        deckTarget: null,
        categoryID: '',
        categoryName: '',
      }
      await saveSettings(settings)
      showStatus(msg('options_paired_ok'), true)
      await refresh()
    } catch (err) {
      showError(err, $<HTMLInputElement>('server-url').value)
    } finally {
      btn.disabled = false
    }
  })()
})

// --- find the server on this machine ---------------------------------
//
// The local URL can't be DERIVED: the desktop app binds whatever
// LocalServerPort says, falling back to an ephemeral port when that one
// is taken, and bruv-server defaults to 9870 — none of it readable from
// an extension. So instead of a "local" checkbox that would be guessing,
// this probes a bounded set of loopback ports and IDENTIFIES a real BRUV
// server by its /version signature. Explicit click, loopback only, ten
// ports, immediate refusals — cheap and honest about what it found.

const PROBE_PORTS = [9870, 9871, 9872, 9873, 9874, 9875, 9876, 9877, 9878, 9879]
const PROBE_TIMEOUT_MS = 800

type FoundServer = { url: string; port: number; version: string }

async function probeBruvServer(port: number): Promise<FoundServer | null> {
  const url = `http://127.0.0.1:${port}`
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS)
  try {
    const res = await fetch(`${url}/version`, { signal: ctrl.signal })
    if (!res.ok) return null
    const body = (await res.json()) as { version?: unknown; go_version?: unknown }
    // Both fields together are BRUV's /version signature — enough to not
    // hand the user some unrelated service listening on a nearby port.
    if (typeof body.version !== 'string' || typeof body.go_version !== 'string') return null
    return { url, port, version: body.version }
  } catch {
    return null
  } finally {
    clearTimeout(timer)
  }
}

$('detect-btn').addEventListener('click', () => {
  void (async () => {
    const btn = $<HTMLButtonElement>('detect-btn')
    btn.disabled = true
    showStatus(msg('options_detect_running'), true)
    try {
      const results = (await Promise.all(PROBE_PORTS.map(probeBruvServer))).filter(
        (r): r is FoundServer => r !== null,
      )
      if (results.length === 0) {
        showStatus(msg('options_detect_none'), false)
        return
      }
      const chosen = results[0]
      $<HTMLInputElement>('server-url').value = chosen.url
      refreshPairAffordances()
      if (results.length > 1) {
        // Desktop app AND bruv-server both up, say — name the others
        // rather than silently picking one.
        showStatus(
          msg('options_detect_multi')
            .replace('{url}', chosen.url)
            .replace('{ports}', results.map((r) => String(r.port)).join(', ')),
          true,
        )
      } else {
        showStatus(msg('options_detected').replace('{url}', chosen.url).replace('{version}', chosen.version), true)
      }
    } finally {
      btn.disabled = false
    }
  })()
})

// Same-machine pairing: for a loopback server URL the bootstrap paste is
// unnecessary — the server trusts unproxied loopback + this extension's
// origin (transport/http/localpair.go). The button only appears when the
// entered URL is loopback, so remote pairings keep the explicit token.
function isLoopbackServerURL(value: string): boolean {
  try {
    const u = new URL(value.trim())
    return ['localhost', '127.0.0.1', '[::1]'].includes(u.hostname)
  } catch {
    return false
  }
}

// Whichever path is actually available leads: for a loopback server the
// token-less button becomes primary and the token form recedes (it was
// the other way round at first, and the purple Pair button — which
// demands a token — kept getting clicked instead).
function refreshPairAffordances(): void {
  const local = isLoopbackServerURL($<HTMLInputElement>('server-url').value)
  const localBtn = $<HTMLButtonElement>('local-pair-btn')
  const tokenBtn = $<HTMLButtonElement>('pair-btn')
  localBtn.hidden = !local
  localBtn.classList.toggle('secondary', !local)
  tokenBtn.classList.toggle('secondary', local)
  const collapseToken = local && !tokenFieldTouched
  $('token-row').hidden = collapseToken
  tokenBtn.hidden = collapseToken
  $('show-token-row').hidden = !collapseToken
  // Pairing with a token needs a token: disable rather than explain
  // afterwards.
  tokenBtn.disabled = !$<HTMLInputElement>('bootstrap-token').value.trim()
}

// Once the user deliberately touches the token field, stop collapsing it.
let tokenFieldTouched = false
$('bootstrap-token').addEventListener('input', () => {
  tokenFieldTouched = true
  refreshPairAffordances()
})
$('show-token-row').addEventListener('click', () => {
  tokenFieldTouched = true
  refreshPairAffordances()
  $<HTMLInputElement>('bootstrap-token').focus()
})
$('server-url').addEventListener('input', refreshPairAffordances)
refreshPairAffordances()

$('local-pair-btn').addEventListener('click', () => {
  void (async () => {
    const btn = $<HTMLButtonElement>('local-pair-btn')
    btn.disabled = true
    try {
      const result = await enrolLocal($<HTMLInputElement>('server-url').value)
      const settings: ClipperSettings = {
        serverURL: result.serverURL,
        deviceToken: result.deviceToken,
        deviceID: result.deviceID,
        repoID: '',
        repoName: '',
        deckTarget: null,
        categoryID: '',
        categoryName: '',
      }
      await saveSettings(settings)
      showStatus(msg('options_paired_ok'), true)
      await refresh()
    } catch (err) {
      showError(err, $<HTMLInputElement>('server-url').value)
    } finally {
      btn.disabled = false
    }
  })()
})

$('unpair-btn').addEventListener('click', () => {
  void (async () => {
    await clearSettings()
    showStatus(msg('options_unpaired'), true)
    await refresh()
  })()
})

$('repo-select').addEventListener('change', () => {
  void (async () => {
    // Categories are per-repo: reload the list and reset the pin selection
    // to Inbox — a category from the old repo is meaningless in the new one.
    await loadCategories()
    selectedPin = { id: '', name: msg('options_category_inbox') }
    pinPicker.setValue(selectedPin.name)
  })()
})

$('save-btn').addEventListener('click', () => {
  void (async () => {
    if (!current) return
    const repoSelect = $<HTMLSelectElement>('repo-select')
    const next: ClipperSettings = {
      ...current,
      repoID: repoSelect.value,
      repoName: repoSelect.selectedOptions[0]?.textContent ?? '',
      categoryID: selectedPin.id,
      categoryName: selectedPin.id ? selectedPin.name : '',
      // Deck targets are per-repo; a repo switch invalidates the old one.
      deckTarget: repoSelect.value === current.repoID ? current.deckTarget : null,
    }
    await saveSettings(next)
    current = next
    showStatus(msg('options_saved'), true)
  })()
})

$<HTMLInputElement>('category-search').placeholder = msg('options_category_placeholder')
void refresh()
