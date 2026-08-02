// The in-page capture options dialog (content-script side).
//
// The toolbar popup can't be opened programmatically and won't hold focus,
// so this renders in the page itself — same mechanism as the clip toast,
// bigger. It is DOM-only: no chrome APIs beyond i18n, no settings, no RPC.
// Everything it shows is decided background-side and arrives in the
// request, so this file's one job is presentation and the user's answer.
//
// Self-containment rules (host pages must not be able to break it):
//   - every class is `bruv-cd-*`, styled in content.css at high specificity;
//   - key and pointer events are stopped at the root, because sites like X
//     bind single-key shortcuts on document and would otherwise act on
//     whatever the user types into the title field;
//   - Escape and a backdrop click cancel; focus goes to the primary action
//     and is handed back to wherever it was when the dialog closes.

import { formatBytes } from './format'
import {
  VIDEO_OPTION_LINK,
  VIDEO_OPTION_SKIP,
  type CaptureChoices,
  type CaptureDialogRequest,
  type CaptureDialogVideoOption,
  type ImageChoice,
} from './types'

export type DialogOutcome = { ok: boolean; choices?: CaptureChoices; openOptions?: boolean }

const msg = (key: string): string => chrome.i18n.getMessage(key)

const ROOT_CLASS = 'bruv-cd-root'

// Only one dialog at a time; a second right-click supersedes the first
// (the toast behaves the same way).
let dismissOpen: ((outcome: DialogOutcome) => void) | null = null

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className: string,
  text?: string,
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function section(labelKey: string): HTMLDivElement {
  const wrap = el('div', 'bruv-cd-section')
  wrap.appendChild(el('div', 'bruv-cd-legend', msg(labelKey)))
  return wrap
}

// One radio row. The <label> wraps its input, so the whole row is the hit
// target without any id/for bookkeeping.
function radio(name: string, value: string, label: string, checked: boolean, hint?: string): HTMLLabelElement {
  const row = el('label', 'bruv-cd-option')
  const input = el('input', 'bruv-cd-radio')
  input.type = 'radio'
  input.name = name
  input.value = value
  input.checked = checked
  row.appendChild(input)
  const body = el('div', 'bruv-cd-optionbody')
  body.appendChild(el('span', 'bruv-cd-optionlabel', label))
  if (hint) body.appendChild(el('span', 'bruv-cd-hint', hint))
  row.appendChild(body)
  return row
}

function pickedValue(root: HTMLElement, name: string, fallback: string): string {
  const checked = root.querySelector<HTMLInputElement>(`input[name="${name}"]:checked`)
  return checked ? checked.value : fallback
}

// "1280×720 · ~725 MB" — the size is the whole point of showing a ladder,
// so a rung whose size is unknown says so rather than looking free.
function videoRowLabel(option: CaptureDialogVideoOption): string {
  const size = formatBytes(option.estBytes ?? 0)
  return size ? msg('dialog_option_size').replace('{label}', option.label).replace('{size}', size) : option.label
}

function buildVideo(req: CaptureDialogRequest): HTMLDivElement | null {
  if (req.videoOptions.length === 0) return null
  const wrap = section('dialog_video')
  for (const option of req.videoOptions) {
    const hint = option.estBytes ? undefined : msg('dialog_size_unknown')
    wrap.appendChild(radio('bruv-cd-video', option.id, videoRowLabel(option), option.id === req.defaults.videoOptionId, hint))
  }
  wrap.appendChild(
    radio(
      'bruv-cd-video',
      VIDEO_OPTION_LINK,
      msg('dialog_video_link'),
      req.defaults.videoOptionId === VIDEO_OPTION_LINK,
      msg('dialog_video_link_note'),
    ),
  )
  wrap.appendChild(
    radio('bruv-cd-video', VIDEO_OPTION_SKIP, msg('dialog_video_skip'), req.defaults.videoOptionId === VIDEO_OPTION_SKIP),
  )
  return wrap
}

function buildImages(req: CaptureDialogRequest): HTMLDivElement | null {
  if (req.imageCount === 0) return null
  const wrap = section('dialog_images')
  const mode = req.defaults.images
  wrap.appendChild(
    radio('bruv-cd-images', 'all', msg('dialog_images_all').replace('{n}', String(req.imageCount)), mode === 'all'),
  )
  // "First only" is a gallery answer; with one image it would be a synonym
  // for "All".
  if (req.imageCount > 1) {
    wrap.appendChild(radio('bruv-cd-images', 'first', msg('dialog_images_first'), mode === 'first'))
  }
  wrap.appendChild(radio('bruv-cd-images', 'link', msg('dialog_images_link'), mode === 'link', msg('dialog_link_note')))
  wrap.appendChild(radio('bruv-cd-images', 'skip', msg('dialog_images_skip'), mode === 'skip'))
  return wrap
}

// Destinations are read-only here: they're sticky settings owned by the
// popup (deck) and the Options page (pin), and re-picking them at every
// capture is the friction this dialog is meant to avoid. The one live
// control is whether THIS clip becomes a slide.
function buildDestination(req: CaptureDialogRequest, onOptions: () => void): { node: HTMLDivElement; deck: HTMLInputElement } {
  const wrap = section('dialog_destination')

  const deckLine = el('div', 'bruv-cd-line')
  deckLine.appendChild(el('span', 'bruv-cd-linelabel', msg('dialog_deck')))
  deckLine.appendChild(el('span', 'bruv-cd-linevalue', req.deckName || msg('dialog_deck_none')))
  wrap.appendChild(deckLine)

  const pinLine = el('div', 'bruv-cd-line')
  pinLine.appendChild(el('span', 'bruv-cd-linelabel', msg('dialog_pin')))
  pinLine.appendChild(el('span', 'bruv-cd-linevalue', req.pinName))
  wrap.appendChild(pinLine)

  const deckRow = el('label', 'bruv-cd-option')
  const deck = el('input', 'bruv-cd-radio')
  deck.type = 'checkbox'
  deck.checked = req.defaults.includeInDeck && req.canDeck
  deck.disabled = !req.canDeck
  deckRow.appendChild(deck)
  const body = el('div', 'bruv-cd-optionbody')
  body.appendChild(el('span', 'bruv-cd-optionlabel', msg('dialog_include_deck')))
  if (!req.canDeck) body.appendChild(el('span', 'bruv-cd-hint', msg('dialog_deck_hint')))
  deckRow.appendChild(body)
  wrap.appendChild(deckRow)

  const link = el('button', 'bruv-cd-linkbtn', msg('dialog_open_options'))
  link.type = 'button'
  link.addEventListener('click', onOptions)
  wrap.appendChild(link)
  return { node: wrap, deck }
}

function buildNotes(req: CaptureDialogRequest): HTMLDivElement | null {
  if (req.notes.length === 0) return null
  const wrap = el('div', 'bruv-cd-notes')
  for (const note of req.notes) wrap.appendChild(el('p', 'bruv-cd-note', note))
  return wrap
}

/** Show the dialog and resolve with the user's answer. Cancelling (Escape,
 *  backdrop, Cancel) resolves `{ ok: false }` — never rejects. */
export function showCaptureDialog(req: CaptureDialogRequest): Promise<DialogOutcome> {
  dismissOpen?.({ ok: false })

  return new Promise<DialogOutcome>((resolve) => {
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const root = el('div', ROOT_CLASS)
    const panel = el('div', 'bruv-cd-panel')
    panel.setAttribute('role', 'dialog')
    panel.setAttribute('aria-modal', 'true')
    panel.setAttribute('aria-label', msg('dialog_title'))

    const head = el('div', 'bruv-cd-head')
    head.appendChild(el('div', 'bruv-cd-title', msg('dialog_title')))
    const subtitle = [req.byline, req.platform].filter((s) => !!s).join(' · ')
    if (subtitle) head.appendChild(el('div', 'bruv-cd-sub', subtitle))
    panel.appendChild(head)

    const body = el('div', 'bruv-cd-body')
    const notes = buildNotes(req)
    if (notes) body.appendChild(notes)

    const titleSection = section('dialog_field_title')
    const titleInput = el('input', 'bruv-cd-input')
    titleInput.type = 'text'
    titleInput.value = req.defaults.title
    titleSection.appendChild(titleInput)
    body.appendChild(titleSection)

    const video = buildVideo(req)
    if (video) body.appendChild(video)
    const images = buildImages(req)
    if (images) body.appendChild(images)

    let done = false
    const finish = (outcome: DialogOutcome): void => {
      if (done) return
      done = true
      dismissOpen = null
      document.removeEventListener('keydown', onKeydown, true)
      root.remove()
      previouslyFocused?.focus()
      resolve(outcome)
    }

    const destination = buildDestination(req, () => finish({ ok: false, openOptions: true }))
    body.appendChild(destination.node)
    panel.appendChild(body)

    const confirm = (): void => {
      const videoOptionId = pickedValue(panel, 'bruv-cd-video', VIDEO_OPTION_SKIP)
      const picked = req.videoOptions.find((o) => o.id === videoOptionId)
      const imageChoice = pickedValue(panel, 'bruv-cd-images', req.defaults.images) as ImageChoice
      finish({
        ok: true,
        choices: {
          title: titleInput.value.trim(),
          includeInDeck: destination.deck.checked && req.canDeck,
          video: videoOptionId === VIDEO_OPTION_SKIP ? 'skip' : picked ? 'store' : 'link',
          videoUrl: picked?.url ?? '',
          images: req.imageCount === 0 ? 'all' : imageChoice,
        },
      })
    }

    const foot = el('div', 'bruv-cd-foot')
    const cancelBtn = el('button', 'bruv-cd-btn bruv-cd-ghost', msg('dialog_cancel'))
    cancelBtn.type = 'button'
    cancelBtn.addEventListener('click', () => finish({ ok: false }))
    const captureBtn = el('button', 'bruv-cd-btn bruv-cd-primary', msg('dialog_capture'))
    captureBtn.type = 'button'
    captureBtn.addEventListener('click', confirm)
    foot.appendChild(cancelBtn)
    foot.appendChild(captureBtn)
    panel.appendChild(foot)

    // Enter commits from the title field (BRUV's keyboard contract), and
    // no keystroke inside the dialog reaches the host page.
    panel.addEventListener('keydown', (e) => {
      e.stopPropagation()
      if (e.key === 'Enter' && e.target === titleInput) {
        e.preventDefault()
        confirm()
      }
    })
    for (const type of ['keyup', 'keypress', 'click', 'mousedown', 'mouseup'] as const) {
      panel.addEventListener(type, (e) => e.stopPropagation())
    }

    root.addEventListener('mousedown', (e) => {
      if (e.target === root) finish({ ok: false })
    })

    function onKeydown(e: KeyboardEvent): void {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      e.preventDefault()
      finish({ ok: false })
    }
    document.addEventListener('keydown', onKeydown, true)

    root.appendChild(panel)
    document.documentElement.appendChild(root)
    dismissOpen = finish
    // The common case is "yes, as configured" — so the primary action is
    // what's focused, not the first field.
    captureBtn.focus()
  })
}
