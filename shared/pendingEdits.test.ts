import { describe, it, expect } from 'vitest'
import { failedEditCount, pendingEditGroups } from './pendingEdits'

const messages = [
  { id: 'm1', pending_edits: [{ id: 'a', status: 'pending' }, { id: 'b', status: 'accepted' }] },
  { id: 'm2' },
  { id: 'm3', pending_edits: [{ id: 'c', status: 'rejected' }] },
  { id: 'm4', pending_edits: [{ id: 'd', status: 'pending' }, { id: 'e', status: 'failed' }] },
]

describe('pendingEditGroups', () => {
  it('keeps only the edits still waiting, by message', () => {
    expect(pendingEditGroups(messages)).toEqual([
      { messageId: 'm1', edits: [{ id: 'a', status: 'pending' }] },
      { messageId: 'm4', edits: [{ id: 'd', status: 'pending' }] },
    ])
  })
})

describe('failedEditCount', () => {
  it('counts the accepted edits that failed', () => {
    expect(failedEditCount(messages, ['e', 'b'])).toBe(1)
    expect(failedEditCount(messages, [])).toBe(0)
  })
})
