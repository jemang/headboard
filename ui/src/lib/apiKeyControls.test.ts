import { describe, expect, it } from 'vitest'
import { expiryFromDays, isExpired } from './apiKeyControls'

describe('expiryFromDays', () => {
  it('converts whole days into the duration accepted by Headscale', () => {
    expect(expiryFromDays('90')).toBe('2160h')
  })

  it('rejects zero, negative, decimal, and non-numeric lifetimes', () => {
    expect(expiryFromDays('0')).toBeUndefined()
    expect(expiryFromDays('-1')).toBeUndefined()
    expect(expiryFromDays('1.5')).toBeUndefined()
    expect(expiryFromDays('week')).toBeUndefined()
  })
})

describe('isExpired', () => {
  it('only marks a key expired when its timestamp is in the past', () => {
    expect(isExpired('2000-01-01T00:00:00Z')).toBe(true)
    expect(isExpired('2999-01-01T00:00:00Z')).toBe(false)
    expect(isExpired()).toBe(false)
  })
})
