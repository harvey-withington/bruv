// Markdown → HTML for every `{@html}` display of user text on both surfaces.
//
// The text is untrusted: it comes from LLM replies, agents, external MCP
// clients, Trello imports and the web clipper, and the desktop window holds
// the bridge to the Go side. marked passes raw HTML through, so every render
// goes through DOMPurify: no scripts, event handlers, styles, forms, frames,
// ids or foreign data-* attributes, and only safe URL schemes. What BRUV's
// own renderers emit (mention / workspace links, task checkboxes, _blank
// links) is allowlisted below.
import DOMPurify, { type Config } from 'dompurify'
import { Marked, type RendererObject } from 'marked'
import { TASK_CLASS } from './markdownTasks'
import { escapeHtml } from './documentFormats/text'

/** The one link target allowed: new window (links from `[text](url,_blank)`). */
const BLANK = '_blank'

function titleAttr(title: string | null | undefined): string {
  return title ? ` title="${escapeHtml(title)}"` : ''
}

const linkRenderer: RendererObject = {
  link({ href, title, tokens }) {
    const text = this.parser.parseInline(tokens)

    // Internal BRUV links: bruv:card:<uuid> or bruv:project:<brand>/<stream>/<project>
    if (href.startsWith('bruv:')) {
      const payload = href.slice('bruv:'.length)
      return `<a class="bruv-link" data-bruv="${escapeHtml(payload)}"${titleAttr(title)}>${text}</a>`
    }

    // Workspace file links: workspace://<ws-id>/<path>. Path-based, so they
    // survive tier transitions; the click handler opens per current tier.
    if (href.startsWith('workspace://')) {
      const payload = href.slice('workspace://'.length)
      return `<a class="bruv-link" data-workspace="${escapeHtml(payload)}"${titleAttr(title)}>${text}</a>`
    }

    // [text](url,_blank) syntax (pre-processed from "url, _blank"). Any
    // "_target" suffix is stripped from the URL; only _blank is honoured —
    // the click interceptors route web links to the browser anyway.
    let blank = false
    const commaIdx = href.lastIndexOf(',')
    if (commaIdx !== -1) {
      const maybeTgt = href.slice(commaIdx + 1).trim()
      if (maybeTgt.startsWith('_')) {
        blank = maybeTgt === BLANK
        href = href.slice(0, commaIdx).trim()
      }
    }
    const targetAttr = blank ? ` target="${BLANK}"` : ''
    return `<a href="${escapeHtml(href)}"${titleAttr(title)}${targetAttr}>${text}</a>`
  },
}

// Enabled task checkboxes for surfaces that can save the toggle back
// (see markdownTasks.ts). Mouse/tap only — the surrounding display is the
// keyboard target and Space there means "edit".
const taskRenderer: RendererObject = {
  ...linkRenderer,
  checkbox({ checked }) {
    return `<input type="checkbox" class="${TASK_CLASS}" tabindex="-1"${checked ? ' checked' : ''}> `
  },
}

const blockMarked = new Marked({ renderer: linkRenderer, gfm: true, breaks: true })
const taskMarked = new Marked({ renderer: taskRenderer, gfm: true, breaks: true })
const inlineMarked = new Marked({ renderer: linkRenderer, gfm: true, breaks: false })

// Normalize "[text](url, _target)" → "[text](url,_target)" so marked's
// tokenizer keeps the target inside the href (spaces break link parsing).
function preprocess(text: string): string {
  return text.replace(/\]\(([^)]+?),\s+(_\w+)\)/g, ']($1,$2)')
}

// ── Sanitizing ──────────────────────────────────────────────────────

// The HTML profile (no SVG or MathML), less what could run, submit, embed,
// restyle or cover the app, or clash with its ids. data-* is off except the
// two link attributes the click delegates read (marked as URI-safe so their
// "card:<id>" values aren't judged as URLs). `target` is allowed here and
// narrowed to <a target="_blank"> in the hook.
const PURIFY: Config = {
  USE_PROFILES: { html: true },
  FORBID_TAGS: [
    'style', 'form', 'button', 'textarea', 'select', 'option', 'iframe', 'frame', 'object', 'embed',
    'dialog', 'template', 'link', 'meta', 'base', 'audio', 'video', 'source', 'track',
  ],
  FORBID_ATTR: ['style', 'id', 'name', 'form', 'formaction', 'srcset', 'autofocus', 'contenteditable', 'popover'],
  ALLOW_DATA_ATTR: false,
  ADD_ATTR: ['data-bruv', 'data-workspace', 'target'],
  ADD_URI_SAFE_ATTR: ['data-bruv', 'data-workspace'],
  // Web, mail and phone links plus relative ones; never javascript:,
  // vbscript: or data: (data: images are stripped in the hook too).
  ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
}

const LANGUAGE_CLASS = /^language-[\w+#-]+$/
const KEEP_CLASS = new Set(['bruv-link', TASK_CLASS])
const DATA_URI = /^\s*data:/i

type Purifier = ReturnType<typeof DOMPurify>

/** A purifier whose hooks know whether task checkboxes may stay enabled. */
function makePurifier(tasks: boolean): Purifier {
  const purify = DOMPurify() // bound to the global window
  purify.addHook('uponSanitizeAttribute', (_node, data) => {
    if (data.attrName === 'class') {
      data.attrValue = data.attrValue.split(/\s+/).filter(c => KEEP_CLASS.has(c) || LANGUAGE_CLASS.test(c)).join(' ')
      if (!data.attrValue) data.keepAttr = false
    } else if ((data.attrName === 'src' || data.attrName === 'href') && DATA_URI.test(data.attrValue)) {
      // DOMPurify lets data: through on <img src>; nothing in BRUV needs it.
      data.keepAttr = false
    }
  })
  purify.addHook('afterSanitizeAttributes', node => {
    if (node.tagName === 'INPUT') {
      // Only checkboxes survive. Enabled ones (clickable, out of the tab
      // order) only in the tasks variant; everywhere else they're read-only.
      if (node.getAttribute('type') !== 'checkbox') {
        node.remove()
        return
      }
      if (tasks && node.classList.contains(TASK_CLASS)) {
        node.removeAttribute('disabled')
        node.setAttribute('tabindex', '-1')
        return
      }
      node.removeAttribute('class')
      node.setAttribute('disabled', '')
    }
    node.removeAttribute('tabindex')
    if (node.tagName === 'A') {
      if (node.getAttribute('target') !== BLANK) node.removeAttribute('target')
      if (node.hasAttribute('href')) node.setAttribute('rel', 'noopener noreferrer')
      else node.removeAttribute('rel')
    } else {
      node.removeAttribute('target')
    }
  })
  return purify
}

let purifiers: { block: Purifier; tasks: Purifier } | null = null

function clean(html: string, tasks: boolean): string {
  // marked escapes text, so output without a tag has nothing to sanitize
  // (most titles and names) — skip the DOM round trip.
  if (!html.includes('<')) return html
  if (typeof window === 'undefined') return escapeHtml(html)
  purifiers ??= { block: makePurifier(false), tasks: makePurifier(true) }
  return (tasks ? purifiers.tasks : purifiers.block).sanitize(html, PURIFY)
}

/** Render full block markdown (paragraphs, lists, headings, code blocks, etc.).
 *  `tasks` makes "- [ ]" checkboxes clickable; pair it with taskToggleFromClick. */
export function renderMarkdown(text: string, opts: { tasks?: boolean } = {}): string {
  if (!text) return ''
  const tasks = opts.tasks ?? false
  return clean((tasks ? taskMarked : blockMarked).parse(preprocess(text)) as string, tasks)
}

/** Render inline-only markdown (bold, italic, strikethrough, code, links — no block elements) */
export function renderInline(text: string): string {
  if (!text) return ''
  return clean(inlineMarked.parseInline(preprocess(text)) as string, false)
}
