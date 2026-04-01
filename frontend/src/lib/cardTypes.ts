export const CARD_TYPE_ORDER = ['feature', 'task', 'brainstorm', 'episode', 'reference'] as const

export const CARD_TYPE_COLORS: Record<string, string> = {
  feature: '#6366f1',
  task: '#22c55e',
  brainstorm: '#f59e0b',
  episode: '#ec4899',
  reference: '#06b6d4',
}

const CARD_TYPE_FALLBACK_COLOR = '#71717a'

export function getCardTypeColor(type: string | null | undefined): string {
  if (!type) return 'var(--bg-elevated)'
  return CARD_TYPE_COLORS[type] ?? CARD_TYPE_FALLBACK_COLOR
}

export function getCardTypeTextColor(type: string | null | undefined): string {
  return type ? '#fff' : 'var(--text-muted)'
}
