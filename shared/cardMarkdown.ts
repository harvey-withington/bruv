import type {
  Card,
  Block,
  CardComment,
  ChecklistItem,
  ListItem,
  MediaItem,
  SurveyQuestion,
} from './types'

// --- Card → readable Markdown (one-way export) ---
//
// Produces a human-readable markdown document for a single card. Aimed
// at "paste into Obsidian / print / share with a collaborator" — not at
// round-tripping back into BRUV. Lossy by design: block IDs, template
// keys, and structured meta (options lists, min/max, etc.) are dropped.
// Attachments are listed by filename only; their bytes are not embedded.
//
// Keep this module dependency-free so both desktop and mobile can use it
// without pulling in DOM or backend-only modules.

export type CardMarkdownOptions = {
  comments?: CardComment[]
  /** Override "Untitled card" fallback when card.title is empty. */
  untitledLabel?: string
}

export function cardToMarkdown(card: Card, opts: CardMarkdownOptions = {}): string {
  const out: string[] = []

  const frontmatter = buildFrontmatter(card)
  if (frontmatter) {
    out.push(frontmatter)
    out.push('')
  }

  const title = card.title?.trim() || opts.untitledLabel || 'Untitled card'
  out.push(`# ${title}`)
  out.push('')

  const description = card.description?.trim()
  if (description) {
    out.push(description)
    out.push('')
  }

  for (const block of card.blocks ?? []) {
    const rendered = renderBlock(block)
    if (rendered === null) continue
    out.push(rendered)
    out.push('')
  }

  const attachments = card.file_attachments ?? []
  if (attachments.length > 0) {
    out.push('## Attachments')
    out.push('')
    for (const a of attachments) {
      out.push(`- ${a.name}`)
    }
    out.push('')
  }

  const comments = opts.comments ?? []
  if (comments.length > 0) {
    out.push('## Comments')
    out.push('')
    for (const c of comments) {
      const when = isoToDate(c.updated_at || c.created_at)
      out.push(`### ${c.author || 'Unknown'} — ${when}`)
      out.push('')
      out.push(c.text.trim())
      out.push('')
    }
  }

  // Collapse runs of blank lines; trim trailing whitespace.
  return out.join('\n').replace(/\n{3,}/g, '\n\n').trimEnd() + '\n'
}

// ---------------------------------------------------------------------
// Frontmatter
// ---------------------------------------------------------------------

function buildFrontmatter(card: Card): string {
  const lines: string[] = []
  if (card.type) lines.push(`type: ${yamlScalar(card.type)}`)
  if (card.tags?.length) lines.push(`tags: ${yamlFlowList(card.tags)}`)
  if (card.due_date) lines.push(`due: ${yamlScalar(isoToDate(card.due_date))}`)
  if (card.created_at) lines.push(`created: ${yamlScalar(isoToDate(card.created_at))}`)
  if (card.members?.length) lines.push(`members: ${yamlFlowList(card.members)}`)
  if (lines.length === 0) return ''
  return ['---', ...lines, '---'].join('\n')
}

function yamlScalar(value: string): string {
  // Quote if the value contains characters YAML would otherwise parse
  // specially (colons, brackets, leading dashes, etc.). When in doubt,
  // quote — readability isn't hurt and parsers stay happy.
  if (value === '') return '""'
  if (/^[\w./-]+$/.test(value)) return value
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function yamlFlowList(items: string[]): string {
  return `[${items.map(yamlScalar).join(', ')}]`
}

// ---------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------

function renderBlock(block: Block): string | null {
  if (block.type === 'divider') return '---'

  if (isEmpty(block.value) && block.type !== 'alarm') return null

  const heading = block.label?.trim() || prettifyKey(block.key) || block.type
  const body = renderBlockBody(block)
  if (body === null || body === '') return null

  return `## ${heading}\n\n${body}`
}

function renderBlockBody(block: Block): string | null {
  switch (block.type) {
    case 'text':
      return String(block.value ?? '').trim()

    case 'checklist': {
      const items = (block.value as ChecklistItem[]) ?? []
      return items
        .filter(i => i.text?.trim())
        .map(i => `- [${i.done ? 'x' : ' '}] ${i.text.trim()}`)
        .join('\n')
    }

    case 'list': {
      const items = (block.value as ListItem[]) ?? []
      return items
        .filter(i => i.text?.trim())
        .map(i => `- ${i.text.trim()}`)
        .join('\n')
    }

    case 'media':
    case 'image': {
      const items = (block.value as MediaItem[]) ?? []
      const lines = items
        .filter(m => m.url)
        .map(m => `![${(m.caption || '').trim()}](${m.url})`)
      return lines.join('\n\n')
    }

    case 'url': {
      const v = block.value as { url: string; caption?: string } | null
      if (!v?.url) return null
      const label = v.caption?.trim() || v.url
      return `[${label}](${v.url})`
    }

    case 'select':
    case 'radio':
      return String(block.value ?? '').trim()

    case 'number':
    case 'progress': {
      const suffix = block.meta?.suffix ? ` ${block.meta.suffix}` : ''
      return `${block.value}${suffix}`
    }

    case 'date':
      return isoToDate(String(block.value ?? ''))

    case 'rating': {
      const n = Number(block.value) || 0
      const max = block.meta?.max ?? 5
      return `${n} / ${max}`
    }

    case 'checkbox':
      return block.value ? '- [x]' : '- [ ]'

    case 'checkbox_group': {
      const selected = new Set((block.value as string[]) ?? [])
      const options = block.meta?.options ?? []
      const source = options.length > 0 ? options : Array.from(selected)
      return source
        .map(opt => `- [${selected.has(opt) ? 'x' : ' '}] ${opt}`)
        .join('\n')
    }

    case 'alarm': {
      const at = block.meta?.alarm_time
      if (!at) return null
      const fired = block.meta?.alarm_fired ? ' (fired)' : ''
      return `Alarm: ${at}${fired}`
    }

    case 'survey': {
      const questions = (block.value as SurveyQuestion[]) ?? []
      return questions
        .filter(q => q.prompt?.trim())
        .map(renderSurveyQuestion)
        .join('\n\n')
    }

    default:
      return String(block.value ?? '').trim() || null
  }
}

function renderSurveyQuestion(q: SurveyQuestion): string {
  const prompt = `**${q.prompt.trim()}**`
  const answer = q.answer
  if (answer === undefined || answer === null || answer === '') return `${prompt}\n\n_No answer_`
  if (Array.isArray(answer)) {
    if (answer.length === 0) return `${prompt}\n\n_No answer_`
    return `${prompt}\n\n${answer.map(a => `- ${a}`).join('\n')}`
  }
  return `${prompt}\n\n${answer}`
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

function isEmpty(v: Block['value']): boolean {
  if (v === null || v === undefined) return true
  if (v === '' || v === 0 || v === false) return true
  if (Array.isArray(v) && v.length === 0) return true
  return false
}

function isoToDate(iso: string): string {
  if (!iso) return ''
  // Accept either YYYY-MM-DD or full ISO 8601 — both yield a clean date prefix.
  return iso.length >= 10 ? iso.slice(0, 10) : iso
}

function prettifyKey(key: string): string {
  if (!key) return ''
  return key
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
    .trim()
}
