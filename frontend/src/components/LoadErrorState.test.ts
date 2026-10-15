import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import LoadErrorState from './LoadErrorState.svelte'

// Catches: load failures going back to a bare "Couldn't load" — the
// cause (e.g. a stale server's "DescribeAgent is not supported…") must
// stay visible so the failure is diagnosable from the screen.
describe('LoadErrorState', () => {
  it('shows the headline, the error underneath, and retries', async () => {
    const onRetry = vi.fn()
    render(LoadErrorState, {
      message: 'Could not load',
      error: new Error('DescribeAgent is not supported by BRUV server dev-f413920'),
      onRetry,
    })
    expect(screen.getByText('Could not load')).toBeTruthy()
    expect(screen.getByText(/DescribeAgent is not supported/)).toBeTruthy()
    await fireEvent.click(screen.getByRole('button'))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('omits the detail when there is nothing beyond the headline', () => {
    const { container } = render(LoadErrorState, { message: 'Could not load', error: new Error('Could not load'), onRetry: () => {} })
    expect(container.querySelector('.load-error-detail')).toBeNull()
  })
})
