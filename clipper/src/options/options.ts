// Options page: pair with a BRUV server (bootstrap token → device token,
// same flow as the mobile PWA), then pick the target repo and where clipped
// cards get pinned. The pin target is a mention-style searchable picker
// (BRUV's picker rules: pre-populated on focus, type to filter, arrow keys
// + Enter to select) rather than a bare <select>.

import { clearSettings, enrol, listRepos, loadSettings, repoRPC, saveSettings } from '../lib/api'
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

// Field names mirror card.CategoryPath's JSON tags (camelCase — unlike
// index.SearchResult, which has no tags and stays PascalCase).
type CategoryPath = { categoryId: string; breadcrumb: string }

type PinEntry = { id: string; name: string; special: boolean }

let current: ClipperSettings | null = null
let allCategories: CategoryPath[] = []
let selectedPin: { id: string; name: string } = { id: '', name: '' }
let activeIdx = -1

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

function selectPin(entry: PinEntry): void {
  selectedPin = { id: entry.id, name: entry.name }
  const input = $<HTMLInputElement>('category-search')
  input.value = entry.name
  input.setAttribute('aria-expanded', 'false')
  $<HTMLUListElement>('category-results').hidden = true
  activeIdx = -1
}

function renderPinList(query: string): void {
  const list = $<HTMLUListElement>('category-results')
  const entries = pinEntries(query)
  list.innerHTML = ''
  entries.forEach((entry, i) => {
    const li = document.createElement('li')
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.textContent = entry.name
    btn.className = (entry.special ? 'special' : '') + (i === activeIdx ? ' active' : '')
    btn.setAttribute('role', 'option')
    // pointerdown, not click — selection must beat the input's blur.
    btn.addEventListener('pointerdown', (e) => {
      e.preventDefault()
      selectPin(entry)
    })
    li.appendChild(btn)
    list.appendChild(li)
  })
  list.hidden = entries.length === 0
  $<HTMLInputElement>('category-search').setAttribute('aria-expanded', String(!list.hidden))
}

function wirePinPicker(): void {
  const input = $<HTMLInputElement>('category-search')
  input.placeholder = msg('options_category_placeholder')

  input.addEventListener('focus', () => {
    // Pre-populate before typing — an empty picker is capture friction.
    input.select()
    activeIdx = -1
    renderPinList('')
  })
  input.addEventListener('input', () => {
    activeIdx = -1
    renderPinList(input.value)
  })
  input.addEventListener('blur', () => {
    // Restore the committed selection; pointerdown-selection already ran.
    setTimeout(() => {
      input.value = selectedPin.name
      $<HTMLUListElement>('category-results').hidden = true
      input.setAttribute('aria-expanded', 'false')
    }, 120)
  })
  input.addEventListener('keydown', (e) => {
    const entries = pinEntries(input.value)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (entries.length === 0) return
      const delta = e.key === 'ArrowDown' ? 1 : -1
      activeIdx = (activeIdx + delta + entries.length) % entries.length
      renderPinList(input.value)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const entry = activeIdx >= 0 ? entries[activeIdx] : entries[0]
      if (entry) selectPin(entry)
      input.blur()
    } else if (e.key === 'Escape') {
      input.value = selectedPin.name
      $<HTMLUListElement>('category-results').hidden = true
      input.setAttribute('aria-expanded', 'false')
      activeIdx = -1
    }
  })
}

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
    showStatus(err instanceof Error ? err.message : String(err), false)
  }

  // Seed the picker from the stored selection.
  if (current.categoryID === PIN_WITH_DECK) {
    selectedPin = { id: PIN_WITH_DECK, name: msg('options_category_deck') }
  } else if (current.categoryID) {
    selectedPin = { id: current.categoryID, name: current.categoryName || current.categoryID }
  } else {
    selectedPin = { id: '', name: msg('options_category_inbox') }
  }
  $<HTMLInputElement>('category-search').value = selectedPin.name
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
      showStatus(err instanceof Error ? err.message : String(err), false)
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
    $<HTMLInputElement>('category-search').value = selectedPin.name
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

wirePinPicker()
void refresh()
