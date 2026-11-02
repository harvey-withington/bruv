import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderInline } from './markdown'

/** Parse rendered HTML into a detached container for DOM assertions. */
function dom(html: string): HTMLDivElement {
  const root = document.createElement('div')
  root.innerHTML = html
  return root
}

/** Every attribute name present anywhere in the rendered tree. */
function attrNames(html: string): string[] {
  return Array.from(dom(html).querySelectorAll('*')).flatMap(el => el.getAttributeNames())
}

describe('renderMarkdown sanitizing', () => {
  it.each([
    ['img onerror', '<img src=x onerror=alert(1)>'],
    ['script', 'hi <script>alert(1)</script>'],
    ['iframe', '<iframe src="https://evil.example"></iframe>'],
    ['svg onload', '<svg onload=alert(1)><circle /></svg>'],
    ['details ontoggle', '<details open ontoggle=alert(1)><summary>x</summary></details>'],
  ])('drops script vectors: %s', (_name, md) => {
    for (const render of [(s: string) => renderMarkdown(s), (s: string) => renderMarkdown(s, { tasks: true }), renderInline]) {
      const root = dom(render(md))
      expect(root.querySelector('script, iframe, svg')).toBeNull()
      expect(attrNames(root.innerHTML).filter(a => a.startsWith('on'))).toEqual([])
    }
  })

  it('refuses javascript:, vbscript: and data: links from markdown', () => {
    for (const url of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'vbscript:msgbox(1)', 'data:text/html,<script>alert(1)</script>']) {
      const a = dom(renderMarkdown(`[x](${url})`)).querySelector('a')
      expect(a?.getAttribute('href') ?? null, url).toBeNull()
    }
  })

  it('refuses javascript: links from raw HTML', () => {
    const a = dom(renderMarkdown('<a href="javascript:alert(1)">x</a>')).querySelector('a')
    expect(a?.getAttribute('href') ?? null).toBeNull()
  })

  it('drops data: image sources', () => {
    const img = dom(renderMarkdown('![x](data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)')).querySelector('img')
    expect(img?.getAttribute('src') ?? null).toBeNull()
  })

  it('keeps a quote in a link title inside the attribute', () => {
    const html = renderMarkdown('[x](https://a.example "say \\"hi\\" onmouseover=alert(1) x")')
    const a = dom(html).querySelector('a')!
    expect(a.getAttribute('title')).toBe('say "hi" onmouseover=alert(1) x')
    expect(a.hasAttribute('onmouseover')).toBe(false)
  })

  it('cannot break out of a bruv: payload', () => {
    const a = dom(renderMarkdown('[x](bruv:card:1"onclick="alert(1))')).querySelector('a')!
    expect(a.hasAttribute('onclick')).toBe(false)
  })

  it('strips style, id, foreign classes and foreign data attributes', () => {
    const html = renderMarkdown('<p style="position:fixed" id="app" class="evil" data-card-id="1" tabindex="0">x</p>')
    const p = dom(html).querySelector('p')!
    expect(p.getAttributeNames()).toEqual([])
    expect(p.textContent).toBe('x')
  })

  it('drops forms, buttons, styles and non-checkbox inputs', () => {
    const root = dom(renderMarkdown('<form><input type="text"><button>go</button></form><style>*{}</style>'))
    expect(root.querySelector('form, input, button, style')).toBeNull()
  })

  it('only allows target="_blank" on links', () => {
    const root = dom(renderMarkdown('<a href="https://a.example" target="_top">a</a> <span target="_blank">b</span>'))
    expect(root.querySelector('a')!.hasAttribute('target')).toBe(false)
    expect(root.querySelector('span')!.hasAttribute('target')).toBe(false)
  })
})

describe('renderMarkdown keeps BRUV markup', () => {
  it('renders bruv: card and project mentions', () => {
    const root = dom(renderInline('see [Card](bruv:card:abc-123 "Brand / Stream") and [P](bruv:project:p1)'))
    const [card, project] = Array.from(root.querySelectorAll('a'))
    expect(card.className).toBe('bruv-link')
    expect(card.getAttribute('data-bruv')).toBe('card:abc-123')
    expect(card.getAttribute('title')).toBe('Brand / Stream')
    expect(card.hasAttribute('href')).toBe(false)
    expect(project.getAttribute('data-bruv')).toBe('project:p1')
  })

  it('renders workspace:// links', () => {
    const a = dom(renderMarkdown('[notes](workspace://ws1/docs/notes.md)')).querySelector('a')!
    expect(a.className).toBe('bruv-link')
    expect(a.getAttribute('data-workspace')).toBe('ws1/docs/notes.md')
    expect(a.hasAttribute('href')).toBe(false)
  })

  it('renders normal links with rel noopener', () => {
    const a = dom(renderMarkdown('[site](https://example.com/a?b=1&c=2)')).querySelector('a')!
    expect(a.getAttribute('href')).toBe('https://example.com/a?b=1&c=2')
    expect(a.getAttribute('rel')).toBe('noopener noreferrer')
    expect(a.hasAttribute('target')).toBe(false)
  })

  it.each(['[site](https://example.com,_blank)', '[site](https://example.com, _blank)'])('honours the _blank syntax: %s', (md) => {
    const a = dom(renderMarkdown(md)).querySelector('a')!
    expect(a.getAttribute('href')).toBe('https://example.com')
    expect(a.getAttribute('target')).toBe('_blank')
    expect(a.getAttribute('rel')).toBe('noopener noreferrer')
  })

  it('strips other _targets from the url without emitting them', () => {
    const a = dom(renderMarkdown('[site](https://example.com,_top)')).querySelector('a')!
    expect(a.getAttribute('href')).toBe('https://example.com')
    expect(a.hasAttribute('target')).toBe(false)
  })

  it('renders emphasis, code, lists and headings', () => {
    const root = dom(renderMarkdown('# Title\n\n**bold** *it* ~~gone~~ `code`\n\n- one\n- two\n\n```js\nlet a = 1 < 2\n```'))
    expect(root.querySelector('h1')!.textContent).toBe('Title')
    expect(root.querySelector('strong')!.textContent).toBe('bold')
    expect(root.querySelector('em')!.textContent).toBe('it')
    expect(root.querySelector('del')!.textContent).toBe('gone')
    expect(root.querySelector('p code')!.textContent).toBe('code')
    expect(root.querySelectorAll('ul li')).toHaveLength(2)
    const block = root.querySelector('pre code')!
    expect(block.textContent).toBe('let a = 1 < 2\n')
    expect(block.className).toBe('language-js')
  })

  it('renders inline emphasis without block wrappers', () => {
    expect(renderInline('**bold** and `code`')).toBe('<strong>bold</strong> and <code>code</code>')
    expect(renderInline('plain & simple')).toBe('plain &amp; simple')
  })

  it('keeps task checkboxes enabled, classed and out of the tab order in the tasks variant', () => {
    const boxes = Array.from(dom(renderMarkdown('- [ ] a\n- [x] b', { tasks: true })).querySelectorAll('input'))
    expect(boxes).toHaveLength(2)
    for (const box of boxes) {
      expect(box.type).toBe('checkbox')
      expect(box.className).toBe('md-task')
      expect(box.getAttribute('tabindex')).toBe('-1')
      expect(box.disabled).toBe(false)
    }
    expect(boxes.map(b => b.defaultChecked)).toEqual([false, true])
  })

  it('keeps task checkboxes read-only outside the tasks variant', () => {
    const boxes = Array.from(dom(renderMarkdown('- [ ] a\n- [x] b')).querySelectorAll('input'))
    expect(boxes).toHaveLength(2)
    for (const box of boxes) {
      expect(box.disabled).toBe(true)
      expect(box.hasAttribute('tabindex')).toBe(false)
    }
  })

  it('does not let raw HTML enable a checkbox outside the tasks variant', () => {
    const box = dom(renderMarkdown('<input type="checkbox" class="md-task" tabindex="0">')).querySelector('input')!
    expect(box.disabled).toBe(true)
    expect(box.hasAttribute('class')).toBe(false)
    expect(box.hasAttribute('tabindex')).toBe(false)
  })
})
