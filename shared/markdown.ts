import { Marked, type RendererObject } from 'marked'
import { TASK_CLASS } from './markdownTasks'

const linkRenderer: RendererObject = {
  link({ href, title, tokens }) {
    const text = this.parser.parseInline(tokens)

    // Internal BRUV links: bruv:card:<uuid> or bruv:project:<brand>/<stream>/<project>
    if (href.startsWith('bruv:')) {
      const payload = href.slice(5) // strip "bruv:"
      const titleAttr = title ? ` title="${title}"` : ''
      return `<a class="bruv-link" data-bruv="${payload}"${titleAttr}>${text}</a>`
    }

    // Workspace file links: workspace://<ws-id>/<path>. Path-based, so they
    // survive tier transitions; the click handler opens per current tier.
    if (href.startsWith('workspace://')) {
      const payload = href.slice('workspace://'.length)
      const titleAttr = title ? ` title="${title}"` : ''
      return `<a class="bruv-link" data-workspace="${payload}"${titleAttr}>${text}</a>`
    }

    let target = ''
    // Support [text](url,_blank) syntax (pre-processed from "url, _blank")
    const commaIdx = href.lastIndexOf(',')
    if (commaIdx !== -1) {
      const maybeTgt = href.slice(commaIdx + 1).trim()
      if (maybeTgt.startsWith('_')) {
        target = maybeTgt
        href = href.slice(0, commaIdx).trim()
      }
    }
    const titleAttr = title ? ` title="${title}"` : ''
    const targetAttr = target ? ` target="${target}" rel="noopener noreferrer"` : ''
    return `<a href="${href}"${titleAttr}${targetAttr}>${text}</a>`
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

/** Render full block markdown (paragraphs, lists, headings, code blocks, etc.).
 *  `tasks` makes "- [ ]" checkboxes clickable; pair it with taskToggleFromClick. */
export function renderMarkdown(text: string, opts: { tasks?: boolean } = {}): string {
  if (!text) return ''
  return (opts.tasks ? taskMarked : blockMarked).parse(preprocess(text)) as string
}

/** Render inline-only markdown (bold, italic, strikethrough, code, links — no block elements) */
export function renderInline(text: string): string {
  if (!text) return ''
  return inlineMarked.parseInline(preprocess(text)) as string
}
