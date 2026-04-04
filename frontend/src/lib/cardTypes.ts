import type { CardTypeInfo } from './types'

export const CARD_TYPE_ORDER = ['brainstorm', 'task', 'reference', 'feature', 'episode'] as const

const CARD_TYPE_COLORS: Record<string, string> = {
  brainstorm: '#f59e0b',
  task: '#22c55e',
  reference: '#06b6d4',
  feature: '#6366f1',
  episode: '#ec4899',
}

const CARD_TYPE_FALLBACK_COLOR = '#71717a'

export function getCardTypeColor(type: string | null | undefined, types?: CardTypeInfo[]): string {
  if (!type) return 'var(--bg-elevated)'
  if (types) {
    const found = types.find(t => t.id === type)
    if (found) return found.color
  }
  return CARD_TYPE_COLORS[type] ?? CARD_TYPE_FALLBACK_COLOR
}

export function getCardTypeTextColor(type: string | null | undefined): string {
  return type ? '#fff' : 'var(--text-muted)'
}

export function getCardTypeLabel(type: string | null | undefined, types?: CardTypeInfo[]): string {
  if (!type) return ''
  if (types) {
    const found = types.find(t => t.id === type)
    if (found) return found.label
  }
  // Capitalise built-in type IDs as fallback
  return type.charAt(0).toUpperCase() + type.slice(1)
}
