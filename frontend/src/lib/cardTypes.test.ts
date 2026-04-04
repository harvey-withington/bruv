import { describe, it, expect } from 'vitest'
import { getCardTypeColor, getCardTypeTextColor, getCardTypeLabel } from './cardTypes'
import type { CardTypeInfo } from './types'

describe('getCardTypeColor', () => {
  it('returns elevated bg for null type', () => {
    expect(getCardTypeColor(null)).toBe('var(--bg-elevated)')
  })

  it('returns elevated bg for undefined type', () => {
    expect(getCardTypeColor(undefined)).toBe('var(--bg-elevated)')
  })

  it('returns elevated bg for empty string', () => {
    expect(getCardTypeColor('')).toBe('var(--bg-elevated)')
  })

  it('returns correct color for built-in type "feature"', () => {
    expect(getCardTypeColor('feature')).toBe('#6366f1')
  })

  it('returns correct color for built-in type "task"', () => {
    expect(getCardTypeColor('task')).toBe('#22c55e')
  })

  it('returns correct color for built-in type "brainstorm"', () => {
    expect(getCardTypeColor('brainstorm')).toBe('#f59e0b')
  })

  it('returns correct color for built-in type "episode"', () => {
    expect(getCardTypeColor('episode')).toBe('#ec4899')
  })

  it('returns correct color for built-in type "reference"', () => {
    expect(getCardTypeColor('reference')).toBe('#06b6d4')
  })

  it('returns fallback color for unknown type', () => {
    expect(getCardTypeColor('unknown-type')).toBe('#71717a')
  })

  it('returns custom type color from types array', () => {
    const types: CardTypeInfo[] = [
      { id: 'custom', label: 'Custom', color: '#ff0000', description: '', builtin: false },
    ]
    expect(getCardTypeColor('custom', types)).toBe('#ff0000')
  })

  it('types array overrides built-in color', () => {
    const types: CardTypeInfo[] = [
      { id: 'feature', label: 'Feature', color: '#000000', description: '', builtin: true },
    ]
    expect(getCardTypeColor('feature', types)).toBe('#000000')
  })

  it('falls back to built-in when type not in types array', () => {
    const types: CardTypeInfo[] = [
      { id: 'other', label: 'Other', color: '#111111', description: '', builtin: false },
    ]
    expect(getCardTypeColor('feature', types)).toBe('#6366f1')
  })
})

describe('getCardTypeTextColor', () => {
  it('returns white for a type with a value', () => {
    expect(getCardTypeTextColor('feature')).toBe('#fff')
  })

  it('returns muted for null type', () => {
    expect(getCardTypeTextColor(null)).toBe('var(--text-muted)')
  })

  it('returns muted for undefined type', () => {
    expect(getCardTypeTextColor(undefined)).toBe('var(--text-muted)')
  })

  it('returns muted for empty string', () => {
    expect(getCardTypeTextColor('')).toBe('var(--text-muted)')
  })
})

describe('getCardTypeLabel', () => {
  it('returns empty string for null type', () => {
    expect(getCardTypeLabel(null)).toBe('')
  })

  it('returns empty string for undefined type', () => {
    expect(getCardTypeLabel(undefined)).toBe('')
  })

  it('returns empty string for empty string', () => {
    expect(getCardTypeLabel('')).toBe('')
  })

  it('capitalises built-in type ID', () => {
    expect(getCardTypeLabel('feature')).toBe('Feature')
  })

  it('capitalises first letter of multi-word type', () => {
    expect(getCardTypeLabel('brainstorm')).toBe('Brainstorm')
  })

  it('returns label from types array', () => {
    const types: CardTypeInfo[] = [
      { id: 'custom', label: 'My Custom Type', color: '#ff0000', description: '', builtin: false },
    ]
    expect(getCardTypeLabel('custom', types)).toBe('My Custom Type')
  })

  it('types array overrides built-in label', () => {
    const types: CardTypeInfo[] = [
      { id: 'feature', label: 'Feature Request', color: '#6366f1', description: '', builtin: true },
    ]
    expect(getCardTypeLabel('feature', types)).toBe('Feature Request')
  })

  it('falls back to capitalised ID when not in types array', () => {
    const types: CardTypeInfo[] = [
      { id: 'other', label: 'Other', color: '#111111', description: '', builtin: false },
    ]
    expect(getCardTypeLabel('feature', types)).toBe('Feature')
  })
})
