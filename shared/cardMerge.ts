import type {
  Block,
  Card,
  ChecklistItem,
  ListItem,
  MediaItem,
  SurveyQuestion,
} from './types'
import type { ExportedCard } from './cardJson'

// Non-destructive merge of an exported card INTO an existing card.
//
// Pure planning: given the target card and the parsed export, compute the
// target's new blocks / tags / description / due date plus a summary of
// what changed. Nothing here touches a backend — `mergeCardFromJson` in
// cardTransfer.ts applies a plan. Keeping plan and apply apart makes the
// rules unit-testable and gives the surfaces a summary to toast.
//
// Rules (ruled by Harvey 2026-09-06):
// - Blocks match on `key` when both have one, else on label
//   (trimmed, case-insensitive). Blocks with neither are appended.
// - Multi-item blocks with a matching counterpart of the SAME type
//   append only the source items the target lacks; identity is per type
//   (text for list/checklist, url for media/image, prompt for survey,
//   option for checkbox_group). Existing items are never touched, so a
//   checklist keeps the target's done state.
// - Every other matching block ("single-value", or a type mismatch) is
//   kept on both sides: the source copy is appended with a "(Merged)"
//   suffix for manual cleanup — unless its value is empty or identical
//   to the target's, in which case it is skipped. That makes merging the
//   same file twice a no-op.
// - Appended blocks always get fresh ids, and a key only if no target
//   block already owns it (keys are unique per card).
// - Title and type are the target's. Description: fill if empty, else
//   append under a heading. Due date: fill only if empty. Tags: union.

export type MergeSummary = {
  /** Multi-item blocks that received at least one new item. */
  blocksMerged: number
  /** Matching single-value blocks kept as a suffixed copy. */
  blocksCopied: number
  /** Source blocks with no counterpart, appended as-is. */
  blocksAdded: number
  /** Items appended across all merged multi-item blocks. */
  itemsAdded: number
  tagsAdded: number
  descriptionChanged: boolean
  dueDateSet: boolean
}

export type MergePlan = {
  /** Full replacement block list for the target, or null when unchanged. */
  blocks: Block[] | null
  /** Full replacement tag list, or null when unchanged. */
  tags: string[] | null
  /** New description, or null when unchanged. */
  description: string | null
  /** New due date (YYYY-MM-DD), or null when unchanged. */
  dueDate: string | null
  summary: MergeSummary
}

export type MergeLabels = {
  /** Appended to a kept single-value copy's label, e.g. "(Merged)". */
  mergedSuffix: string
  /** Heading placed above an appended description, e.g. "Merged from imported card". */
  mergedHeading: string
}

export type MergeOptions = MergeLabels & {
  /** Id factory, injected so tests are deterministic. Receives a prefix like "blk". */
  newId?: (prefix: string) => string
}

export function isMergeNoop(summary: MergeSummary): boolean {
  return (
    summary.blocksMerged === 0 &&
    summary.blocksCopied === 0 &&
    summary.blocksAdded === 0 &&
    summary.tagsAdded === 0 &&
    !summary.descriptionChanged &&
    !summary.dueDateSet
  )
}

// --- Item identity per multi-item type ---

type ItemLike = { id?: string }

type MultiItemRule = {
  idPrefix: string
  /** Normalised identity of one item, or null when the item is empty. */
  identity: (item: unknown) => string | null
}

// Prefixes match what the block editors mint (EditableChecklist "ck-",
// EditableList "li-", MediaBlock "med-") so merged items are
// indistinguishable from hand-entered ones.
const MULTI_ITEM_RULES: Partial<Record<Block['type'], MultiItemRule>> = {
  checklist: { idPrefix: 'ck', identity: (i) => normText((i as ChecklistItem)?.text) },
  list: { idPrefix: 'li', identity: (i) => normText((i as ListItem)?.text) },
  media: { idPrefix: 'med', identity: (i) => normText((i as MediaItem)?.url) },
  image: { idPrefix: 'med', identity: (i) => normText((i as MediaItem)?.url) },
  survey: { idPrefix: 'sq', identity: (i) => normText((i as SurveyQuestion)?.prompt) },
}

function normText(s: unknown): string | null {
  if (typeof s !== 'string') return null
  const n = s.trim().replace(/\s+/g, ' ').toLowerCase()
  return n === '' ? null : n
}

function normLabel(s: string | undefined): string {
  return (s ?? '').trim().replace(/\s+/g, ' ').toLowerCase()
}

function defaultNewId(prefix: string): string {
  return `${prefix}-${crypto.randomUUID().slice(0, 8)}`
}

// --- Value emptiness / equality for the single-value rule ---

function isEmptyValue(v: Block['value']): boolean {
  if (v === null || v === undefined) return true
  if (typeof v === 'string') return v.trim() === ''
  if (typeof v === 'number') return false
  if (typeof v === 'boolean') return v === false
  if (Array.isArray(v)) return v.length === 0
  if (typeof v === 'object') {
    if ('url' in v && typeof (v as { url?: unknown }).url === 'string') {
      return (v as { url: string }).url.trim() === ''
    }
    return Object.keys(v).length === 0
  }
  return false
}

/** Structural equality with trimmed strings and sorted object keys. */
function canonical(v: unknown): string {
  if (typeof v === 'string') return JSON.stringify(v.trim())
  if (Array.isArray(v)) return `[${v.map(canonical).join(',')}]`
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>
    return `{${Object.keys(o).sort().map((k) => `${JSON.stringify(k)}:${canonical(o[k])}`).join(',')}}`
  }
  return JSON.stringify(v ?? null)
}

function sameValue(a: Block['value'], b: Block['value']): boolean {
  return canonical(a) === canonical(b)
}

// --- Matching ---

function findMatch(target: Block[], source: Block, claimed: Set<string>): Block | null {
  // Key match first — the machine identity a template assigned.
  if (source.key) {
    const byKey = target.find((b) => b.key === source.key && !claimed.has(b.id))
    if (byKey) return byKey
  }
  const label = normLabel(source.label)
  if (label === '') return null
  return target.find((b) => normLabel(b.label) === label && !claimed.has(b.id)) ?? null
}

// --- Multi-item merge ---

function mergeItems(
  targetBlock: Block,
  sourceBlock: Block,
  rule: MultiItemRule,
  newId: (prefix: string) => string,
): { value: Block['value']; added: number } {
  const targetItems = Array.isArray(targetBlock.value) ? [...(targetBlock.value as unknown[])] : []
  const sourceItems = Array.isArray(sourceBlock.value) ? (sourceBlock.value as unknown[]) : []
  const seen = new Set<string>()
  for (const it of targetItems) {
    const id = rule.identity(it)
    if (id) seen.add(id)
  }
  let added = 0
  for (const it of sourceItems) {
    const id = rule.identity(it)
    if (!id || seen.has(id)) continue
    seen.add(id)
    targetItems.push({ ...(it as object), id: newId(rule.idPrefix) } as ItemLike)
    added++
  }
  return { value: targetItems as Block['value'], added }
}

/** checkbox_group: value is the selected option list, meta.options the choices. */
function mergeCheckboxGroup(targetBlock: Block, sourceBlock: Block): { block: Block; added: number } {
  const tOpts = (targetBlock.meta?.options ?? []) as string[]
  const sOpts = (sourceBlock.meta?.options ?? []) as string[]
  const tSel = Array.isArray(targetBlock.value) ? (targetBlock.value as string[]) : []
  const sSel = Array.isArray(sourceBlock.value) ? (sourceBlock.value as string[]) : []

  const optionSeen = new Set(tOpts.map(normLabel))
  const options = [...tOpts]
  let added = 0
  for (const o of [...sOpts, ...sSel]) {
    const n = normLabel(o)
    if (n === '' || optionSeen.has(n)) continue
    optionSeen.add(n)
    options.push(o)
    added++
  }
  const selSeen = new Set(tSel.map(normLabel))
  const selected = [...tSel]
  for (const s of sSel) {
    const n = normLabel(s)
    if (n === '' || selSeen.has(n)) continue
    selSeen.add(n)
    selected.push(s)
  }
  if (added === 0 && selected.length === tSel.length) return { block: targetBlock, added: 0 }
  const meta = options.length === tOpts.length ? targetBlock.meta : { ...(targetBlock.meta ?? {}), options }
  return { block: { ...targetBlock, value: selected, meta }, added }
}

// --- The plan ---

export function planCardMerge(target: Card, source: ExportedCard, opts: MergeOptions): MergePlan {
  const newId = opts.newId ?? defaultNewId
  const summary: MergeSummary = {
    blocksMerged: 0, blocksCopied: 0, blocksAdded: 0, itemsAdded: 0,
    tagsAdded: 0, descriptionChanged: false, dueDateSet: false,
  }

  // Blocks. Work on a copy of the target list; matched target blocks are
  // replaced in place, everything else appends in source order.
  const blocks: Block[] = target.blocks.map((b) => ({ ...b }))
  const usedKeys = new Set(blocks.map((b) => b.key).filter((k) => k !== ''))
  const claimed = new Set<string>() // target blocks already matched this merge
  let blocksChanged = false

  const appendBlock = (src: Block, label: string, dropKey: boolean) => {
    const key = dropKey || !src.key || usedKeys.has(src.key) ? '' : src.key
    if (key) usedKeys.add(key)
    blocks.push({ ...src, id: newId('blk'), label, key })
    blocksChanged = true
  }

  for (const src of source.blocks ?? []) {
    const match = findMatch(blocks, src, claimed)

    if (!match) {
      // Unmatched: append as-is, but only blocks that carry content.
      // Empty template fields, dividers and stray blanks add structure
      // the target didn't ask for, not information.
      if (isEmptyValue(src.value)) continue
      appendBlock(src, src.label, false)
      summary.blocksAdded++
      continue
    }
    claimed.add(match.id)

    if (match.type === src.type) {
      if (src.type === 'checkbox_group') {
        const { block, added } = mergeCheckboxGroup(match, src)
        if (added > 0 || block !== match) {
          blocks[blocks.indexOf(match)] = block
          blocksChanged = true
          summary.blocksMerged++
          summary.itemsAdded += added
        }
        continue
      }
      const rule = MULTI_ITEM_RULES[src.type]
      if (rule) {
        const { value, added } = mergeItems(match, src, rule, newId)
        if (added > 0) {
          blocks[blocks.indexOf(match)] = { ...match, value }
          blocksChanged = true
          summary.blocksMerged++
          summary.itemsAdded += added
        }
        continue
      }
    }

    // Single-value rule (also covers a type mismatch under one label).
    if (isEmptyValue(src.value) || sameValue(match.value, src.value)) continue
    appendBlock(src, `${src.label.trim()} ${opts.mergedSuffix}`.trim(), true)
    summary.blocksCopied++
  }

  // Tags: union, case-insensitive, target order first.
  let tags: string[] | null = null
  const tagSeen = new Set(target.tags.map((t) => t.toLowerCase()))
  const mergedTags = [...target.tags]
  for (const t of source.tags ?? []) {
    const n = t.trim()
    if (n === '' || tagSeen.has(n.toLowerCase())) continue
    tagSeen.add(n.toLowerCase())
    mergedTags.push(n)
    summary.tagsAdded++
  }
  if (summary.tagsAdded > 0) tags = mergedTags

  // Description: fill, or append under a heading; never overwrite.
  let description: string | null = null
  const srcDesc = (source.description ?? '').trim()
  const tgtDesc = (target.description ?? '').trim()
  if (srcDesc !== '' && srcDesc !== tgtDesc && !tgtDesc.includes(srcDesc)) {
    description = tgtDesc === '' ? srcDesc : `${tgtDesc}\n\n---\n\n**${opts.mergedHeading}**\n\n${srcDesc}`
    summary.descriptionChanged = true
  }

  // Due date: fill only.
  let dueDate: string | null = null
  if (!target.due_date && source.due_date) {
    dueDate = source.due_date.slice(0, 10)
    summary.dueDateSet = true
  }

  return { blocks: blocksChanged ? blocks : null, tags, description, dueDate, summary }
}
