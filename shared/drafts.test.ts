import { describe, it, expect, beforeEach } from 'vitest'
import { cardDraftScope, loadDraft, projectDraftScope, saveDraft } from './drafts'

describe('chat drafts', () => {
  beforeEach(() => localStorage.clear())

  it('keeps a draft per chat', () => {
    saveDraft(cardDraftScope('c1'), 'half a thought')
    saveDraft(projectDraftScope('home/trips/winter'), 'another')
    expect(loadDraft(cardDraftScope('c1'))).toBe('half a thought')
    expect(loadDraft(projectDraftScope('home/trips/winter'))).toBe('another')
    expect(loadDraft(cardDraftScope('c2'))).toBe('')
  })

  it('forgets a draft once it is emptied (sent or cleared)', () => {
    saveDraft(cardDraftScope('c1'), 'something')
    saveDraft(cardDraftScope('c1'), '   ')
    expect(localStorage.length).toBe(0)
    expect(loadDraft(cardDraftScope('c1'))).toBe('')
  })
})
