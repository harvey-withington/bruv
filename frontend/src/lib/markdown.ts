import { Marked, type RendererObject } from 'marked'

const linkRenderer: RendererObject = {
  link(this: any, { href, title, tokens }: any) {
    const text = this.parser.parseInline(tokens)

    // Internal BRUV links: bruv:card:<uuid> or bruv:project:<brand>/<stream>/<project>
    if (href.startsWith('bruv:')) {
      const payload = href.slice(5) // strip "bruv:"
      const titleAttr = title ? ` title="${title}"` : ''
      return `<a class="bruv-link" data-bruv="${payload}"${titleAttr}>${text}</a>`
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

const blockMarked = new Marked({ renderer: linkRenderer, gfm: true, breaks: true })
const inlineMarked = new Marked({ renderer: linkRenderer, gfm: true, breaks: false })

// Normalize "[text](url, _target)" → "[text](url,_target)" so marked's
// tokenizer keeps the target inside the href (spaces break link parsing).
function preprocess(text: string): string {
  return text.replace(/\]\(([^)]+?),\s+(_\w+)\)/g, ']($1,$2)')
}

/** Render full block markdown (paragraphs, lists, headings, code blocks, etc.) */
export function renderMarkdown(text: string): string {
  if (!text) return ''
  return blockMarked.parse(preprocess(text)) as string
}

/** Render inline-only markdown (bold, italic, strikethrough, code, links — no block elements) */
export function renderInline(text: string): string {
  if (!text) return ''
  return inlineMarked.parseInline(preprocess(text)) as string
}
