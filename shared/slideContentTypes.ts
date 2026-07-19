import type { SlideContentType } from './types'

// Curated built-in Slide Content Types — each a named set of typed fields,
// mirroring Card Types. The set is intentionally small; the format is open
// (user-defined content types are a later feature). The atomic unit is the
// field TYPE (text/longtext/image/video), which covers essentially any field,
// so a curated list of common bundles is enough for v1.
export const SLIDE_CONTENT_TYPES: SlideContentType[] = [
  {
    id: 'title',
    name: 'Title',
    fields: [
      { key: 'title', label: 'Title', type: 'text' },
      { key: 'subtitle', label: 'Subtitle', type: 'text' },
    ],
  },
  {
    id: 'statement',
    name: 'Statement',
    fields: [{ key: 'statement', label: 'Statement', type: 'longtext' }],
  },
  {
    id: 'quote',
    name: 'Quote',
    fields: [
      { key: 'quote', label: 'Quote', type: 'longtext' },
      { key: 'author', label: 'Author', type: 'text' },
    ],
  },
  {
    id: 'image',
    name: 'Image',
    fields: [
      { key: 'image', label: 'Image', type: 'image' },
      { key: 'caption', label: 'Caption', type: 'text' },
    ],
  },
  {
    id: 'video',
    name: 'Video',
    fields: [
      { key: 'video', label: 'Video', type: 'video' },
      { key: 'caption', label: 'Caption', type: 'text' },
    ],
  },
  {
    id: 'lower_third',
    name: 'Lower Third',
    fields: [
      { key: 'name', label: 'Name', type: 'text' },
      { key: 'subtitle', label: 'Subtitle', type: 'text' },
    ],
  },
]

export const DEFAULT_CONTENT_TYPE_ID = 'title'

const BY_ID = new Map(SLIDE_CONTENT_TYPES.map((ct) => [ct.id, ct]))

export function resolveContentType(id: string): SlideContentType | undefined {
  return BY_ID.get(id)
}
