// Field → card-block bindings. A slide can link a card and bind each of its
// content-type fields to a compatible block of that card, so the slide pulls
// live content. Compatibility + value extraction live here so the desktop
// editor, the (future) present-side resolver, and any binding UI agree.

import type { Block, BlockType, SlideFieldType } from './types'

// blockTypesForFieldType lists the block types that can supply a value for a
// given slide field type. Media fields want image/media; everything else takes
// any block that renders as readable text.
export function blockTypesForFieldType(fieldType: SlideFieldType): BlockType[] {
  switch (fieldType) {
    case 'image':
      return ['image', 'media']
    case 'video':
      return ['media']
    case 'text':
    case 'longtext':
    default:
      return ['text', 'checklist', 'list', 'select', 'radio', 'number', 'date', 'url', 'progress', 'rating']
  }
}

export function isBlockCompatible(blockType: BlockType, fieldType: SlideFieldType): boolean {
  return blockTypesForFieldType(fieldType).includes(blockType)
}

// multi=true (image fields) joins EVERY item URL with '\n' — a multi-image
// media block resolves to a multi-URL value that renderers show as a
// carousel. Video fields stay first-URL (one player per slide).
function urlFromValue(v: unknown, multi: boolean): string {
  if (typeof v === 'string') return v
  if (Array.isArray(v)) {
    const urls = v
      .map((it) => (it && typeof it === 'object' && 'url' in it ? String((it as { url?: unknown }).url ?? '') : ''))
      .filter(Boolean)
    return multi ? urls.join('\n') : (urls[0] ?? '')
  }
  if (v && typeof v === 'object' && 'url' in v) return String((v as { url?: unknown }).url ?? '')
  return ''
}

// resolveBlockValueForField extracts a renderable string from a block for the
// given field type — URL(s) for media fields, readable text otherwise.
export function resolveBlockValueForField(block: Block, fieldType: SlideFieldType): string {
  const v = block.value
  if (fieldType === 'image' || fieldType === 'video') {
    return urlFromValue(v, fieldType === 'image')
  }
  if (typeof v === 'string') return v
  if (typeof v === 'number') return String(v)
  if (typeof v === 'boolean') return v ? 'Yes' : 'No'
  if (Array.isArray(v)) {
    return v
      .map((it) => (it && typeof it === 'object' && 'text' in it ? String((it as { text?: unknown }).text ?? '') : ''))
      .filter(Boolean)
      .join('\n')
  }
  if (v && typeof v === 'object' && 'url' in v) return String((v as { url?: unknown }).url ?? '')
  return v == null ? '' : String(v)
}
