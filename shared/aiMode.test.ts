import { describe, it, expect } from 'vitest'
import { aiModeOf, aiModeTurnContext, nextAIMode } from './aiMode'

describe('aiMode', () => {
  it('reads the stored value, defaulting to Edit', () => {
    expect(aiModeOf('chat')).toBe('chat')
    expect(aiModeOf('suggest')).toBe('suggest')
    expect(aiModeOf('')).toBe('edit')
    expect(aiModeOf(undefined)).toBe('edit')
    expect(aiModeOf('nonsense')).toBe('edit')
  })

  it('cycles Edit → Suggest → Chat → Edit', () => {
    expect(nextAIMode('edit')).toBe('suggest')
    expect(nextAIMode('suggest')).toBe('chat')
    expect(nextAIMode('chat')).toBe('edit')
  })

  it('tells the model what its board writes will do', () => {
    expect(aiModeTurnContext('chat')).toContain("won't change")
    expect(aiModeTurnContext('suggest')).toContain('staged')
    expect(aiModeTurnContext('edit')).toContain('apply at once')
  })
})
