import { describe, expect, it } from 'vitest'
import { preAuthHardening } from './preAuthHardening'

describe('preAuthHardening', () => {
  it('requires hardening while a key without expiry remains', () => {
    const now = new Date('2026-08-01T00:00:00Z')
    expect(preAuthHardening([{ id: 'active', reusable: false, ephemeral: false, used: true }], now)).toMatchObject({ compliant: false })
    expect(preAuthHardening([{ id: 'expired', reusable: false, ephemeral: false, used: false, expiry: '2026-07-31T00:00:00Z' }], now)).toMatchObject({ compliant: true })
    expect(preAuthHardening([{ id: 'unknown-expiry', reusable: false, ephemeral: false, used: false, expiry: 'not-a-date' }], now)).toMatchObject({ compliant: false })
  })
})
