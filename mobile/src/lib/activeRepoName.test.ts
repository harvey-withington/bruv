import { describe, it, expect, beforeEach } from 'vitest'
import { saveActiveRepoID, saveActiveRepoName, readActiveRepoName, clearActiveRepoID } from './auth'

// The Browse header shows the cached vault name before /repos answers
// (offline start). It must never show one vault's name for another.
describe('active vault name cache', () => {
  beforeEach(() => localStorage.clear())

  it('keeps the name while the same vault stays active', () => {
    saveActiveRepoID('v1')
    saveActiveRepoName('Home')
    saveActiveRepoID('v1')
    expect(readActiveRepoName()).toBe('Home')
  })

  it('drops the name when another vault is picked or the selection is cleared', () => {
    saveActiveRepoID('v1')
    saveActiveRepoName('Home')
    saveActiveRepoID('v2')
    expect(readActiveRepoName()).toBeNull()

    saveActiveRepoName('Work')
    clearActiveRepoID()
    expect(readActiveRepoName()).toBeNull()
  })
})
