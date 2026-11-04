import { describe, it, expect } from 'vitest'
import { uncliUnavailable } from './uncliStatus'

describe('uncliUnavailable', () => {
  it('is null when the engine is ready', () => {
    expect(uncliUnavailable({ ready: true }, '')).toBeNull()
  })

  it('says it is still checking before the status arrives', () => {
    expect(uncliUnavailable(null, '')).toEqual({ key: 'uncli.checking' })
  })

  it('carries the error when the server could not be asked', () => {
    expect(uncliUnavailable(null, 'GetUncliStatus is not supported')).toEqual({
      key: 'uncli.unavailable_unknown',
      params: { detail: 'GetUncliStatus is not supported' },
    })
  })

  it('names the service account the server runs as', () => {
    expect(uncliUnavailable({ ready: false, reason: 'service_account', account: 'LocalSystem' }, '')).toEqual({
      key: 'uncli.unavailable_service_account',
      params: { account: 'LocalSystem' },
    })
  })

  it('explains a host without an engine and a failed start', () => {
    expect(uncliUnavailable({ ready: false, reason: 'not_hosted' }, '')?.key).toBe('uncli.unavailable_not_hosted')
    expect(uncliUnavailable({ ready: false, reason: 'failed', detail: 'folder claimed' }, '')).toEqual({
      key: 'uncli.unavailable_failed',
      params: { detail: 'folder claimed' },
    })
  })
})
