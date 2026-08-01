import { describe, expect, it } from 'vitest'
import { connectionPresentation } from './headscaleConnection'

describe('connectionPresentation', () => {
  it.each([
    ['checking', undefined, { tone: 'checking', label: 'Checking Headscale', title: 'Checking Headscale connection' }],
    ['connected', { headscaleState: 'connected' }, { tone: 'connected', label: 'Headscale connected', title: 'Headscale is connected' }],
    ['unavailable', { headscaleState: 'unavailable' }, { tone: 'unavailable', label: 'Headscale unavailable', title: 'Headscale cannot be reached' }],
  ] as const)('shows %s state', (_, health, expected) => {
    expect(connectionPresentation(health)).toEqual(expected)
  })

  it('shows the last successful sync while reconnecting', () => {
    const lastSynced = '2026-08-01T00:00:00Z'

    expect(connectionPresentation({ headscaleState: 'stale', headscaleLastSynced: lastSynced })).toEqual({
      tone: 'stale',
      label: 'Headscale reconnecting',
      title: `Last successful sync: ${new Date(lastSynced).toLocaleString()}`,
    })
  })
})
