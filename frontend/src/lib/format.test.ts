import { describe, it, expect } from 'vitest'
import { formatBytes } from './format'

// This number is what the user weighs a decision against — "commit 3.2 GB?"
// — so it has to be right at the boundaries, not just in the middle.
describe('formatBytes', () => {
  it('formats each unit the way a person reads it', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1 KB')
    expect(formatBytes(1536)).toBe('2 KB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(formatBytes(3.2e9)).toBe('3.0 GB')
  })

  it('drops the decimal once it stops carrying information', () => {
    // 100 MB+ doesn't need a tenth; 1.4 GB does.
    expect(formatBytes(150 * 1024 * 1024)).toBe('150 MB')
    expect(formatBytes(1.4 * 1024 * 1024 * 1024)).toBe('1.4 GB')
  })

  it('never renders a negative or nonsense size', () => {
    expect(formatBytes(-1)).toBe('0 B')
    expect(formatBytes(NaN)).toBe('0 B')
  })
})
